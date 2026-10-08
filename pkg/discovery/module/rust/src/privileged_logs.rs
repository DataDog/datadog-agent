// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! Opens log files on behalf of the core agent for the privileged logs module,
//! in system-probe-lite directly and in system-probe through `ffi.rs`.

use std::ffi::OsStr;
use std::fs::{self, File};
use std::io::{self, IoSlice};
use std::os::fd::{AsFd, AsRawFd, OwnedFd};
use std::os::unix::fs::FileExt;
use std::path::{Component, Path, PathBuf};

use anyhow::{Context, Result, bail};
use nix::fcntl::{AT_FDCWD, OFlag, openat};
use nix::sys::socket::{ControlMessage, MsgFlags, sendmsg};
use nix::sys::stat::Mode;
use serde::Deserialize;
use tokio::io::Interest;
use tokio::net::UnixStream;

/// Body of `POST /privileged_logs/open`, as sent by pkg/privileged-logs/client.
#[derive(Deserialize, Debug)]
pub struct OpenFileRequest {
    pub path: String,
    /// Reject symlinks in every path component instead of resolving them.
    #[serde(default)]
    pub no_follow: bool,
}

/// Files under this directory are allowed whatever their name.
const ALLOWED_PREFIX: &str = "/var/log";

/// Opens the requested file if it is a log file: a `.log` file, a file under
/// /var/log, or a file with an ancestor directory named `logs`. It must also be
/// a regular file that starts with text.
pub fn open_log_file(req: &OpenFileRequest) -> Result<File> {
    open_allowed(req, Path::new(ALLOWED_PREFIX), || {})
}

/// `before_open` runs between path resolution and opening, so that tests can
/// swap in symlinks there.
fn open_allowed(
    req: &OpenFileRequest,
    allowed_prefix: &Path,
    before_open: impl FnOnce(),
) -> Result<File> {
    let path = req.path.as_str();
    if path.is_empty() {
        bail!("empty file path provided");
    }
    if !Path::new(path).is_absolute() {
        bail!("relative path not allowed: {path}");
    }
    let resolved = if req.no_follow {
        PathBuf::from(path)
    } else {
        fs::canonicalize(path).with_context(|| format!("failed to resolve path {path}"))?
    };
    before_open();
    if !is_allowed(&resolved, allowed_prefix) {
        bail!("non-log file not allowed: {}", resolved.display());
    }
    // Also rejects symlinks swapped in after canonicalize().
    let file = open_without_symlinks(&resolved)
        .with_context(|| format!("failed to open path {}", resolved.display()))?;
    if !file.metadata()?.is_file() {
        bail!("not a regular file: {}", resolved.display());
    }
    if !starts_with_text(&file) {
        bail!("not a text file: {}", resolved.display());
    }
    Ok(file)
}

fn is_allowed(path: &Path, allowed_prefix: &Path) -> bool {
    path.extension()
        .is_some_and(|ext| ext.eq_ignore_ascii_case("log"))
        || path.starts_with(allowed_prefix)
        || path
            .parent()
            .is_some_and(|dir| dir.iter().any(|name| name.eq_ignore_ascii_case("logs")))
}

/// Opens an absolute path without following symlinks in any component, like
/// common.OpenPathWithoutSymlinks in Go. `..` is rejected because, unlike Go's
/// filepath.Clean, `Path` keeps it, and it would escape the checked directory.
fn open_without_symlinks(path: &Path) -> Result<File> {
    let mut names = Vec::new();
    for component in path.components() {
        match component {
            Component::RootDir => {}
            Component::Normal(name) => names.push(name),
            _ => bail!("unexpected component in {}", path.display()),
        }
    }
    let Some((file_name, dir_names)) = names.split_last() else {
        bail!("no file name in {}", path.display());
    };

    // O_PATH only needs search permission on directories, like opening the
    // whole path at once.
    let nofollow = OFlag::O_NOFOLLOW | OFlag::O_CLOEXEC;
    let dir_flags = nofollow | OFlag::O_PATH | OFlag::O_DIRECTORY;
    let mut dir =
        open_at(AT_FDCWD, OsStr::new("/"), dir_flags).context("failed to open root directory")?;
    for name in dir_names {
        dir = open_at(&dir, name, dir_flags)
            .with_context(|| format!("failed to open directory component {}", name.display()))?;
    }
    let file = open_at(&dir, file_name, nofollow | OFlag::O_RDONLY)
        .with_context(|| format!("failed to open file {}", file_name.display()))?;
    Ok(file.into())
}

/// openat(2), with the system's error messages rather than nix's.
fn open_at(dir: impl AsFd, name: &OsStr, flags: OFlag) -> io::Result<OwnedFd> {
    Ok(openat(dir, name, flags, Mode::empty())?)
}

/// Whether the file starts with UTF-8 text. A multi-byte character cut at the
/// end of the sample is fine, and an empty file counts as text.
fn starts_with_text(file: &File) -> bool {
    let mut sample = [0u8; 128];
    let Ok(len) = file.read_at(&mut sample, 0) else {
        return false;
    };
    match std::str::from_utf8(sample.get(..len).unwrap_or_default()) {
        Ok(_) => true,
        Err(e) => e.error_len().is_none(),
    }
}

/// Sends the file descriptor to the client as SCM_RIGHTS, with the payload
/// that pkg/privileged-logs/client ignores.
pub async fn send_fd(stream: &UnixStream, file: &File) -> io::Result<()> {
    let payload = [IoSlice::new(br#"{"success":true}"#)];
    let fds = [file.as_raw_fd()];
    let rights = [ControlMessage::ScmRights(&fds)];
    let flags = MsgFlags::MSG_NOSIGNAL;
    stream
        .async_io(Interest::WRITABLE, || {
            sendmsg::<()>(stream.as_raw_fd(), &payload, &rights, flags, None)
                .map_err(io::Error::from)
        })
        .await?;
    Ok(())
}

#[cfg(test)]
#[allow(clippy::panic)] // Tests are allowed to use panic for test failures
mod tests {
    use super::*;
    use std::os::unix::fs::symlink;
    use tempfile::TempDir;

    fn request(path: &Path, no_follow: bool) -> OpenFileRequest {
        OpenFileRequest {
            path: path.to_string_lossy().into_owned(),
            no_follow,
        }
    }

    fn open(path: &Path, allowed_prefix: &Path) -> Result<File> {
        open_allowed(&request(path, false), allowed_prefix, || {})
    }

    fn write(path: &Path, content: &[u8]) -> PathBuf {
        fs::write(path, content).unwrap_or_else(|e| panic!("write {}: {e}", path.display()));
        path.to_path_buf()
    }

    fn mkdir(path: &Path) -> PathBuf {
        fs::create_dir_all(path).unwrap_or_else(|e| panic!("mkdir {}: {e}", path.display()));
        path.to_path_buf()
    }

    fn link(target: &Path, path: &Path) -> PathBuf {
        symlink(target, path).unwrap_or_else(|e| panic!("symlink {}: {e}", path.display()));
        path.to_path_buf()
    }

    /// 127 bytes of text ending with the first byte of a two-byte character.
    fn cut_character() -> Vec<u8> {
        let mut content = "é".repeat(64).into_bytes();
        content.pop();
        content
    }

    /// dir/sub/parent/sub/t.log, where dir/sub/parent links back to dir.
    fn through_parent(dir: &Path) -> PathBuf {
        write(&mkdir(&dir.join("sub")).join("t.log"), b"content");
        link(dir, &dir.join("sub/parent")).join("sub/t.log")
    }

    fn is_eloop(err: &anyhow::Error) -> bool {
        err.root_cause()
            .downcast_ref::<io::Error>()
            .is_some_and(|e| e.raw_os_error() == Some(nix::libc::ELOOP))
    }

    #[test]
    fn test_is_allowed() {
        for (path, allowed_prefix, expected) in [
            // .log extension, case-insensitive
            ("/etc/application.log", "/var/log", true),
            ("/etc/application.Log", "/var/log", true),
            ("/opt/app/data/debug.log", "/var/log", true),
            // Ancestor directory named logs, case-insensitive
            ("/databricks/driver/logs/stdout", "/var/log", true),
            ("/var/Logs/app.txt", "/var/log", true),
            ("/logs/app/subdir/debug.txt", "/var/log", true),
            ("/logs/file.txt", "/var/log", true),
            ("/logs/app/logs/debug.txt", "/var/log", true),
            // Allowed prefix
            ("/var/log/syslog", "/var/log", true),
            ("/var/log/apache2/error.txt", "/var/log", true),
            ("/tmp/testfile", "/tmp", true),
            ("/opt/custom/app.txt", "/opt/custom/", true),
            ("/opt/app.txt", "", true),
            // Not allowed
            ("/etc/passwd", "/var/log", false),
            ("/etc/logserver.conf", "/var/log", false),
            ("/var/logstash/data.txt", "/var/log", false),
            ("/var/syslogs/data.txt", "/var/log", false),
            ("/var/log/data.txt", "/tmp", false),
            ("/var/log2/file.txt", "/var/log", false),
            ("/home/user/documents/file.txt", "/var/log", false),
            ("/opt/logs", "/var/log", false),
        ] {
            assert_eq!(
                is_allowed(Path::new(path), Path::new(allowed_prefix)),
                expected,
                "is_allowed({path:?}, {allowed_prefix:?})"
            );
        }
    }

    #[test]
    fn test_validation_errors() {
        for (path, expected_error) in [
            ("", "empty file path provided"),
            (
                "relative/path.log",
                "relative path not allowed: relative/path.log",
            ),
            (
                "./relative/path.log",
                "relative path not allowed: ./relative/path.log",
            ),
            (
                "../relative/path.log",
                "relative path not allowed: ../relative/path.log",
            ),
            ("/etc/passwd", "non-log file not allowed: /etc/passwd"),
            // Allowed but missing files fail to resolve.
            (
                "/var/log/missing",
                "failed to resolve path /var/log/missing",
            ),
            (
                "/etc/missing.LOG",
                "failed to resolve path /etc/missing.LOG",
            ),
            (
                "/missing/logs/stdout",
                "failed to resolve path /missing/logs/stdout",
            ),
        ] {
            let err = open(Path::new(path), Path::new(ALLOWED_PREFIX))
                .err()
                .unwrap_or_else(|| panic!("{path:?} should fail"));
            assert!(
                format!("{err:#}").contains(expected_error),
                "{path:?}: {err:#}"
            );
        }
    }

    #[test]
    fn test_open_real_files() {
        let tmp = TempDir::new().unwrap_or_else(|e| panic!("{e}"));
        let dir = tmp.path();
        let var_log = Path::new(ALLOWED_PREFIX);
        let cases: Vec<(&str, PathBuf, &Path, Option<&str>)> = vec![
            (
                "log file",
                write(&dir.join("test.log"), b"content"),
                var_log,
                None,
            ),
            (
                "file in prefix",
                write(&dir.join("testfile"), b"content"),
                dir,
                None,
            ),
            (
                "directory",
                mkdir(&dir.join("testdir")),
                dir,
                Some("not a regular file"),
            ),
            (
                "binary",
                write(&dir.join("binary.log"), &[0xFF, 0xFE, 0, 1]),
                dir,
                Some("not a text file"),
            ),
            (
                "mixed",
                write(&dir.join("mixed.log"), b"text\xFF\xFE"),
                dir,
                Some("not a text file"),
            ),
            ("empty", write(&dir.join("empty.log"), b""), dir, None),
            (
                "cut character",
                write(&dir.join("cut.log"), &cut_character()),
                dir,
                None,
            ),
            (
                "in Logs",
                write(&mkdir(&dir.join("a/Logs")).join("app.txt"), b"content"),
                var_log,
                None,
            ),
            (
                "symlink to log",
                link(
                    &write(&dir.join("target.log"), b"content"),
                    &dir.join("link.log"),
                ),
                dir,
                None,
            ),
            (
                "symlink in prefix",
                link(&write(&dir.join("target"), b"content"), &dir.join("link")),
                dir,
                None,
            ),
            (
                "symlink to non-log",
                link(&dir.join("target"), &dir.join("fake.log")),
                var_log,
                Some("non-log file not allowed"),
            ),
            (
                "symlink to directory",
                link(&dir.join("testdir"), &dir.join("dir.log")),
                dir,
                Some("not a regular file"),
            ),
            (
                "broken symlink",
                link(Path::new("/nonexistent"), &dir.join("broken.log")),
                dir,
                Some("failed to resolve path"),
            ),
            (
                "relative symlink in logs",
                link(Path::new("app.txt"), &dir.join("a/Logs/link.txt")),
                var_log,
                None,
            ),
            ("symlink through parent", through_parent(dir), dir, None),
        ];
        for (name, path, allowed_prefix, expected_error) in cases {
            match (open(&path, allowed_prefix), expected_error) {
                (Ok(_), None) => {}
                (Err(err), Some(expected)) => {
                    let resolved = fs::canonicalize(&path).unwrap_or(path);
                    let err = format!("{err:#}");
                    assert!(err.contains(expected), "{name}: {err}");
                    assert!(err.contains(&*resolved.to_string_lossy()), "{name}: {err}");
                }
                (result, _) => panic!("{name}: unexpected {:?}", result.map(|_| ())),
            }
        }
    }

    #[test]
    fn test_no_follow_rejects_symlinks() {
        let tmp = TempDir::new().unwrap_or_else(|e| panic!("{e}"));
        let dir = tmp.path();
        let target = write(&dir.join("secret.log"), b"secret");
        let file_link = link(&target, &dir.join("app.log"));
        let dir_link = link(dir, &dir.join("dir"));

        let err = open_allowed(&request(&file_link, true), dir, || {})
            .err()
            .unwrap_or_else(|| panic!("symlinked file should fail"));
        assert!(is_eloop(&err), "{err:#}");
        assert!(open_allowed(&request(&dir_link.join("secret.log"), true), dir, || {}).is_err());
        assert!(open_allowed(&request(&dir.join("dir/../secret.log"), true), dir, || {}).is_err());

        assert!(open_allowed(&request(&target, true), dir, || {}).is_ok());
        assert!(open_allowed(&request(&file_link, false), dir, || {}).is_ok());
    }

    #[test]
    fn test_toctou_directory_symlink() {
        let tmp = TempDir::new().unwrap_or_else(|e| panic!("{e}"));
        let log_dir = mkdir(&tmp.path().join("var/log"));
        let log_file = write(&log_dir.join("shadow"), b"syslog");
        let etc = mkdir(&tmp.path().join("etc"));
        write(&etc.join("shadow"), b"sensitive");

        // Replace the allowed directory with a symlink after validation.
        let err = open_allowed(&request(&log_file, false), &log_dir, || {
            fs::remove_dir_all(&log_dir).unwrap_or_else(|e| panic!("{e}"));
            link(&etc, &log_dir);
        })
        .err()
        .unwrap_or_else(|| panic!("swapped directory should fail"));
        assert!(
            format!("{err:#}").contains("failed to open directory component log"),
            "{err:#}"
        );
    }

    #[test]
    fn test_toctou_file_symlink() {
        let tmp = TempDir::new().unwrap_or_else(|e| panic!("{e}"));
        let log_file = write(&tmp.path().join("foo.log"), b"log");
        let other = write(&tmp.path().join("foo.nonlog"), b"non-log");

        // Replace the log file with a symlink after validation.
        let err = open_allowed(
            &request(&log_file, false),
            Path::new(ALLOWED_PREFIX),
            || {
                fs::remove_file(&log_file).unwrap_or_else(|e| panic!("{e}"));
                link(&other, &log_file);
            },
        )
        .err()
        .unwrap_or_else(|| panic!("swapped file should fail"));
        assert!(is_eloop(&err), "{err:#}");
    }
}
