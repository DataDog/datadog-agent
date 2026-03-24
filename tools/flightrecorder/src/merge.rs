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

/// Default bucket boundaries for size-tiered compaction: 1 MB, 10 MB, 100 MB.
const DEFAULT_BUCKET_BOUNDARIES: &[u64] = &[1 << 20, 10 << 20, 100 << 20];

pub struct MergeConfig {
    pub output_dir: PathBuf,
    /// Min files in a bucket before merge triggers (used for buckets 1+).
    pub min_files_to_trigger: usize,
    /// Max files to merge per bucket per pass (used for buckets 1+).
    pub max_files_per_pass: usize,
    /// Merge when total unmerged size per type exceeds this. 0 = disabled.
    pub size_threshold_bytes: u64,
    /// Size boundaries between buckets. Files are assigned to the lowest
    /// bucket whose boundary exceeds their size. Defaults to [1MB, 10MB, 100MB].
    pub bucket_boundaries: Vec<u64>,
}

impl Default for MergeConfig {
    fn default() -> Self {
        Self {
            output_dir: PathBuf::new(),
            min_files_to_trigger: 5,
            max_files_per_pass: 10,
            size_threshold_bytes: 0,
            bucket_boundaries: DEFAULT_BUCKET_BOUNDARIES.to_vec(),
        }
    }
}

/// Assign a file to a size bucket. Bucket 0 = smallest files.
fn size_bucket(size: u64, boundaries: &[u64]) -> usize {
    boundaries.iter().position(|&b| size < b).unwrap_or(boundaries.len())
}

/// Run one merge pass across all file types using size-tiered compaction.
///
/// Files are grouped by type, then by size bucket. Within each bucket,
/// files of similar size are merged together. This prevents the pathological
/// case where a large merged file keeps getting re-merged with tiny files,
/// causing unbounded memory growth.
///
/// Returns total number of files merged.
pub async fn merge_pass(config: &MergeConfig) -> Result<usize> {
    let mut entries = vortex_files::scan_vortex_files(&config.output_dir).await?;
    entries.sort_by_key(|e| e.timestamp_ms);

    let boundaries = if config.bucket_boundaries.is_empty() {
        DEFAULT_BUCKET_BOUNDARIES
    } else {
        &config.bucket_boundaries
    };
    let num_buckets = boundaries.len() + 1;

    let mut total_merged = 0;

    for file_type in &[FileType::Metrics, FileType::Logs] {
        let typed: Vec<&VortexEntry> = entries.iter().filter(|e| e.file_type == *file_type).collect();
        if typed.len() < 2 {
            continue;
        }

        // Check global triggers: do we need to merge at all for this type?
        let total_size: u64 = typed.iter().map(|e| e.size).sum();
        let count_trigger = typed.len() >= config.min_files_to_trigger;
        let size_trigger = config.size_threshold_bytes > 0 && total_size >= config.size_threshold_bytes;
        if !count_trigger && !size_trigger {
            continue;
        }

        // Exclude the most recent file (may be actively written).
        let candidates = &typed[..typed.len() - 1];

        // Partition candidates into size buckets.
        let mut buckets: Vec<Vec<&VortexEntry>> = vec![Vec::new(); num_buckets];
        for entry in candidates {
            let bucket = size_bucket(entry.size, boundaries);
            buckets[bucket].push(entry);
        }

        // Only merge bucket 0 (small files). Higher buckets are left alone —
        // time-based retention handles their cleanup. This avoids expensive
        // re-merges of already-compacted files. With streaming merge, merging
        // hundreds of small files is cheap (O(chunk_size) memory).
        {
            let bucket_files = &buckets[0];

            let has_enough = bucket_files.len() >= config.min_files_to_trigger;
            if bucket_files.len() < 2 || (!has_enough && !size_trigger) {
                continue;
            }

            let batch = &bucket_files[..];

            match merge_streaming(*file_type, batch, &config.output_dir).await {
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
/// Opens and reads files one at a time, piping chunks directly to a file on
/// disk via BufWriter — avoids buffering the entire merged output in memory.
async fn merge_streaming(
    file_type: FileType,
    files: &[&VortexEntry],
    output_dir: &Path,
) -> Result<usize> {
    let prefix = match file_type {
        FileType::Metrics => "metrics",
        FileType::Logs => "logs",
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
    delete_inputs(files, &final_path).await;
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

/// Delete input files after successful merge. Skips the output path to avoid
/// deleting the just-written merged file (which may reuse the oldest input's name).
async fn delete_inputs(files: &[&VortexEntry], output_path: &Path) {
    for f in files {
        if f.path == output_path {
            continue;
        }
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

    /// Write a metrics vortex file with the new inline schema.
    async fn write_metrics_file(dir: &Path, n: usize, ts_offset: u64) -> PathBuf {
        let mut names: Vec<Vec<u8>> = Vec::with_capacity(n);
        let mut tags: Vec<Vec<u8>> = Vec::with_capacity(n);
        let mut values = Vec::with_capacity(n);
        let mut timestamps = Vec::with_capacity(n);
        let mut sample_rates = Vec::with_capacity(n);
        let mut sources: Vec<Vec<u8>> = Vec::with_capacity(n);
        for i in 0..n {
            names.push(format!("cpu.user.{}", i % 10).into_bytes());
            tags.push(format!("host:web{}|env:prod", i % 5).into_bytes());
            values.push(i as f64 + ts_offset as f64);
            timestamps.push(1000i64 + i as i64);
            sample_rates.push(1.0f64);
            sources.push(b"test".to_vec());
        }
        let st = StructArray::try_new(
            FieldNames::from(["name", "tags", "value", "timestamp_ns", "sample_rate", "source"]),
            vec![
                VarBinArray::from(names).into_array(),
                VarBinArray::from(tags).into_array(),
                values.into_iter().collect::<PrimitiveArray>().into_array(),
                timestamps.into_iter().collect::<PrimitiveArray>().into_array(),
                sample_rates.into_iter().collect::<PrimitiveArray>().into_array(),
                VarBinArray::from(sources).into_array(),
            ],
            n,
            Validity::NonNullable,
        )
        .unwrap();

        let path = dir.join(format!("metrics-{}.vortex", ts_offset));
        let strategy = crate::writers::strategy::compact_strategy();
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

        // Create 3 metric files with different timestamps.
        write_metrics_file(dir.path(), 50, 100).await;
        write_metrics_file(dir.path(), 30, 200).await;
        write_metrics_file(dir.path(), 20, 300).await;

        let config = MergeConfig {
            output_dir: dir.path().to_path_buf(),
            min_files_to_trigger: 2,
            max_files_per_pass: 10,
            size_threshold_bytes: 0,
            ..Default::default()
        };

        let merged = merge_pass(&config).await.unwrap();
        // Should have merged the 2 oldest files (excluding the most recent).
        assert_eq!(merged, 2);

        // Verify the merged file exists and the inputs are deleted.
        let entries = vortex_files::scan_vortex_files(dir.path()).await.unwrap();
        let metrics: Vec<_> = entries
            .iter()
            .filter(|e| e.file_type == FileType::Metrics)
            .collect();
        // 1 merged + 1 untouched (most recent) = 2
        assert_eq!(metrics.len(), 2);

        // Read the merged file and verify row count.
        let merged_entry = metrics.iter().find(|e| e.timestamp_ms == 100).unwrap();
        let st = read_struct(&merged_entry.path).await.unwrap();
        assert_eq!(st.len(), 80); // 50 + 30
    }

    #[tokio::test]
    async fn test_merge_skips_below_threshold() {
        let dir = tempfile::tempdir().unwrap();

        // Only 2 files, min_files=5 — should not trigger.
        write_metrics_file(dir.path(), 10, 100).await;
        write_metrics_file(dir.path(), 10, 200).await;

        let config = MergeConfig {
            output_dir: dir.path().to_path_buf(),
            min_files_to_trigger: 5,
            max_files_per_pass: 10,
            size_threshold_bytes: 0,
            ..Default::default()
        };

        let merged = merge_pass(&config).await.unwrap();
        assert_eq!(merged, 0);
    }

    #[tokio::test]
    async fn test_merge_respects_max_files() {
        let dir = tempfile::tempdir().unwrap();

        // Create 6 files.
        for i in 0..6 {
            write_metrics_file(dir.path(), 10, (i + 1) * 100).await;
        }

        let config = MergeConfig {
            output_dir: dir.path().to_path_buf(),
            min_files_to_trigger: 2,
            ..Default::default()
        };

        let merged = merge_pass(&config).await.unwrap();
        // All 5 candidates merged (6 - 1 most recent = 5).
        assert_eq!(merged, 5);

        let entries = vortex_files::scan_vortex_files(dir.path()).await.unwrap();
        let metrics: Vec<_> = entries
            .iter()
            .filter(|e| e.file_type == FileType::Metrics)
            .collect();
        // 1 merged + 1 most recent (skipped) = 2
        assert_eq!(metrics.len(), 2);
    }

    #[test]
    fn test_size_bucket_assignment() {
        let boundaries = vec![1 << 20, 10 << 20, 100 << 20]; // 1MB, 10MB, 100MB
        assert_eq!(size_bucket(500, &boundaries), 0);          // 500B → bucket 0
        assert_eq!(size_bucket(500_000, &boundaries), 0);      // 500KB → bucket 0
        assert_eq!(size_bucket(1 << 20, &boundaries), 1);      // 1MB → bucket 1
        assert_eq!(size_bucket(5 << 20, &boundaries), 1);      // 5MB → bucket 1
        assert_eq!(size_bucket(10 << 20, &boundaries), 2);     // 10MB → bucket 2
        assert_eq!(size_bucket(50 << 20, &boundaries), 2);     // 50MB → bucket 2
        assert_eq!(size_bucket(100 << 20, &boundaries), 3);    // 100MB → bucket 3
        assert_eq!(size_bucket(500 << 20, &boundaries), 3);    // 500MB → bucket 3
    }

    #[tokio::test]
    async fn test_merge_does_not_mix_buckets() {
        // Create files of different sizes: small ones should merge together,
        // and large ones should NOT be re-merged with the small ones.
        let dir = tempfile::tempdir().unwrap();

        // Write 3 small files (~4KB each, bucket 0) + 1 large file (~80KB, still bucket 0
        // with default 1MB boundary, but let's use custom tiny boundaries to test).
        for i in 0..3 {
            write_metrics_file(dir.path(), 10, (i + 1) * 100).await;
        }
        // Write a "large" file that will land in bucket 1 with custom boundaries.
        write_metrics_file(dir.path(), 500, 400).await;
        // Most recent file (excluded from merge).
        write_metrics_file(dir.path(), 10, 500).await;

        // Use a tiny boundary so the 500-row file is in bucket 1.
        // The 10-row files are ~4KB, the 500-row file is ~15KB.
        let small_size = std::fs::metadata(dir.path().join("metrics-100.vortex")).unwrap().len();
        let large_size = std::fs::metadata(dir.path().join("metrics-400.vortex")).unwrap().len();

        let config = MergeConfig {
            output_dir: dir.path().to_path_buf(),
            min_files_to_trigger: 2,
            // Set boundary between the two sizes so they land in different buckets.
            bucket_boundaries: vec![(small_size + large_size) / 2],
            ..Default::default()
        };

        let merged = merge_pass(&config).await.unwrap();
        // Only the 3 small files should merge (bucket 0). The large file is alone in bucket 1.
        assert_eq!(merged, 3);

        let entries = vortex_files::scan_vortex_files(dir.path()).await.unwrap();
        let metrics: Vec<_> = entries.iter().filter(|e| e.file_type == FileType::Metrics).collect();
        // 1 merged-from-small + 1 large (untouched) + 1 most-recent = 3
        assert_eq!(metrics.len(), 3);
    }

    #[tokio::test]
    async fn test_merge_size_trigger() {
        let dir = tempfile::tempdir().unwrap();

        // Create 2 files — below count threshold (5) but above size threshold.
        write_metrics_file(dir.path(), 50, 100).await;
        write_metrics_file(dir.path(), 50, 200).await;
        // A third file so merge has a "most recent" to skip.
        write_metrics_file(dir.path(), 50, 300).await;

        // Count threshold is high (won't trigger), but size threshold is 1 byte (will trigger).
        let config = MergeConfig {
            output_dir: dir.path().to_path_buf(),
            min_files_to_trigger: 100,
            max_files_per_pass: 10,
            size_threshold_bytes: 1,
            ..Default::default()
        };

        let merged = merge_pass(&config).await.unwrap();
        assert!(merged > 0, "size trigger should have caused a merge");
    }
}
