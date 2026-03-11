/// Reads a vortex file written by pipelinerecorder and prints its contents.
/// Usage: reader <path-to-vortex-file> [--search <substring>]
use std::path::PathBuf;

use anyhow::{Context, Result};
use clap::Parser;
use vortex::array::stream::ArrayStreamExt;
use vortex::array::Canonical;
use vortex::file::OpenOptionsSessionExt;
use vortex::session::VortexSession;
use vortex::VortexSessionDefault;

#[derive(Parser)]
struct Args {
    /// Path to the .vortex file to read
    path: PathBuf,
    /// Only print rows containing this substring
    #[arg(long)]
    search: Option<String>,
    /// Maximum rows to print (0 = all)
    #[arg(long, default_value = "0")]
    limit: usize,
}

#[tokio::main]
async fn main() -> Result<()> {
    let args = Args::parse();

    let session = VortexSession::default();
    let array = session
        .open_options()
        .open_path(args.path.clone())
        .await
        .with_context(|| format!("opening {}", args.path.display()))?
        .scan()?
        .into_array_stream()?
        .read_all()
        .await
        .context("reading array")?;

    let canonical = array.to_canonical().context("canonicalizing top array")?;
    let st = canonical.into_struct();

    let n = st.len();
    let names: Vec<String> = st.names().iter().map(|s| s.as_ref().to_string()).collect();

    eprintln!("file: {}", args.path.display());
    eprintln!("rows: {n}");
    eprintln!("columns: {}", names.join(", "));
    eprintln!("---");

    // Build per-column string representations
    let mut cols: Vec<Vec<String>> = Vec::with_capacity(names.len());
    for col_name in &names {
        let field_arr = st.unmasked_field_by_name(col_name.as_str())
            .with_context(|| format!("accessing column {col_name}"))?;
        let col_canonical = field_arr.to_canonical()
            .with_context(|| format!("canonicalizing column {col_name}"))?;

        let strings: Vec<String> = match col_canonical {
            Canonical::VarBinView(vbv) => {
                (0..n).map(|i| {
                    let bytes = vbv.bytes_at(i);
                    String::from_utf8_lossy(bytes.as_slice()).into_owned()
                }).collect()
            }
            Canonical::Primitive(prim) => {
                use vortex::array::dtype::PType;
                match prim.ptype() {
                    PType::I64 => prim.as_slice::<i64>().iter().map(|v| v.to_string()).collect(),
                    PType::F64 => prim.as_slice::<f64>().iter().map(|v| format!("{v:.6}")).collect(),
                    PType::I32 => prim.as_slice::<i32>().iter().map(|v| v.to_string()).collect(),
                    PType::F32 => prim.as_slice::<f32>().iter().map(|v| format!("{v:.6}")).collect(),
                    pt => (0..n).map(|_| format!("<{pt:?}>")).collect(),
                }
            }
            other => {
                let dt = other.dtype().to_string();
                (0..n).map(|_| format!("<{dt}>")).collect()
            }
        };
        cols.push(strings);
    }

    let mut printed = 0usize;
    for row in 0..n {
        let line = names.iter().zip(cols.iter())
            .map(|(name, col)| format!("{name}={}", col[row]))
            .collect::<Vec<_>>()
            .join(" | ");

        if let Some(ref search) = args.search {
            if !line.contains(search.as_str()) {
                continue;
            }
        }
        println!("{}", line);
        printed += 1;
        if args.limit > 0 && printed >= args.limit {
            eprintln!("(limit reached)");
            break;
        }
    }

    eprintln!("---");
    if let Some(ref s) = args.search {
        eprintln!("matched: {printed} / {n} rows (search: {s:?})");
    } else {
        eprintln!("printed: {printed} rows");
    }

    Ok(())
}
