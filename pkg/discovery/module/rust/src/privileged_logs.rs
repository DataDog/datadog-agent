// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! Opens log files on behalf of the core agent for the privileged logs module,
//! with the checks of pkg/privileged-logs/module/validate.go.

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
    if !is_allowed(&resolved) {
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

fn is_allowed(path: &Path) -> bool {
    path.extension()
        .is_some_and(|ext| ext.eq_ignore_ascii_case("log"))
        || path.starts_with(ALLOWED_PREFIX)
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
