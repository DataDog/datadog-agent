// Read a tarball and emit a file of the md5 checksums of all the plain files in it.
//
// Requirements:
// Read a .tar file (which might be compressed) and emits a file containing the md5 checksum of each file within that tar file.
//
// - must take the path to the input tar file as a command line arg
//   - if no input tar is provided, then use stdin as the file
// - must take the path to the output file as a command line arg
// - sample of the desired output format:  md5_sum path
// ```
// e3c6a486a70a471110731b1708d232cc  opt/datadog-installer/LICENSE
// f9a6f2aa44430e18abbc7363751e3f7c  opt/datadog-installer/LICENSES/THIRD-PARTY-0BSD
// 3b83ef96387f14655fc854ddc3c6bd57  opt/datadog-installer/LICENSES/THIRD-PARTY-Apache-2.0
// 11d3feb7137319430849e84dbc75ac27  opt/datadog-installer/LICENSES/THIRD-PARTY-BSD-2-Clause
// ```
// - emitted paths must be relative, with no preceding "./"
// - directories and symlinks in the tar file should be ignored.
// - support different compression algorithms that are used in our product
//   - we do not have to decode the compression from the binary itself, we can use the file name as a hint
//   - required for first implementation:  XZ compression, if the file ends in .xz,  gzip compression if the file ends in .gz or .tgz.

use std::env;
use std::fs;
use std::io::{self, Read, Write};
use std::thread;

use flate2::read::GzDecoder;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = env::args().collect();

    let (tar_file_path, output_path): (Option<String>, String) = match args.len() {
        2 => (None, args[1].clone()),
        3 => (Some(args[1].clone()), args[2].clone()),
        _ => {
            eprintln!("Usage: {} [input.tar[.gz|.tgz|.xz]] <output>", args[0]);
            std::process::exit(1);
        }
    };

    // Wrap the input file in a decompressor stream depending on the file extension.
    // Since XZ will spawn a thread, we need to Join at the end to catch any errors
    // which may have happened during decompression.
    let mut xz_thread: Option<thread::JoinHandle<Result<(), String>>> = None;
    let reader: Box<dyn Read> = match &tar_file_path {
        None => Box::new(io::stdin()),
        Some(path) => {
            let file = fs::File::open(path)?;
            if path.ends_with(".gz") || path.ends_with(".tgz") {
                Box::new(GzDecoder::new(file))
            } else if path.ends_with(".xz") {
                /* We are using lzma-rs because it is pure rust, but it does not
                 * have a streaming interface. So put it in a separate thread
                 * and use the write end of that pipe as our read end.
                 */
                let (pipe_reader, pipe_writer) = io::pipe()?;
                xz_thread = Some(thread::spawn(move || {
                    let mut reader = io::BufReader::with_capacity(1 << 16, file);
                    let mut writer = io::BufWriter::with_capacity(1 << 16, pipe_writer);
                    lzma_rs::xz_decompress(&mut reader, &mut writer).map_err(|e| e.to_string())
                }));
                Box::new(pipe_reader)
            } else {
                Box::new(file)
            }
        }
    };

    let mut archive = tar::Archive::new(reader);
    let mut output = fs::File::create(&output_path)?;
    for entry in archive.entries()? {
        let mut entry = entry?;
        if !entry.header().entry_type().is_file() {
            continue;
        }

        let path = entry.path()?.into_owned();
        let path_str = path.to_string_lossy();
        let path_str = path_str.strip_prefix("./").unwrap_or(&path_str);

        // pump the content of the file into the md5 hasher.
        let mut ctx = md5::Context::new();
        let mut buf = [0u8; 32768];
        loop {
            let n = entry.read(&mut buf)?;
            if n == 0 {
                break;
            }
            ctx.consume(&buf[..n]);
        }
        let digest = ctx.finalize();

        writeln!(output, "{:x}  {}", digest, path_str)?;
    }

    if let Some(handle) = xz_thread {
        handle
            .join()
            .unwrap_or_else(|_| Err("XZ decompressor thread panicked".to_string()))
            .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))?;
    }

    Ok(())
}
