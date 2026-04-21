/// `flightrecorder archive` subcommand.
///
/// Converts `contexts.bin` to `contexts.parquet` (in a temporary directory that
/// is cleaned up on exit) and packs all `.parquet` files in the input directory
/// together with the generated `contexts.parquet` into a `.tar.zst` archive.
use std::fs::File;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};

use flightrecorder::context_parquet::write_contexts_parquet;
use flightrecorder::writers::context_store::read_contexts_bin;

pub fn run(input_dir: &Path, output: Option<&Path>) -> Result<()> {
    if !input_dir.is_dir() {
        anyhow::bail!("input directory does not exist: {}", input_dir.display());
    }

    // Collect all existing .parquet files in the input directory.
    let mut parquet_files: Vec<PathBuf> = Vec::new();
    for entry in std::fs::read_dir(input_dir)
        .with_context(|| format!("reading directory {}", input_dir.display()))?
    {
        let entry = entry?;
        let path = entry.path();
        if path.extension().and_then(|e| e.to_str()) == Some("parquet") {
            parquet_files.push(path);
        }
    }
    parquet_files.sort();

    // Hydrate contexts.bin → contexts.parquet in a temp dir (if present).
    let contexts_bin = input_dir.join("contexts.bin");
    let temp_dir = if contexts_bin.exists() {
        let td = tempfile::TempDir::new().context("creating temp dir")?;
        let out = td.path().join("contexts.parquet");
        let contexts = read_contexts_bin(&contexts_bin)
            .with_context(|| format!("reading {}", contexts_bin.display()))?;
        write_contexts_parquet(&contexts, &out)
            .with_context(|| format!("writing contexts.parquet to {}", out.display()))?;
        eprintln!("  {} contexts hydrated", contexts.len());
        Some((td, out))
    } else {
        None
    };

    if parquet_files.is_empty() && temp_dir.is_none() {
        anyhow::bail!(
            "no .parquet files or contexts.bin found in {}",
            input_dir.display()
        );
    }

    // Resolve output path.
    let output_path: PathBuf = match output {
        Some(p) => p.to_path_buf(),
        None => {
            let ts = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap_or_default()
                .as_millis();
            PathBuf::from(format!("signals-{}.tar.zst", ts))
        }
    };

    // Write tar.zst archive.
    let out_file = File::create(&output_path)
        .with_context(|| format!("creating archive {}", output_path.display()))?;
    let zstd_enc = zstd::stream::write::Encoder::new(out_file, 3)
        .context("initializing zstd encoder")?;
    let mut tar = tar::Builder::new(zstd_enc);

    let mut file_count = 0usize;

    for path in &parquet_files {
        let name = path
            .file_name()
            .expect("parquet path has no filename")
            .to_string_lossy()
            .into_owned();
        tar.append_path_with_name(path, &name)
            .with_context(|| format!("archiving {}", path.display()))?;
        file_count += 1;
    }

    if let Some((_, ctx_path)) = &temp_dir {
        tar.append_path_with_name(ctx_path, "contexts.parquet")
            .context("archiving contexts.parquet")?;
        file_count += 1;
    }

    let zstd_enc = tar.into_inner().context("finalizing tar archive")?;
    zstd_enc.finish().context("finalizing zstd stream")?;

    let archive_size = std::fs::metadata(&output_path)
        .map(|m| m.len())
        .unwrap_or(0);
    eprintln!(
        "Archived {} file(s) → {} ({:.1} MB)",
        file_count,
        output_path.display(),
        archive_size as f64 / 1_048_576.0,
    );

    // temp_dir dropped here — cleans up the temporary contexts.parquet.
    drop(temp_dir);

    Ok(())
}
