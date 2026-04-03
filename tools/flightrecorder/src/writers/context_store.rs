use std::fs::{File, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use bloomfilter::Bloom;
use tracing::info;

/// Append-only binary store for metric context definitions.
///
/// Each unique (context_key → name, tags) pair is written exactly once to
/// `contexts.bin`. A bloom filter (~120 KB) deduplicates context_keys so
/// repeated samples for the same metric don't cause writes.
///
/// ## File format
///
/// Length-prefixed binary records, appended sequentially:
/// ```text
/// u64 LE   context_key
/// u32 LE   name_len
/// [u8]     name bytes
/// u32 LE   tags_len
/// [u8]     tags bytes (pipe-joined: "host:web01|env:prod")
/// ```
///
/// No footer or index. The reader scans linearly.
pub struct ContextStore {
    file: File,
    path: PathBuf,
    bloom: Bloom<u64>,
    write_buf: Vec<u8>,

    /// Total number of unique contexts written to disk.
    pub contexts_written: u64,
    /// Total number of duplicate context_keys skipped by the bloom filter.
    pub contexts_deduplicated: u64,
}

impl ContextStore {
    /// Create or re-open the ContextStore in append mode.
    ///
    /// On sidecar restart, the file retains entries from previous runs.
    /// The bloom filter starts empty so agents will re-send all contexts;
    /// duplicates are harmless (the hydrate tool deduplicates on read).
    pub fn new(output_dir: impl AsRef<Path>) -> Result<Self> {
        let path = output_dir.as_ref().join("contexts.bin");
        let file = OpenOptions::new()
            .create(true)
            .append(true)
            .open(&path)
            .with_context(|| format!("opening {}", path.display()))?;

        info!(path = %path.display(), "context store opened (append mode)");

        Ok(Self {
            file,
            path,
            // 500K keys at 0.1% false positive rate ≈ 120 KB.
            bloom: Bloom::new_for_fp_rate(500_000, 0.001),
            write_buf: Vec::with_capacity(256),
            contexts_written: 0,
            contexts_deduplicated: 0,
        })
    }

    /// Record a context definition if the bloom filter hasn't seen this key.
    ///
    /// Returns `Ok(true)` if the context was new and written, `Ok(false)` if
    /// it was a bloom hit (already persisted).
    pub fn try_record(
        &mut self,
        context_key: u64,
        name: &str,
        tags_joined: &str,
    ) -> Result<bool> {
        if self.bloom.check(&context_key) {
            self.contexts_deduplicated += 1;
            return Ok(false);
        }
        self.bloom.set(&context_key);

        // Serialize: [context_key: u64 LE][name_len: u32 LE][name][tags_len: u32 LE][tags]
        self.write_buf.clear();
        self.write_buf
            .extend_from_slice(&context_key.to_le_bytes());
        self.write_buf
            .extend_from_slice(&(name.len() as u32).to_le_bytes());
        self.write_buf.extend_from_slice(name.as_bytes());
        self.write_buf
            .extend_from_slice(&(tags_joined.len() as u32).to_le_bytes());
        self.write_buf.extend_from_slice(tags_joined.as_bytes());

        self.file
            .write_all(&self.write_buf)
            .with_context(|| "appending to contexts.bin")?;

        self.contexts_written += 1;
        Ok(true)
    }

}

/// Read all context records from a `contexts.bin` file.
/// Returns a Vec of (context_key, name, tags_joined) tuples.
pub fn read_contexts_bin(path: &Path) -> Result<Vec<(u64, String, String)>> {
    let data = std::fs::read(path).with_context(|| format!("reading {}", path.display()))?;
    let mut results = Vec::new();
    let mut pos = 0;
    while pos < data.len() {
        if pos + 8 > data.len() {
            anyhow::bail!("truncated context_key at offset {pos}");
        }
        let context_key = u64::from_le_bytes(data[pos..pos + 8].try_into().unwrap());
        pos += 8;

        if pos + 4 > data.len() {
            anyhow::bail!("truncated name_len at offset {pos}");
        }
        let name_len = u32::from_le_bytes(data[pos..pos + 4].try_into().unwrap()) as usize;
        pos += 4;

        if pos + name_len > data.len() {
            anyhow::bail!("truncated name at offset {pos}");
        }
        let name = String::from_utf8_lossy(&data[pos..pos + name_len]).into_owned();
        pos += name_len;

        if pos + 4 > data.len() {
            anyhow::bail!("truncated tags_len at offset {pos}");
        }
        let tags_len = u32::from_le_bytes(data[pos..pos + 4].try_into().unwrap()) as usize;
        pos += 4;

        if pos + tags_len > data.len() {
            anyhow::bail!("truncated tags at offset {pos}");
        }
        let tags = String::from_utf8_lossy(&data[pos..pos + tags_len]).into_owned();
        pos += tags_len;

        results.push((context_key, name, tags));
    }
    Ok(results)
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn test_try_record_new_key() {
        let dir = tempdir().unwrap();
        let mut store = ContextStore::new(dir.path()).unwrap();
        assert!(store.try_record(1, "cpu.user", "host:a|env:prod").unwrap());
        assert_eq!(store.contexts_written, 1);
    }

    #[test]
    fn test_bloom_dedup() {
        let dir = tempdir().unwrap();
        let mut store = ContextStore::new(dir.path()).unwrap();
        assert!(store.try_record(42, "mem.used", "host:b").unwrap());
        assert!(!store.try_record(42, "mem.used", "host:b").unwrap());
        assert_eq!(store.contexts_written, 1);
        assert_eq!(store.contexts_deduplicated, 1);
    }

    #[test]
    fn test_roundtrip_binary_format() {
        let dir = tempdir().unwrap();
        let mut store = ContextStore::new(dir.path()).unwrap();

        store
            .try_record(100, "cpu.user", "host:web01|env:prod")
            .unwrap();
        store
            .try_record(200, "mem.usage", "host:web02|service:api")
            .unwrap();
        // Duplicate — should not be written.
        store
            .try_record(100, "cpu.user", "host:web01|env:prod")
            .unwrap();

        assert_eq!(store.contexts_written, 2);
        assert_eq!(store.contexts_deduplicated, 1);

        // Read back and verify.
        drop(store);
        let contexts = read_contexts_bin(&dir.path().join("contexts.bin")).unwrap();
        assert_eq!(contexts.len(), 2);
        assert_eq!(contexts[0], (100, "cpu.user".to_string(), "host:web01|env:prod".to_string()));
        assert_eq!(contexts[1], (200, "mem.usage".to_string(), "host:web02|service:api".to_string()));
    }

    #[test]
    fn test_append_survives_reopen() {
        let dir = tempdir().unwrap();
        // First open: write two contexts.
        {
            let mut store = ContextStore::new(dir.path()).unwrap();
            store.try_record(1, "a", "t:1").unwrap();
            store.try_record(2, "b", "t:2").unwrap();
        }
        // Second open: file is preserved, new bloom filter is empty so
        // the same keys can be written again (duplicates are harmless).
        {
            let mut store = ContextStore::new(dir.path()).unwrap();
            store.try_record(1, "a", "t:1").unwrap(); // duplicate, appended
            store.try_record(3, "c", "t:3").unwrap(); // new
        }
        let contexts = read_contexts_bin(&dir.path().join("contexts.bin")).unwrap();
        assert_eq!(contexts.len(), 4); // 2 from first open + 2 from second
        assert_eq!(contexts[0].0, 1);
        assert_eq!(contexts[3].0, 3);
    }

    #[test]
    fn test_empty_file_reads_ok() {
        let dir = tempdir().unwrap();
        let _store = ContextStore::new(dir.path()).unwrap();
        let contexts = read_contexts_bin(&dir.path().join("contexts.bin")).unwrap();
        assert!(contexts.is_empty());
    }
}
