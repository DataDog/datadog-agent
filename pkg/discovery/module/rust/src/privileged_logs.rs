// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! Port of pkg/privileged-logs/module/validate.go, which full system-probe still uses.

use std::fs::{self, File};
use std::io::{self, IoSlice};
use std::os::{fd::AsRawFd, unix::fs::FileExt};
use std::path::{Component, Path};

use anyhow::{Context, Result, bail};
use nix::fcntl::{OFlag, open, openat};
use nix::sys::socket::{ControlMessage::ScmRights, MsgFlags, sendmsg};
use nix::sys::stat::Mode;
use tokio::{io::Interest, net::UnixStream};

fn is_allowed(path: &str) -> bool {
    let dir = path.rsplit_once('/').map_or("", |(dir, _)| dir);
    path.to_ascii_lowercase().ends_with(".log")
        || path.starts_with("/var/log/")
        || dir.split('/').any(|part| part.eq_ignore_ascii_case("logs"))
}

/// Opens the path without following symlinks. Rejects `..`, which would escape
/// the directory checked by `is_allowed`.
fn open_no_symlinks(path: &Path) -> Result<File> {
    let mut parts = Vec::new();
    for component in path.components() {
        match component {
            Component::RootDir => {}
            Component::Normal(part) => parts.push(part),
            _ => bail!("invalid path component"),
        }
    }
    let Some((name, dirs)) = parts.split_last() else {
        bail!("no file name");
    };
    // O_PATH only needs search permission on directories, like a plain open.
    let dir_flags = OFlag::O_PATH | OFlag::O_NOFOLLOW | OFlag::O_DIRECTORY | OFlag::O_CLOEXEC;
    let mut dir = open("/", dir_flags, Mode::empty())?;
    for part in dirs {
        dir = openat(&dir, *part, dir_flags, Mode::empty())?;
    }
    let file_flags = OFlag::O_RDONLY | OFlag::O_NOFOLLOW | OFlag::O_CLOEXEC;
    Ok(openat(&dir, *name, file_flags, Mode::empty())?.into())
}

/// Opens the path if it is an allowed log file.
pub fn validate_and_open(path: &str, no_follow: bool) -> Result<File> {
    if !path.starts_with('/') {
        bail!("relative path not allowed: {path}");
    }
    let resolved = match no_follow {
        true => path.into(),
        false => fs::canonicalize(path).with_context(|| format!("failed to resolve {path}"))?,
    };
    let resolved = resolved.to_string_lossy();
    if !is_allowed(&resolved) {
        bail!("non-log file not allowed: {resolved}");
    }
    let file = open_no_symlinks(Path::new(resolved.as_ref()))
        .with_context(|| format!("failed to open path {resolved}"))?;
    if !file.metadata()?.is_file() {
        bail!("not a regular file: {resolved}");
    }
    let mut buf = [0u8; 128]; // Zero-padded, like Go's utf8.Valid(buf).
    if file.read_at(&mut buf, 0).is_err() || std::str::from_utf8(&buf).is_err() {
        bail!("not a text file: {resolved}");
    }
    Ok(file)
}

/// Sends the file descriptor to the client as SCM_RIGHTS.
pub async fn send_fd(stream: &UnixStream, file: &File) -> io::Result<usize> {
    let iov = [IoSlice::new(br#"{"success":true}"#)];
    let fds = [file.as_raw_fd()];
    let cmsgs = [ScmRights(&fds)];
    let (fd, flags) = (stream.as_raw_fd(), MsgFlags::MSG_NOSIGNAL);
    let send = || Ok(sendmsg::<()>(fd, &iov, &cmsgs, flags, None)?);
    stream.async_io(Interest::WRITABLE, send).await
}

#[cfg(test)]
#[allow(clippy::panic)] // Tests are allowed to use panic for test failures
mod tests {
    use super::*;

    #[test]
    fn test_is_allowed() {
        for (path, allowed) in [
            ("/opt/APP.LOG", true),
            ("/var/log/syslog", true),
            ("/opt/Logs/sub/out.txt", true),
            ("/var/log", false),
            ("/opt/logs", false),
            ("/etc/shadow", false),
        ] {
            assert_eq!(is_allowed(path), allowed, "{path}");
        }
    }

    #[test]
    fn test_rejects_symlink_and_parent_dir() {
        let dir = tempfile::tempdir().unwrap_or_else(|e| panic!("{e}"));
        let path = |p: &str| format!("{}/{p}", dir.path().display());
        std::fs::create_dir(path("real")).unwrap_or_else(|e| panic!("{e}"));
        std::fs::write(path("real/a.log"), "ok").unwrap_or_else(|e| panic!("{e}"));
        std::os::unix::fs::symlink(path("real"), path("link")).unwrap_or_else(|e| panic!("{e}"));
        assert!(validate_and_open(&path("real/a.log"), true).is_ok());
        assert!(validate_and_open(&path("link/a.log"), true).is_err());
        assert!(validate_and_open(&path("real/../real/a.log"), true).is_err());
    }
}
