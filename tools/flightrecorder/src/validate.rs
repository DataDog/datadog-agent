/// Validates vortex files written by flightrecorder.
///
/// Reads all .vortex metric files in a directory and outputs a JSON report with:
///   - total row count
///   - distinct (name, tags) context count
///   - any rows with empty/missing names (corruption indicator)
///   - per-file row counts
///
/// Usage: validate <directory> [--json]
///
/// Exit code 0 = all checks pass, 1 = validation errors found.
use std::collections::{HashMap, HashSet};
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
    /// Directory containing .vortex files
    dir: PathBuf,
    /// Output machine-readable JSON
    #[arg(long)]
    json: bool,
    /// Only validate metric files (metrics-*.vortex)
    #[arg(long, default_value = "true")]
    metrics_only: bool,
}

#[derive(Default)]
struct ValidationResult {
    total_rows: usize,
    files_read: usize,
    distinct_contexts: usize,
    empty_name_rows: usize,
    file_details: Vec<FileDetail>,
    errors: Vec<String>,
}

struct FileDetail {
    path: String,
    rows: usize,
}

#[tokio::main]
async fn main() -> Result<()> {
    let args = Args::parse();

    let mut entries: Vec<PathBuf> = std::fs::read_dir(&args.dir)
        .with_context(|| format!("reading directory {}", args.dir.display()))?
        .filter_map(|e| e.ok())
        .map(|e| e.path())
        .filter(|p| {
            p.extension().and_then(|e| e.to_str()) == Some("vortex")
                && (!args.metrics_only
                    || p.file_name()
                        .and_then(|n| n.to_str())
                        .is_some_and(|n| n.starts_with("metrics-")))
        })
        .collect();
    entries.sort();

    let mut result = ValidationResult::default();
    let mut contexts: HashSet<(String, String)> = HashSet::new();
    let mut context_counts: HashMap<(String, String), usize> = HashMap::new();
    let mut source_counts: HashMap<String, usize> = HashMap::new();

    let session = VortexSession::default();

    for path in &entries {
        match validate_file(&session, path, &mut contexts, &mut context_counts, &mut source_counts).await {
            Ok(detail) => {
                result.total_rows += detail.rows;
                result.files_read += 1;
                result.file_details.push(detail);
            }
            Err(e) => {
                result.errors.push(format!("{}: {e}", path.display()));
            }
        }
    }

    // Count anomalies from context_counts
    for ((name, _tags), _count) in &context_counts {
        if name.is_empty() {
            result.empty_name_rows += *_count;
        }
    }
    result.distinct_contexts = contexts.len();

    // Check for data quality issues across all files
    let has_errors = !result.errors.is_empty() || result.empty_name_rows > 0;

    if args.json {
        print_json(&result);
    } else {
        print_human(&result, &context_counts, &source_counts);
    }

    if has_errors {
        std::process::exit(1);
    }
    Ok(())
}

async fn validate_file(
    session: &VortexSession,
    path: &PathBuf,
    contexts: &mut HashSet<(String, String)>,
    context_counts: &mut HashMap<(String, String), usize>,
    source_counts: &mut HashMap<String, usize>,
) -> Result<FileDetail> {
    let array = session
        .open_options()
        .open_path(path.clone())
        .await
        .with_context(|| format!("opening {}", path.display()))?
        .scan()?
        .into_array_stream()?
        .read_all()
        .await
        .context("reading array")?;

    let canonical = array.to_canonical().context("canonicalizing")?;
    let st = canonical.into_struct();
    let n = st.len();

    // Extract name and tags columns
    let name_arr = st
        .unmasked_field_by_name("name")
        .context("accessing 'name' column")?;
    let name_canonical = name_arr.to_canonical().context("canonicalizing 'name'")?;

    let tags_arr = st
        .unmasked_field_by_name("tags")
        .context("accessing 'tags' column")?;
    let tags_canonical = tags_arr.to_canonical().context("canonicalizing 'tags'")?;

    let source_arr = st
        .unmasked_field_by_name("source")
        .context("accessing 'source' column")?;
    let source_canonical = source_arr.to_canonical().context("canonicalizing 'source'")?;

    let names = extract_strings(&name_canonical, n)?;
    let tags = extract_strings(&tags_canonical, n)?;
    let sources = extract_strings(&source_canonical, n)?;

    for i in 0..n {
        let key = (names[i].clone(), tags[i].clone());
        contexts.insert(key.clone());
        *context_counts.entry(key).or_insert(0) += 1;
        *source_counts.entry(sources[i].clone()).or_insert(0) += 1;
    }

    Ok(FileDetail {
        path: path.file_name().unwrap_or_default().to_string_lossy().into_owned(),
        rows: n,
    })
}

fn extract_strings(canonical: &Canonical, n: usize) -> Result<Vec<String>> {
    match canonical {
        Canonical::VarBinView(vbv) => Ok((0..n)
            .map(|i| String::from_utf8_lossy(vbv.bytes_at(i).as_slice()).into_owned())
            .collect()),
        other => anyhow::bail!("expected VarBinView column, got {:?}", other.dtype()),
    }
}

fn print_json(result: &ValidationResult) {
    println!("{{");
    println!("  \"total_rows\": {},", result.total_rows);
    println!("  \"files_read\": {},", result.files_read);
    println!("  \"distinct_contexts\": {},", result.distinct_contexts);
    println!("  \"empty_name_rows\": {},", result.empty_name_rows);
    println!(
        "  \"pass\": {},",
        result.errors.is_empty() && result.empty_name_rows == 0
    );
    println!("  \"files\": [");
    for (i, f) in result.file_details.iter().enumerate() {
        let comma = if i + 1 < result.file_details.len() {
            ","
        } else {
            ""
        };
        println!(
            "    {{\"path\": \"{}\", \"rows\": {}}}{}",
            f.path, f.rows, comma
        );
    }
    println!("  ],");
    println!("  \"errors\": [");
    for (i, e) in result.errors.iter().enumerate() {
        let comma = if i + 1 < result.errors.len() {
            ","
        } else {
            ""
        };
        // Escape quotes in error messages
        let escaped = e.replace('\\', "\\\\").replace('"', "\\\"");
        println!("    \"{escaped}\"{comma}");
    }
    println!("  ]");
    println!("}}");
}

fn print_human(result: &ValidationResult, context_counts: &HashMap<(String, String), usize>, source_counts: &HashMap<String, usize>) {
    eprintln!("=== Vortex Validation Report ===");
    eprintln!("Files read:        {}", result.files_read);
    eprintln!("Total rows:        {}", result.total_rows);
    eprintln!("Distinct contexts: {}", result.distinct_contexts);
    eprintln!();

    if result.empty_name_rows > 0 {
        eprintln!(
            "FAIL: {} rows with empty metric name",
            result.empty_name_rows
        );
    }

    if !result.errors.is_empty() {
        eprintln!("FAIL: {} file read errors:", result.errors.len());
        for e in &result.errors {
            eprintln!("  - {e}");
        }
    }

    if result.errors.is_empty() && result.empty_name_rows == 0 {
        eprintln!("PASS: all rows have valid metric names");
    }

    // Show top 20 contexts by frequency
    let mut sorted: Vec<_> = context_counts.iter().collect();
    sorted.sort_by(|a, b| b.1.cmp(a.1));
    eprintln!();
    eprintln!("Top 20 contexts by frequency:");
    for (i, ((name, tags), count)) in sorted.iter().take(20).enumerate() {
        let tag_preview = if tags.len() > 60 {
            format!("{}...", &tags[..60])
        } else {
            tags.clone()
        };
        eprintln!("  {}. {} [{}] — {} rows", i + 1, name, tag_preview, count);
    }

    // Show rows by pipeline source
    let mut source_sorted: Vec<_> = source_counts.iter().collect();
    source_sorted.sort_by(|a, b| b.1.cmp(a.1));
    eprintln!();
    eprintln!("Rows by pipeline source:");
    for (source, count) in &source_sorted {
        let label = if source.is_empty() { "(empty)" } else { source.as_str() };
        eprintln!("  {:40} {:>8} rows", label, count);
    }

    // Show distinct metric name prefixes (first dotted segment)
    let mut name_prefixes: HashMap<String, usize> = HashMap::new();
    for ((name, _), count) in context_counts.iter() {
        let prefix = name.split('.').next().unwrap_or(name).to_string();
        *name_prefixes.entry(prefix).or_insert(0) += count;
    }
    let mut prefix_sorted: Vec<_> = name_prefixes.iter().collect();
    prefix_sorted.sort_by(|a, b| b.1.cmp(a.1));
    eprintln!();
    eprintln!("Metric name prefixes:");
    for (prefix, count) in &prefix_sorted {
        eprintln!("  {:40} {:>8} rows", prefix, count);
    }

    eprintln!();
    eprintln!("Per-file row counts:");
    for f in &result.file_details {
        eprintln!("  {:50} {:>8} rows", f.path, f.rows);
    }
}
