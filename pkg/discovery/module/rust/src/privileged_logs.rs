// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! Port of pkg/privileged-logs/module/validate.go. The tests in
//! pkg/privileged-logs/test run against both implementations.

use std::fs::{self, File, OpenOptions};
use std::io::{self, IoSlice};
use std::os::fd::AsRawFd;
use std::os::unix::fs::{FileExt, OpenOptionsExt};
use std::path::Path;

use anyhow::{Context, Result, bail};
use nix::libc::O_PATH;
use nix::sys::socket::{ControlMessage::ScmRights, MsgFlags, sendmsg};
use tokio::{io::Interest, net::UnixStream};

fn is_allowed(path: &str) -> bool {
    let dir = path.rsplit_once('/').map_or("", |(dir, _)| dir);
    path.to_ascii_lowercase().ends_with(".log")
        || path.starts_with("/var/log/")
        || dir.split('/').any(|part| part.eq_ignore_ascii_case("logs"))
}

/// Opens the path if it is an allowed log file.
pub fn validate_and_open(path: &str, no_follow: bool) -> Result<File> {
    if !path.starts_with('/') {
        bail!("relative path not allowed: {path}");
    }
    // O_PATH resolves the path without opening the file, so devices and FIFOs
    // aren't touched. The checks apply to the path the kernel reports for this
    // fd, and the file is then reopened through the same fd: swapping a symlink
    // in afterwards changes nothing.
    let mut options = OpenOptions::new();
    options.read(true).custom_flags(O_PATH);
    let handle = options
        .open(path)
        .with_context(|| format!("failed to resolve path {path}"))?;
    let fd_path = format!("/proc/self/fd/{}", handle.as_raw_fd());
    let resolved = fs::read_link(&fd_path)?.to_string_lossy().into_owned();
    if no_follow && Path::new(&resolved) != Path::new(path) {
        bail!(
            "failed to open path {path}: resolves to {resolved}: too many levels of symbolic links"
        );
    }
    if !is_allowed(&resolved) {
        bail!("non-log file not allowed: {resolved}");
    }
    if !handle.metadata()?.is_file() {
        bail!("not a regular file: {resolved}");
    }
    let file = File::open(&fd_path)?;
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
