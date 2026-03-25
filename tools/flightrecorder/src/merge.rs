use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use tracing::{info, warn};
use vortex::array::arrays::StructArray;
use vortex::array::stream::{ArrayStreamAdapter, ArrayStreamExt, SendableArrayStream};
use vortex::file::{OpenOptionsSessionExt, VortexWriteOptions};
use vortex::session::VortexSession;
use vortex::VortexSessionDefault;
use crate::vortex_files::{self, FileType, VortexEntry};
use crate::writers::strategy::compact_strategy;

pub struct MergeConfig {
    pub output_dir: PathBuf,
    /// Min flush files before merge triggers.
    pub min_files_to_trigger: usize,
}

impl Default for MergeConfig {
    fn default() -> Self {
        Self {
            output_dir: PathBuf::new(),
            min_files_to_trigger: 5,
        }
    }
}

/// Run one merge pass across all file types.
///
/// Collects all flush files (`flush-metrics-*`, `flush-logs-*`) per type and
/// merges them into a single compressed file (`metrics-*`, `logs-*`).
/// Already-merged files are never re-merged.
///
/// Returns total number of flush files merged.
pub async fn merge_pass(config: &MergeConfig) -> Result<usize> {
    let mut entries = vortex_files::scan_vortex_files(&config.output_dir).await?;
    entries.sort_by_key(|e| e.timestamp_ms);

    let mut total_merged = 0;

    for file_type in &[FileType::Metrics, FileType::Logs, FileType::TraceStats] {
        // Only collect flush files — merged files are already compressed.
        let flush_files: Vec<&VortexEntry> = entries
            .iter()
            .filter(|e| e.file_type == *file_type && e.is_flush)
            .collect();

        if flush_files.len() < config.min_files_to_trigger {
            continue;
        }

        // Exclude the most recent flush file (may be actively written).
        if flush_files.len() < 2 {
            continue;
        }
        let candidates = &flush_files[..flush_files.len() - 1];

        match merge_streaming(*file_type, candidates, &config.output_dir).await {
            Ok(merged) => {
                if merged > 0 {
                    info!(
                        file_type = %file_type,
                        input_files = merged,
                        "merge complete"
                    );
                }
                total_merged += merged;
            }
            Err(e) => {
                warn!(file_type = %file_type, "merge pass failed: {}", e);
            }
        }
    }

    Ok(total_merged)
}

// ---------------------------------------------------------------------------
// Streaming merge for metrics/logs (no dedup needed)
// ---------------------------------------------------------------------------

/// Create a streaming merge for file types.
/// Opens files lazily one at a time via an async generator pattern.
async fn open_chained_stream(
    paths: Vec<PathBuf>,
) -> Result<impl vortex::array::stream::ArrayStream + Send + 'static> {
    // We need the dtype from the first file.
    if paths.is_empty() {
        anyhow::bail!("no files to chain");
    }

    let session = VortexSession::default();
    let first_scan = session
        .open_options()
        .open_path(paths[0].clone())
        .await
        .with_context(|| format!("opening {}", paths[0].display()))?
        .scan()?;
    let dtype = first_scan.dtype()?;
    let first_stream = first_scan.into_array_stream()?;

    // Build a stream that yields chunks from all files sequentially.
    // We use futures::stream::unfold to lazily open files one at a time.
    struct State {
        paths: Vec<PathBuf>,
        current_idx: usize,
        current_stream: SendableArrayStream,
    }

    let state = State {
        paths,
        current_idx: 0,
        current_stream: ArrayStreamExt::boxed(first_stream),
    };

    let stream = futures::stream::unfold(state, |mut state| async move {
        loop {
            // Try to get next chunk from current stream.
            use futures::StreamExt;
            match state.current_stream.next().await {
                Some(item) => return Some((item, state)),
                None => {
                    // Current file exhausted, move to next.
                    state.current_idx += 1;
                    if state.current_idx >= state.paths.len() {
                        return None; // All files consumed.
                    }
                    // Open next file.
                    let session = VortexSession::default();
                    let stream = match session
                        .open_options()
                        .open_path(state.paths[state.current_idx].clone())
                        .await
                    {
                        Ok(file) => match file.scan() {
                            Ok(scan) => match scan.into_array_stream() {
                                Ok(s) => ArrayStreamExt::boxed(s),
                                Err(e) => return Some((Err(e), state)),
                            },
                            Err(e) => return Some((Err(e), state)),
                        },
                        Err(e) => return Some((Err(e), state)),
                    };
                    state.current_stream = stream;
                }
            }
        }
    });

    Ok(ArrayStreamAdapter::new(dtype, stream))
}

/// Streaming merge for metrics and logs.
/// Opens and reads flush files one at a time, piping chunks directly to a file
/// on disk — avoids buffering the entire merged output in memory.
/// Output is a compressed merged file (no `flush-` prefix).
async fn merge_streaming(
    file_type: FileType,
    files: &[&VortexEntry],
    output_dir: &Path,
) -> Result<usize> {
    let prefix = match file_type {
        FileType::Metrics => "metrics",
        FileType::Logs => "logs",
        FileType::TraceStats => "trace_stats",
    };
    let oldest_ts = files[0].timestamp_ms;
    let final_path = output_dir.join(format!("{prefix}-{oldest_ts}.vortex"));
    let tmp_path = output_dir.join(format!("{prefix}-{oldest_ts}.vortex.tmp"));

    let paths: Vec<PathBuf> = files.iter().map(|e| e.path.clone()).collect();
    let stream = open_chained_stream(paths)
        .await
        .context("opening chained stream")?;

    let strategy = compact_strategy();
    let session = VortexSession::default();

    // Write directly to a file instead of buffering in a Vec to avoid holding
    // the entire merged output in memory (which caused ~130 MB RSS spikes).
    let mut file = tokio::fs::File::create(&tmp_path)
        .await
        .with_context(|| format!("creating {}", tmp_path.display()))?;

    VortexWriteOptions::new(session)
        .with_strategy(strategy)
        .write(&mut file, stream)
        .await
        .context("streaming merge write")?;

    // Dump heap profile at peak memory — the Vortex pipeline's internal state
    // is still live here, before we drop the file handle and clean up.
    crate::heap_prof::dump_heap_profile(output_dir, &format!("merge-peak-{prefix}"));

    let output_bytes = tmp_path
        .metadata()
        .map(|m| m.len())
        .unwrap_or(0);

    tokio::fs::rename(&tmp_path, &final_path)
        .await
        .with_context(|| format!("renaming {} -> {}", tmp_path.display(), final_path.display()))?;

    let count = files.len();
    delete_inputs(files).await;
    info!(
        output = %final_path.display(),
        input_files = count,
        output_bytes,
        "streaming merge complete ({file_type})"
    );

    Ok(count)
}

/// Read a vortex file and return its StructArray.
/// Used by tests.
#[allow(dead_code)]
async fn read_struct(path: &Path) -> Result<StructArray> {
    let session = VortexSession::default();
    let array = session
        .open_options()
        .open_path(path.to_path_buf())
        .await
        .with_context(|| format!("opening {}", path.display()))?
        .scan()?
        .into_array_stream()?
        .read_all()
        .await
        .with_context(|| format!("reading {}", path.display()))?;
    let canonical = array.to_canonical()?;
    Ok(canonical.into_struct())
}

/// Delete input flush files after successful merge.
async fn delete_inputs(files: &[&VortexEntry]) {
    for f in files {
        if let Err(e) = tokio::fs::remove_file(&f.path).await {
            warn!(path = %f.path.display(), "failed to delete merged input: {}", e);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use vortex::array::arrays::{PrimitiveArray, VarBinArray};
    use vortex::array::dtype::FieldNames;
    use vortex::array::validity::Validity;
    use vortex::array::IntoArray;

    /// Write a flush metrics vortex file with the decomposed-tag schema (13 columns).
    async fn write_flush_metrics_file(dir: &Path, n: usize, ts_offset: u64) -> PathBuf {
        use crate::writers::metrics::METRIC_FIELD_NAMES;

        let mut names: Vec<Vec<u8>> = Vec::with_capacity(n);
        let mut tag_host: Vec<Vec<u8>> = Vec::with_capacity(n);
        let mut tag_device: Vec<Vec<u8>> = Vec::with_capacity(n);
        let mut tag_source: Vec<Vec<u8>> = Vec::with_capacity(n);
        let mut tag_service: Vec<Vec<u8>> = Vec::with_capacity(n);
        let mut tag_env: Vec<Vec<u8>> = Vec::with_capacity(n);
        let mut tag_version: Vec<Vec<u8>> = Vec::with_capacity(n);
        let mut tag_team: Vec<Vec<u8>> = Vec::with_capacity(n);
        let mut tags_overflow: Vec<Vec<u8>> = Vec::with_capacity(n);
        let mut values = Vec::with_capacity(n);
        let mut timestamps = Vec::with_capacity(n);
        let mut sample_rates = Vec::with_capacity(n);
        let mut sources: Vec<Vec<u8>> = Vec::with_capacity(n);
        for i in 0..n {
            names.push(format!("cpu.user.{}", i % 10).into_bytes());
            tag_host.push(format!("web{}", i % 5).into_bytes());
            tag_device.push(b"".to_vec());
            tag_source.push(b"".to_vec());
            tag_service.push(b"".to_vec());
            tag_env.push(b"prod".to_vec());
            tag_version.push(b"".to_vec());
            tag_team.push(b"".to_vec());
            tags_overflow.push(b"".to_vec());
            values.push(i as f64 + ts_offset as f64);
            timestamps.push(1000i64 + i as i64);
            sample_rates.push(1.0f64);
            sources.push(b"test".to_vec());
        }
        let st = StructArray::try_new(
            FieldNames::from(METRIC_FIELD_NAMES),
            vec![
                VarBinArray::from(names).into_array(),
                VarBinArray::from(tag_host).into_array(),
                VarBinArray::from(tag_device).into_array(),
                VarBinArray::from(tag_source).into_array(),
                VarBinArray::from(tag_service).into_array(),
                VarBinArray::from(tag_env).into_array(),
                VarBinArray::from(tag_version).into_array(),
                VarBinArray::from(tag_team).into_array(),
                VarBinArray::from(tags_overflow).into_array(),
                values.into_iter().collect::<PrimitiveArray>().into_array(),
                timestamps.into_iter().collect::<PrimitiveArray>().into_array(),
                sample_rates.into_iter().collect::<PrimitiveArray>().into_array(),
                VarBinArray::from(sources).into_array(),
            ],
            n,
            Validity::NonNullable,
        )
        .unwrap();

        // Flush files use the flush- prefix.
        let path = dir.join(format!("flush-metrics-{}.vortex", ts_offset));
        let strategy = crate::writers::strategy::fast_flush_strategy();
        let session = VortexSession::default();
        let mut buf = Vec::new();
        VortexWriteOptions::new(session)
            .with_strategy(strategy)
            .write(&mut buf, st.into_array().to_array_stream())
            .await
            .unwrap();
        tokio::fs::write(&path, &buf).await.unwrap();
        path
    }

    #[tokio::test]
    async fn test_merge_metrics() {
        let dir = tempfile::tempdir().unwrap();

        // Create 3 flush metric files with different timestamps.
        write_flush_metrics_file(dir.path(), 50, 100).await;
        write_flush_metrics_file(dir.path(), 30, 200).await;
        write_flush_metrics_file(dir.path(), 20, 300).await;

        let config = MergeConfig {
            output_dir: dir.path().to_path_buf(),
            min_files_to_trigger: 2,
        };

        let merged = merge_pass(&config).await.unwrap();
        // Should have merged the 2 oldest flush files (excluding the most recent).
        assert_eq!(merged, 2);

        // Verify the merged file exists and the flush inputs are deleted.
        let entries = vortex_files::scan_vortex_files(dir.path()).await.unwrap();
        let metrics: Vec<_> = entries
            .iter()
            .filter(|e| e.file_type == FileType::Metrics)
            .collect();
        // 1 merged (metrics-100.vortex) + 1 untouched flush (flush-metrics-300.vortex) = 2
        assert_eq!(metrics.len(), 2);

        let merged_entry = metrics.iter().find(|e| !e.is_flush).unwrap();
        assert_eq!(merged_entry.timestamp_ms, 100);
        let st = read_struct(&merged_entry.path).await.unwrap();
        assert_eq!(st.len(), 80); // 50 + 30

        let flush_entry = metrics.iter().find(|e| e.is_flush).unwrap();
        assert_eq!(flush_entry.timestamp_ms, 300);
    }

    #[tokio::test]
    async fn test_merge_skips_below_threshold() {
        let dir = tempfile::tempdir().unwrap();

        // Only 2 flush files, min_files=5 — should not trigger.
        write_flush_metrics_file(dir.path(), 10, 100).await;
        write_flush_metrics_file(dir.path(), 10, 200).await;

        let config = MergeConfig {
            output_dir: dir.path().to_path_buf(),
            min_files_to_trigger: 5,
        };

        let merged = merge_pass(&config).await.unwrap();
        assert_eq!(merged, 0);
    }

    #[tokio::test]
    async fn test_merge_all_flush_files() {
        let dir = tempfile::tempdir().unwrap();

        // Create 6 flush files.
        for i in 0..6 {
            write_flush_metrics_file(dir.path(), 10, (i + 1) * 100).await;
        }

        let config = MergeConfig {
            output_dir: dir.path().to_path_buf(),
            min_files_to_trigger: 2,
        };

        let merged = merge_pass(&config).await.unwrap();
        // All 5 candidates merged (6 - 1 most recent = 5).
        assert_eq!(merged, 5);

        let entries = vortex_files::scan_vortex_files(dir.path()).await.unwrap();
        let metrics: Vec<_> = entries
            .iter()
            .filter(|e| e.file_type == FileType::Metrics)
            .collect();
        // 1 merged + 1 most recent flush = 2
        assert_eq!(metrics.len(), 2);
        assert!(metrics.iter().any(|e| !e.is_flush)); // merged file exists
        assert!(metrics.iter().any(|e| e.is_flush));  // most recent flush preserved
    }

    #[tokio::test]
    async fn test_merge_ignores_already_merged() {
        let dir = tempfile::tempdir().unwrap();

        // Create a pre-existing merged file (no flush- prefix).
        write_flush_metrics_file(dir.path(), 50, 50).await;
        // Rename it to look like a merged file.
        tokio::fs::rename(
            dir.path().join("flush-metrics-50.vortex"),
            dir.path().join("metrics-50.vortex"),
        )
        .await
        .unwrap();

        // Create 3 flush files.
        write_flush_metrics_file(dir.path(), 10, 100).await;
        write_flush_metrics_file(dir.path(), 10, 200).await;
        write_flush_metrics_file(dir.path(), 10, 300).await;

        let config = MergeConfig {
            output_dir: dir.path().to_path_buf(),
            min_files_to_trigger: 2,
        };

        let merged = merge_pass(&config).await.unwrap();
        // Only 2 flush files merged (3 - 1 most recent = 2).
        // The already-merged file is NOT re-merged.
        assert_eq!(merged, 2);

        let entries = vortex_files::scan_vortex_files(dir.path()).await.unwrap();
        let metrics: Vec<_> = entries
            .iter()
            .filter(|e| e.file_type == FileType::Metrics)
            .collect();
        // original merged (ts=50) + new merged (ts=100) + most recent flush (ts=300) = 3
        assert_eq!(metrics.len(), 3);
        assert_eq!(metrics.iter().filter(|e| !e.is_flush).count(), 2);
        assert_eq!(metrics.iter().filter(|e| e.is_flush).count(), 1);
    }
}
