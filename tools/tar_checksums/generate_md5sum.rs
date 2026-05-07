// Read a tarball and emit a file of the md5 checksums of all the plain files in it.
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
                    let mut reader = io::BufReader::new(file);
                    let mut writer = pipe_writer;
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
        let mut buf = [0u8; 65536];
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
