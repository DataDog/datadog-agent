// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! Test utilities for handling cross-platform test data paths
#![allow(clippy::unwrap_used)]
#![allow(clippy::panic)]

use crate::fs::SubDirFs;
use normalize_path::NormalizePath;
use std::path::{Path, PathBuf};
use tempfile::TempDir;
use walkdir::WalkDir;

/// Get the base path for testdata files.
pub fn testdata_path() -> PathBuf {
    let manifest_dir = std::env::var("CARGO_MANIFEST_DIR").unwrap_or_else(|_| ".".to_string());
    PathBuf::from(manifest_dir).join("testdata").normalize()
}

pub struct TestDataFs {
    path: PathBuf,
    temp_dir: TempDir,
    subdirfs: SubDirFs,
}

impl AsRef<SubDirFs> for TestDataFs {
    fn as_ref(&self) -> &SubDirFs {
        &self.subdirfs
    }
}

// Implement Drop so that the individual fields can't be moved out, since we
// need to keep the temporary directory alive while the TestData is in use,
// since the temporary directory will be deleted when it is dropped.
impl Drop for TestDataFs {
    fn drop(&mut self) {}
}

impl AsRef<TempDir> for TestDataFs {
    fn as_ref(&self) -> &TempDir {
        &self.temp_dir
    }
}

impl AsRef<Path> for TestDataFs {
    fn as_ref(&self) -> &Path {
        &self.path
    }
}

impl TestDataFs {
    pub fn new_empty() -> Self {
        let temp_dir = TempDir::new().unwrap();
        let subdirfs = SubDirFs::new(temp_dir.path()).unwrap();
        Self {
            path: temp_dir.path().to_path_buf(),
            temp_dir,
            subdirfs,
        }
    }

    /// Create a TestDataFs from a path relative to the testdata directory.
    ///
    /// When using Bazel, the sandbox contains symbolic links to the original
    /// testdata directory. That doesn't work with SubDirFs since the symbolic
    /// links point to location outside the sandbox. To have our tests work with
    /// Bazel without complicating the Bazel setup, we create a temporary
    /// directory and copy the testdata directory into it, and convert any
    /// absolute symbolic links to regular files.
    ///
    /// In the case of Cargo, the copying is not needed, but we do it avoid
    /// unncessary differences between Cargo and Bazel.
    pub fn new<P: AsRef<Path>>(path: P) -> Self {
        let path = testdata_path().join(path);

        // For a single file, just use the original file with testdata_path(),
        // doesn't make sense to create a SubDirFs.
        if !path.is_dir() {
            panic!("path is not a directory: {:?}", path);
        }

        let temp_dir = TempDir::new().unwrap();
        let dest = temp_dir.path();

        dircpy::copy_dir(path, dest).unwrap();

        for entry in WalkDir::new(dest) {
            let entry = entry.unwrap();
            if entry.file_type().is_symlink()
                && let Ok(link_target) = std::fs::read_link(entry.path())
                && link_target.starts_with("/")
            {
                if entry.file_type().is_dir() {
                    panic!("symlink to directory: {:?}", entry.path());
                }

                let path = &entry.path();

                std::fs::remove_file(path).unwrap();

                if std::fs::symlink_metadata(&link_target)
                    .unwrap()
                    .is_symlink()
                {
                    let original_target = std::fs::read_link(&link_target).unwrap();
                    std::os::unix::fs::symlink(original_target, path).unwrap();
                } else {
                    std::fs::copy(link_target, path).unwrap();
                }
            }
        }

        let subdirfs = SubDirFs::new(dest).unwrap();

        Self {
            path: dest.to_path_buf(),
            temp_dir,
            subdirfs,
        }
    }
}
