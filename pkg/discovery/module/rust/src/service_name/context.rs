// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use crate::fs::SubDirFs;
use std::cell::OnceCell;
use std::collections::HashMap;
use std::path::{Path, PathBuf};

/// DetectionContext provides context for service name detection
pub struct DetectionContext<'a> {
    pid: i32,
    pub envs: HashMap<String, String>,
    pub fs: &'a SubDirFs,
    /// Cached candidate working directories to avoid repeated lookups
    cached_working_dirs: OnceCell<Vec<String>>,
}

impl<'a> DetectionContext<'a> {
    pub fn new(pid: i32, envs: HashMap<String, String>, fs: &'a SubDirFs) -> Self {
        Self {
            pid,
            envs,
            fs,
            cached_working_dirs: OnceCell::new(),
        }
    }

    /// Resolves a path relative to the working directory.
    ///
    /// There are two sources of working directory: the procfs cwd and the PWD
    /// environment variable. However, we can't know which is the correct one to
    /// resolve relative paths since the working directory could have changed before
    /// or after the command line we're looking at was executed. So, we check if
    /// the path we're looking for exists in either of the working directories, and
    /// pick that as the correct one.
    ///
    /// It returns None if the path is absolute or if there are no working directories.
    pub fn resolve_working_dir_relative_path(&self, path: impl AsRef<Path>) -> Option<PathBuf> {
        // If already absolute, return as-is
        let path = path.as_ref();
        if path.is_absolute() {
            return None;
        }

        // Initialize cached working dirs if not already done
        let candidates = self.cached_working_dirs.get_or_init(|| {
            let mut candidates = Vec::new();

            // Check PWD environment variable
            if let Some(pwd) = self.envs.get("PWD")
                && !pwd.is_empty()
            {
                candidates.push(pwd.clone());
            }

            // Check procfs cwd using procfs path helper
            let procfs_cwd_path = crate::procfs::root_path()
                .join(self.pid.to_string())
                .join("cwd");
            if let Ok(cwd) = std::fs::read_link(procfs_cwd_path)
                && let Some(cwd_str) = cwd.to_str()
                && !cwd_str.is_empty()
            {
                candidates.push(cwd_str.to_string());
            }

            candidates
        });

        if candidates.is_empty() {
            return None;
        }

        if candidates.len() > 1 {
            // Check which one exists
            for cwd in candidates.iter() {
                let abs_path = PathBuf::from(cwd).join(path);
                if self.fs.symlink_metadata(&abs_path).is_ok() {
                    return Some(abs_path);
                }
            }
        }

        // If we got here, we either just have a single candidate, or multiple
        // candidates but none of the paths appear to exist. Just return the
        // absolute path of the first candidate.
        Some(PathBuf::from(candidates.first()?).join(path))
    }
}

#[cfg(test)]
#[allow(clippy::unwrap_used, clippy::expect_used)]
mod tests {
    use super::*;
    use crate::fs::SubDirFs;

    #[test]
    fn test_resolve_absolute_path() {
        let envs = HashMap::new();
        let testdata = crate::test_utils::testdata_path().join("nodejs");
        let fs = SubDirFs::new(&testdata).unwrap();
        let ctx = DetectionContext::new(0, envs, &fs);

        let result = ctx.resolve_working_dir_relative_path("/absolute/path");
        assert_eq!(result, None)
    }

    #[test]
    fn test_resolve_relative_path_with_pwd() {
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/some/dir".to_string());

        let testdata = crate::test_utils::testdata_path().join("nodejs");
        let fs = SubDirFs::new(&testdata).unwrap();
        let ctx = DetectionContext::new(0, envs, &fs);

        let result = ctx.resolve_working_dir_relative_path("relative/path");
        assert_eq!(result, Some(PathBuf::from("/some/dir/relative/path")));
    }

    #[test]
    fn test_resolve_working_dir_relative_path() {
        use scopeguard::defer;
        use std::fs;
        use std::process::Command;
        use tempfile::TempDir;

        // Create temporary directory structure
        let tmp_dir = TempDir::new().unwrap();
        let cwd_dir = tmp_dir.path().join("cwd");
        let pwd_dir = tmp_dir.path().join("pwd");
        fs::create_dir(&cwd_dir).unwrap();
        fs::create_dir(&pwd_dir).unwrap();

        // Create test files
        let pwd_txt = pwd_dir.join("pwd.txt");
        fs::File::create(&pwd_txt).unwrap();
        let cwd_txt = cwd_dir.join("cwd.txt");
        fs::File::create(&cwd_txt).unwrap();

        fs::File::create(pwd_dir.join("both.txt")).unwrap();
        fs::File::create(cwd_dir.join("both.txt")).unwrap();

        // Spawn a child process with cwd_dir as its working directory
        let mut child = Command::new("sleep")
            .arg("60")
            .current_dir(&cwd_dir)
            .spawn()
            .expect("failed to spawn child process");
        let child_pid = child.id().cast_signed();
        defer! {
            let _ = child.kill();
            let _ = child.wait();
        }

        // Test cases
        struct TestCase {
            name: &'static str,
            pid: i32,
            envs: HashMap<String, String>,
            filename: &'static str,
            want_path: PathBuf,
        }

        let tests = vec![
            TestCase {
                name: "file exists in procfs cwd",
                pid: child_pid,
                envs: HashMap::new(),
                filename: "cwd.txt",
                want_path: cwd_txt.clone(),
            },
            TestCase {
                name: "file exists in PWD env",
                pid: child_pid,
                envs: HashMap::from([("PWD".to_string(), pwd_dir.to_string_lossy().to_string())]),
                filename: "pwd.txt",
                want_path: pwd_txt.clone(),
            },
            TestCase {
                name: "file exists in both",
                pid: child_pid,
                envs: HashMap::from([("PWD".to_string(), pwd_dir.to_string_lossy().to_string())]),
                filename: "both.txt",
                want_path: pwd_dir.join("both.txt"),
            },
            TestCase {
                name: "no working dir candidates",
                pid: 0,
                envs: HashMap::new(),
                filename: "nonexistent.txt",
                want_path: PathBuf::from("nonexistent.txt"),
            },
            TestCase {
                name: "file doesn't exist but has working dir",
                pid: child_pid,
                envs: HashMap::new(),
                filename: "nonexistent.txt",
                want_path: cwd_dir.join("nonexistent.txt"),
            },
            TestCase {
                name: "file doesn't exist but has PWD",
                pid: child_pid,
                envs: HashMap::from([("PWD".to_string(), pwd_dir.to_string_lossy().to_string())]),
                filename: "nonexistent.txt",
                want_path: pwd_dir.join("nonexistent.txt"),
            },
        ];

        for tt in tests {
            // Use RealFs for these tests since we're working with actual filesystem
            let fs = SubDirFs::new("/").unwrap();
            let ctx = DetectionContext::new(tt.pid, tt.envs, &fs);

            let got_path = ctx.resolve_working_dir_relative_path(tt.filename);

            // For the "no working dir candidates" case, we expect None
            if tt.name == "no working dir candidates" {
                assert_eq!(
                    got_path, None,
                    "test case '{}' failed: expected None, got {:?}",
                    tt.name, got_path
                );
            } else {
                assert_eq!(
                    got_path,
                    Some(tt.want_path.clone()),
                    "test case '{}' failed",
                    tt.name
                );
            }
        }
    }
}
