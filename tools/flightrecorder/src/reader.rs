/// Reads a vortex file written by flightrecorder and prints its contents.
///
/// For metrics files, resolves context_key to name+tags using context files
/// from the same directory (or --context-dir).
///
/// Usage: reader <path-to-vortex-file> [--search <substring>] [--context-dir <dir>]
use std::collections::HashMap;
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
    /// Directory containing contexts-*.vortex files for resolving context keys.
    /// Defaults to the parent directory of the input file.
    #[arg(long)]
    context_dir: Option<PathBuf>,
}

/// Load all `contexts-*.vortex` files from a directory into a HashMap.
async fn load_contexts(
    session: &VortexSession,
    dir: &PathBuf,
) -> Result<HashMap<u64, (String, String)>> {
    let mut context_map: HashMap<u64, (String, String)> = HashMap::new();

    let mut entries: Vec<PathBuf> = std::fs::read_dir(dir)
        .with_context(|| format!("reading directory {}", dir.display()))?
        .filter_map(|e| e.ok())
        .map(|e| e.path())
        .filter(|p| {
            p.extension().and_then(|e| e.to_str()) == Some("vortex")
                && p.file_name()
                    .and_then(|n| n.to_str())
                    .is_some_and(|n| n.starts_with("contexts-"))
        })
        .collect();
    entries.sort();

    for path in &entries {
        let array = session
            .open_options()
            .open_path(path.clone())
            .await
            .with_context(|| format!("opening context file {}", path.display()))?
            .scan()?
            .into_array_stream()?
            .read_all()
            .await
            .context("reading context array")?;

        let canonical = array.to_canonical().context("canonicalizing context array")?;
        let st = canonical.into_struct();
        let n = st.len();

        let ckey_arr = st
            .unmasked_field_by_name("context_key")
            .context("accessing 'context_key' column")?;
        let ckey_canonical = ckey_arr
            .to_canonical()
            .context("canonicalizing 'context_key'")?;
        let ckeys = match &ckey_canonical {
            Canonical::Primitive(prim) => prim.as_slice::<u64>().to_vec(),
            other => anyhow::bail!("expected Primitive u64 for context_key, got {:?}", other.dtype()),
        };

        let name_arr = st
            .unmasked_field_by_name("name")
            .context("accessing 'name' column")?;
        let name_canonical = name_arr.to_canonical().context("canonicalizing 'name'")?;
        let names = extract_strings(&name_canonical, n)?;

        let tags_arr = st
            .unmasked_field_by_name("tags")
            .context("accessing 'tags' column")?;
        let tags_canonical = tags_arr.to_canonical().context("canonicalizing 'tags'")?;
        let tags = extract_strings(&tags_canonical, n)?;

        for i in 0..n {
            context_map.insert(ckeys[i], (names[i].clone(), tags[i].clone()));
        }
    }

    Ok(context_map)
}

fn extract_strings(canonical: &Canonical, n: usize) -> Result<Vec<String>> {
    match canonical {
        Canonical::VarBinView(vbv) => Ok((0..n)
            .map(|i| String::from_utf8_lossy(vbv.bytes_at(i).as_slice()).into_owned())
            .collect()),
        other => anyhow::bail!("expected VarBinView column, got {:?}", other.dtype()),
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    let args = Args::parse();

    let session = VortexSession::default();

    // Determine if this is a metrics file that needs context resolution.
    let is_metrics = args
        .path
        .file_name()
        .and_then(|n| n.to_str())
        .is_some_and(|n| n.starts_with("metrics-"));

    // Load context map for metrics files.
    let context_map = if is_metrics {
        let ctx_dir = args
            .context_dir
            .clone()
            .or_else(|| args.path.parent().map(|p| p.to_path_buf()));
        if let Some(dir) = ctx_dir {
            match load_contexts(&session, &dir).await {
                Ok(map) => {
                    if !map.is_empty() {
                        eprintln!("loaded {} context definitions", map.len());
                    }
                    Some(map)
                }
                Err(e) => {
                    eprintln!("warning: could not load contexts: {e}");
                    None
                }
            }
        } else {
            None
        }
    } else {
        None
    };

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
        let field_arr = st
            .unmasked_field_by_name(col_name.as_str())
            .with_context(|| format!("accessing column {col_name}"))?;
        let col_canonical = field_arr
            .to_canonical()
            .with_context(|| format!("canonicalizing column {col_name}"))?;

        let strings: Vec<String> = match col_canonical {
            Canonical::VarBinView(vbv) => (0..n)
                .map(|i| {
                    let bytes = vbv.bytes_at(i);
                    String::from_utf8_lossy(bytes.as_slice()).into_owned()
                })
                .collect(),
            Canonical::Primitive(prim) => {
                use vortex::array::dtype::PType;
                match prim.ptype() {
                    PType::I64 => prim
                        .as_slice::<i64>()
                        .iter()
                        .map(|v| v.to_string())
                        .collect(),
                    PType::F64 => prim
                        .as_slice::<f64>()
                        .iter()
                        .map(|v| format!("{v:.6}"))
                        .collect(),
                    PType::I32 => prim
                        .as_slice::<i32>()
                        .iter()
                        .map(|v| v.to_string())
                        .collect(),
                    PType::F32 => prim
                        .as_slice::<f32>()
                        .iter()
                        .map(|v| format!("{v:.6}"))
                        .collect(),
                    PType::U64 => prim
                        .as_slice::<u64>()
                        .iter()
                        .map(|v| v.to_string())
                        .collect(),
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

    // If we have a context map and a context_key column, build virtual name/tags columns.
    let context_key_col_idx = names.iter().position(|n| n == "context_key");
    let resolved_names: Option<Vec<String>>;
    let resolved_tags: Option<Vec<String>>;
    if let (Some(ctx_map), Some(ckey_idx)) = (&context_map, context_key_col_idx) {
        let mut r_names = Vec::with_capacity(n);
        let mut r_tags = Vec::with_capacity(n);
        for row in 0..n {
            let ckey: u64 = cols[ckey_idx][row].parse().unwrap_or(0);
            if let Some((name, tags)) = ctx_map.get(&ckey) {
                r_names.push(name.clone());
                r_tags.push(tags.clone());
            } else {
                r_names.push(format!("<unresolved:{ckey}>"));
                r_tags.push(String::new());
            }
        }
        resolved_names = Some(r_names);
        resolved_tags = Some(r_tags);
    } else {
        resolved_names = None;
        resolved_tags = None;
    }

    let mut printed = 0usize;
    for row in 0..n {
        let mut parts: Vec<String> = names
            .iter()
            .zip(cols.iter())
            .map(|(name, col)| format!("{name}={}", col[row]))
            .collect();

        // Append resolved name/tags as virtual columns.
        if let (Some(ref rn), Some(ref rt)) = (&resolved_names, &resolved_tags) {
            parts.push(format!("name={}", rn[row]));
            parts.push(format!("tags={}", rt[row]));
        }

        let line = parts.join(" | ");

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
