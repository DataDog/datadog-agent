// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Opens log files on behalf of the unprivileged core agent, for the privileged
//! logs module of system-probe and system-probe-lite.
//!
//! A path is allowed if it names a `.log` file, a file under `/var/log/`, or a
//! file with an ancestor directory named `logs`, all case-insensitively. The
//! file is opened without following symlinks in any component, so that the
//! checked path is the one opened, and must be a regular file starting with
//! text.

use std::ffi::OsStr;
use std::fs::{self, File};
use std::io;
use std::os::fd::{AsFd, OwnedFd};
use std::os::unix::ffi::OsStrExt;
use std::os::unix::fs::FileExt;
use std::path::Path;

use anyhow::{Context, Result, bail};
use nix::fcntl::{AT_FDCWD, FcntlArg, OFlag, fcntl, openat};
use nix::sys::stat::Mode;
use normalize_path::NormalizePath;

/// Opens `path` if it is an allowed log file. With `no_follow`, symlinks in
/// `path` are refused rather than resolved.
pub fn open_log_file(path: &Path, no_follow: bool) -> Result<File> {
    if path.as_os_str().is_empty() {
        bail!("empty file path provided");
    }
    if !path.is_absolute() {
        bail!("relative path not allowed: {}", path.display());
    }
    // Without symlinks, resolving `..` lexically gives the kernel's result.
    let path = if no_follow {
        path.normalize()
    } else {
        fs::canonicalize(path)
            .with_context(|| format!("failed to resolve path {}", path.display()))?
    };
    if !is_allowed(&path) {
        bail!("non-log file not allowed: {}", path.display());
    }

    let file = open_without_symlinks(&path)
        .with_context(|| format!("failed to open path {}", path.display()))?;
    if !file.metadata()?.is_file() {
        bail!("not a regular file: {}", path.display());
    }
    if !starts_with_text(&file) {
        bail!("not a text file: {}", path.display());
    }
    Ok(file)
}

fn is_allowed(path: &Path) -> bool {
    let bytes = path.as_os_str().as_bytes();
    let extension = bytes
        .get(bytes.len().saturating_sub(4)..)
        .unwrap_or_default();
    extension.eq_ignore_ascii_case(b".log")
        || bytes.starts_with(b"/var/log/")
        || path
            .parent()
            .is_some_and(|dir| dir.iter().any(|name| name.eq_ignore_ascii_case("logs")))
}

/// Opens a normalized absolute path for reading, one component at a time with
/// O_NOFOLLOW, so a symlink swapped in after the path was checked is refused.
/// Directories are opened with O_PATH, which only needs search permission,
/// like a regular open of the whole path. The file is opened with O_NONBLOCK,
/// cleared afterwards, so that a FIFO or a file lease can't block the open.
fn open_without_symlinks(path: &Path) -> Result<File> {
    let (Some(parent), Some(name)) = (path.parent(), path.file_name()) else {
        bail!("not a file path");
    };
    let nofollow = OFlag::O_NOFOLLOW | OFlag::O_CLOEXEC;
    let dir_flags = nofollow | OFlag::O_PATH | OFlag::O_DIRECTORY;

    let mut dir = open_at(AT_FDCWD, OsStr::new("/"), dir_flags)?;
    for component in parent.iter().skip(1) {
        dir = open_at(&dir, component, dir_flags).with_context(|| {
            format!("failed to open directory component {}", component.display())
        })?;
    }
    let file = open_at(&dir, name, nofollow | OFlag::O_RDONLY | OFlag::O_NONBLOCK)
        .with_context(|| format!("failed to open file {}", name.display()))?;
    fcntl(&file, FcntlArg::F_SETFL(OFlag::empty()))?;
    Ok(file.into())
}

/// openat(2), with the system's error messages rather than nix's.
fn open_at(dir: impl AsFd, name: &OsStr, flags: OFlag) -> io::Result<OwnedFd> {
    Ok(openat(dir, name, flags, Mode::empty())?)
}

/// Whether the file starts with UTF-8 text. An empty file counts as text, and
/// so does a full sample that ends partway through a multi-byte character.
fn starts_with_text(file: &File) -> bool {
    let mut sample = [0u8; 128];
    let Ok(len) = file.read_at(&mut sample, 0) else {
        return false;
    };
    match std::str::from_utf8(sample.get(..len).unwrap_or_default()) {
        Ok(_) => true,
        Err(e) => len == sample.len() && e.error_len().is_none(),
    }
}

#[cfg(test)]
#[allow(clippy::panic)] // Tests are allowed to use panic for test failures
mod tests {
    use super::*;
    use nix::sys::signal::{SigHandler, Signal, signal};
    use std::os::fd::AsRawFd;
    use std::os::unix::fs::PermissionsExt;
    use std::os::unix::fs::symlink;
    use std::path::PathBuf;
    use std::time::{Duration, Instant};

    /// A temporary directory to lay out files in.
    struct Fixture(tempfile::TempDir);

    impl Fixture {
        fn new() -> Self {
            Self(tempfile::tempdir().unwrap_or_else(|e| panic!("tempdir: {e}")))
        }

        fn path(&self, name: &str) -> PathBuf {
            self.0.path().join(name)
        }

        fn dir(&self, name: &str) -> PathBuf {
            let path = self.path(name);
            fs::create_dir_all(&path).unwrap_or_else(|e| panic!("mkdir {name}: {e}"));
            path
        }

        fn file(&self, name: &str, content: &[u8]) -> PathBuf {
            let path = self.path(name);
            if let Some(dir) = path.parent() {
                fs::create_dir_all(dir).unwrap_or_else(|e| panic!("mkdir for {name}: {e}"));
            }
            fs::write(&path, content).unwrap_or_else(|e| panic!("write {name}: {e}"));
            path
        }

        fn symlink(&self, name: &str, target: impl AsRef<Path>) -> PathBuf {
            let path = self.path(name);
            symlink(target, &path).unwrap_or_else(|e| panic!("symlink {name}: {e}"));
            path
        }
    }

    fn open(path: &Path, no_follow: bool) -> Result<File> {
        open_log_file(path, no_follow)
    }

    fn error(path: &Path, no_follow: bool) -> String {
        match open(path, no_follow) {
            Ok(_) => panic!("{} should not open", path.display()),
            Err(e) => format!("{e:#}"),
        }
    }

    fn os_error(path: &Path, no_follow: bool) -> Option<i32> {
        let err = open(path, no_follow).err()?;
        err.root_cause().downcast_ref::<io::Error>()?.raw_os_error()
    }

    #[test]
    fn test_is_allowed() {
        for (path, allowed) in [
            ("/etc/app.log", true),
            ("/etc/app.LoG", true),
            ("/etc/.log", true),
            ("/var/log/syslog", true),
            ("/var/log/apache2/error", true),
            ("/opt/logs/out", true),
            ("/opt/Logs/sub/out", true),
            ("/logs/out", true),
            ("/etc/passwd", false),
            ("/etc/app.log.1", false),
            ("/etc/xlog", false),
            ("/etc/logserver.conf", false),
            ("/var/log", false),
            ("/var/log2/out", false),
            ("/var/logstash/out", false),
            ("/var/syslogs/out", false),
            ("/opt/logs", false),
        ] {
            assert_eq!(is_allowed(Path::new(path)), allowed, "{path}");
        }
    }

    #[test]
    fn test_rejected_paths() {
        for (path, no_follow, expected) in [
            ("", false, "empty file path provided"),
            ("app.log", false, "relative path not allowed: app.log"),
            (
                "/etc/passwd",
                false,
                "non-log file not allowed: /etc/passwd",
            ),
            (
                "/nonexistent/app.log",
                false,
                "failed to resolve path /nonexistent/app.log",
            ),
            (
                "/var/log/../../etc/passwd",
                true,
                "non-log file not allowed: /etc/passwd",
            ),
            ("/var/log/", true, "non-log file not allowed: /var/log"),
        ] {
            let err = error(Path::new(path), no_follow);
            assert!(err.contains(expected), "{path:?}: {err}");
        }
    }

    #[test]
    fn test_content() {
        let fx = Fixture::new();
        // The 128-byte sample ends with the first byte of the last "é".
        let cut_character = format!("a{}", "é".repeat(64)).into_bytes();
        for no_follow in [false, true] {
            assert!(open(&fx.file("text.log", b"text\n"), no_follow).is_ok());
            assert!(open(&fx.file("empty.log", b""), no_follow).is_ok());
            assert!(open(&fx.file("cut.log", &cut_character), no_follow).is_ok());
            let binary = fx.file("binary.log", &[0xFF, 0xFE, 0, 1]);
            assert!(error(&binary, no_follow).contains("not a text file"));
            let mixed = fx.file("mixed.log", b"text\xFF\xFE");
            assert!(error(&mixed, no_follow).contains("not a text file"));
            let truncated = fx.file("truncated.log", b"text\xC3");
            assert!(error(&truncated, no_follow).contains("not a text file"));
            let dir = fx.dir("dir.log");
            assert!(error(&dir, no_follow).contains("not a regular file"));
        }
    }

    #[test]
    fn test_does_not_block() {
        let fx = Fixture::new();
        let fifo = fx.path("fifo.log");
        nix::unistd::mkfifo(&fifo, Mode::S_IRWXU).unwrap_or_else(|e| panic!("mkfifo: {e}"));
        assert!(error(&fifo, false).contains("not a regular file"));

        // An open that breaks a lease fails right away instead of waiting for
        // the holder, who is notified with SIGIO.
        let leased = fx.file("leased.log", b"text");
        let holder = fs::OpenOptions::new()
            .write(true)
            .open(&leased)
            .unwrap_or_else(|e| panic!("open: {e}"));
        // SAFETY: SIG_IGN is not a function and runs no code.
        unsafe { signal(Signal::SIGIO, SigHandler::SigIgn) }.unwrap_or_else(|e| panic!("{e}"));
        // SAFETY: plain fcntl(2) call on a descriptor that `holder` keeps open.
        let leased_ok = unsafe {
            nix::libc::fcntl(
                holder.as_raw_fd(),
                nix::libc::F_SETLEASE,
                nix::libc::F_WRLCK,
            )
        };
        assert_eq!(leased_ok, 0, "F_SETLEASE: {}", io::Error::last_os_error());
        let start = Instant::now();
        assert_eq!(os_error(&leased, false), Some(nix::libc::EWOULDBLOCK));
        assert!(start.elapsed() < Duration::from_secs(5));
        drop(holder);
        assert!(open(&leased, false).is_ok());
    }

    #[test]
    fn test_search_only_directory() {
        // Root would bypass the read permission check that this test is about.
        if uzers::get_effective_uid() == 0 {
            return;
        }
        let fx = Fixture::new();
        let file = fx.file("searchonly/app.log", b"text");
        let dir = fx.path("searchonly");
        fs::set_permissions(&dir, fs::Permissions::from_mode(0o111))
            .unwrap_or_else(|e| panic!("chmod: {e}"));
        let opened = open(&file, true);
        fs::set_permissions(&dir, fs::Permissions::from_mode(0o755))
            .unwrap_or_else(|e| panic!("chmod: {e}"));
        assert!(opened.is_ok(), "{:?}", opened.err());
    }

    #[test]
    fn test_follow() {
        let fx = Fixture::new();
        let target = fx.file("logs/target.txt", b"text");
        assert!(open(&fx.symlink("logs/link.txt", "target.txt"), false).is_ok());
        assert!(open(&fx.symlink("link.log", &target), false).is_ok());

        let secret = fx.file("secret", b"text");
        let err = error(&fx.symlink("secret.log", &secret), false);
        let resolved = fs::canonicalize(&secret).unwrap_or(secret);
        assert!(err.contains(&format!("non-log file not allowed: {}", resolved.display())));

        let err = error(&fx.symlink("broken.log", "/nonexistent"), false);
        assert!(err.contains("failed to resolve path"), "{err}");

        fx.symlink("linked", fx.path("logs"));
        assert!(open(&fx.path("linked/target.txt"), false).is_ok());
    }

    #[test]
    fn test_no_follow() {
        let fx = Fixture::new();
        let target = fx.file("logs/target.log", b"text");
        assert!(open(&target, true).is_ok());
        assert!(open(&fx.path("logs/sub/../target.log"), true).is_ok());

        let link = fx.symlink("logs/link.log", &target);
        assert_eq!(os_error(&link, true), Some(nix::libc::ELOOP));
        fx.symlink("linked", fx.path("logs"));
        let err = error(&fx.path("linked/target.log"), true);
        assert!(
            err.contains("failed to open directory component linked"),
            "{err}"
        );
    }
}
