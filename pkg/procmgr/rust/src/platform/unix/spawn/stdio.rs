// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use log::warn;
use nix::libc;
use std::fs::{File, OpenOptions};
use std::io;
use std::os::fd::AsRawFd;
use std::os::unix::fs::{FileTypeExt, MetadataExt, OpenOptionsExt};
use std::path::Path;
use std::process::Stdio;

use crate::spawn::StdioSetting;

pub(crate) fn to_command_stdio(setting: &StdioSetting, inheritable: bool) -> Stdio {
    match setting {
        StdioSetting::Null => Stdio::null(),
        StdioSetting::Inherit => {
            if inheritable {
                Stdio::inherit()
            } else {
                Stdio::null()
            }
        }
        StdioSetting::File(path) => file_to_stdio(path),
    }
}

fn file_to_stdio(path: &Path) -> Stdio {
    match open_stdio_file(path) {
        Ok(f) => f.into(),
        Err(e) => {
            warn!(
                "failed to open stdio file {}: {e}, falling back to inherit",
                path.display()
            );
            Stdio::inherit()
        }
    }
}

fn open_stdio_file(path: &Path) -> io::Result<File> {
    // O_NOFOLLOW covers only the last component. O_NONBLOCK makes a FIFO with no
    // reader fail instead of blocking, and is cleared below so the child does not
    // inherit it. O_NOCTTY keeps an accepted character device from becoming the
    // controlling terminal. std adds O_CLOEXEC.
    let file = OpenOptions::new()
        .create(true)
        .append(true)
        .custom_flags(libc::O_NOFOLLOW | libc::O_NONBLOCK | libc::O_NOCTTY)
        .open(path)?;

    let metadata = file.metadata()?;
    let file_type = metadata.file_type();
    if file_type.is_file() {
        // An extra link removed between open and fstat, or a link added after
        // fstat, is not detected.
        let nlink = metadata.nlink();
        if nlink != 1 {
            return Err(io::Error::new(
                io::ErrorKind::InvalidInput,
                format!("expected link count 1, got {nlink}"),
            ));
        }
    } else if !file_type.is_char_device() {
        return Err(io::Error::new(
            io::ErrorKind::InvalidInput,
            format!(
                "unsupported stdio file type: {}",
                stdio_file_type_name(&file_type)
            ),
        ));
    }

    clear_nonblock(&file)?;
    Ok(file)
}

fn stdio_file_type_name(file_type: &std::fs::FileType) -> &'static str {
    if file_type.is_fifo() {
        "fifo"
    } else if file_type.is_block_device() {
        "block device"
    } else {
        "unknown"
    }
}

fn clear_nonblock(file: &File) -> io::Result<()> {
    let fd = file.as_raw_fd();
    // SAFETY: `fd` belongs to `file`, which outlives both calls.
    let flags = unsafe { libc::fcntl(fd, libc::F_GETFL) };
    if flags < 0 {
        return Err(io::Error::last_os_error());
    }
    let rc = unsafe { libc::fcntl(fd, libc::F_SETFL, flags & !libc::O_NONBLOCK) };
    if rc < 0 {
        return Err(io::Error::last_os_error());
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::spawn::parse_stdio_setting;
    use crate::test_helpers;
    use std::io::Write;
    use std::os::unix::ffi::OsStrExt;
    use std::sync::mpsc;
    use std::time::Duration;

    fn command_stdio(yaml: &str) -> Stdio {
        to_command_stdio(&parse_stdio_setting(yaml), true)
    }

    fn c_string(path: &Path) -> std::ffi::CString {
        std::ffi::CString::new(path.as_os_str().as_bytes()).unwrap()
    }

    fn make_fifo(path: &Path) {
        let c_path = c_string(path);
        // SAFETY: `c_path` is a NUL-terminated path and outlives the call.
        let rc = unsafe { libc::mkfifo(c_path.as_ptr(), 0o600) };
        assert_eq!(rc, 0, "mkfifo: {}", io::Error::last_os_error());
    }

    fn assert_append_and_blocking(file: &File) {
        // SAFETY: `fd` belongs to `file`, which outlives the call.
        let flags = unsafe { libc::fcntl(file.as_raw_fd(), libc::F_GETFL) };
        assert!(flags >= 0, "{}", io::Error::last_os_error());
        assert_eq!(
            flags & libc::O_NONBLOCK,
            0,
            "O_NONBLOCK still set: {flags:#x}"
        );
        assert_ne!(flags & libc::O_APPEND, 0, "O_APPEND missing: {flags:#x}");
    }

    #[test]
    fn inherit_spawns() {
        let (cmd, args) = test_helpers::true_cmd();
        let status = std::process::Command::new(cmd)
            .args(&args)
            .stdout(command_stdio("inherit"))
            .stderr(command_stdio("inherit"))
            .status()
            .unwrap();
        assert!(status.success());
    }

    #[test]
    fn empty_string_matches_inherit() {
        let (cmd, args) = test_helpers::true_cmd();
        let status = std::process::Command::new(cmd)
            .args(&args)
            .stdout(command_stdio(""))
            .status()
            .unwrap();
        assert!(status.success());
    }

    #[test]
    fn inherit_without_console_maps_to_null() {
        let stdio = to_command_stdio(&StdioSetting::Inherit, false);
        let (cmd, args) = test_helpers::true_cmd();
        let status = std::process::Command::new(cmd)
            .args(&args)
            .stdout(stdio)
            .status()
            .unwrap();
        assert!(status.success());
    }

    #[test]
    fn null_discards_child_stdout() {
        let (sh, flag) = test_helpers::shell_cmd();
        let out = std::process::Command::new(sh)
            .arg(flag)
            .arg("echo hello")
            .stdout(command_stdio("null"))
            .output()
            .unwrap();
        assert!(
            out.stdout.is_empty(),
            "stdout should be discarded, got {:?}",
            String::from_utf8_lossy(&out.stdout)
        );
    }

    #[test]
    fn writable_path_redirect() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("pmgr_stdio_redirect.log");
        let path_str = path.to_str().unwrap();
        let (sh, flag) = test_helpers::shell_cmd();
        let status = std::process::Command::new(sh)
            .arg(flag)
            .arg("echo fileline")
            .stdout(command_stdio(path_str))
            .status()
            .unwrap();
        assert!(status.success());
        let contents = std::fs::read_to_string(&path).unwrap();
        assert!(
            contents.contains("fileline"),
            "expected fileline in log, got {contents:?}"
        );
    }

    #[test]
    fn path_appends_on_respawn() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("pmgr_stdio_append.log");
        let path_str = path.to_str().unwrap();
        let (sh, flag) = test_helpers::shell_cmd();
        for msg in ["first", "second"] {
            let status = std::process::Command::new(sh)
                .arg(flag)
                .arg(format!("echo {msg}"))
                .stdout(command_stdio(path_str))
                .status()
                .unwrap();
            assert!(status.success());
        }
        let contents = std::fs::read_to_string(&path).unwrap();
        assert!(contents.contains("first"), "got {contents:?}");
        assert!(contents.contains("second"), "got {contents:?}");
    }

    #[test]
    fn unopenable_path_falls_back_to_inherit() {
        let (sh, flag) = test_helpers::shell_cmd();
        let out = std::process::Command::new(sh)
            .arg(flag)
            .arg("echo fallback_ok")
            .stdout(command_stdio("/nonexistent_dir_pmgr_stdio/out.log"))
            .output()
            .unwrap();
        assert!(
            out.status.success(),
            "spawn with unopenable stdout path should still succeed (falls back to inherit)"
        );
    }

    #[test]
    fn new_file_is_created_and_appends() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("out.log");
        {
            let mut file = open_stdio_file(&path).unwrap();
            file.write_all(b"first\n").unwrap();
        }
        {
            let mut file = open_stdio_file(&path).unwrap();
            file.write_all(b"second\n").unwrap();
        }
        assert_eq!(std::fs::read_to_string(&path).unwrap(), "first\nsecond\n");
    }

    #[test]
    fn symlink_last_component_is_rejected() {
        let dir = tempfile::tempdir().unwrap();
        let target = dir.path().join("target.log");
        std::fs::write(&target, "original").unwrap();
        let link = dir.path().join("link.log");
        std::os::unix::fs::symlink(&target, &link).unwrap();
        let err = open_stdio_file(&link).unwrap_err();
        assert_eq!(err.raw_os_error(), Some(libc::ELOOP), "{err}");
        assert_eq!(std::fs::read_to_string(&target).unwrap(), "original");
    }

    #[test]
    fn dangling_symlink_is_rejected_and_creates_nothing() {
        let dir = tempfile::tempdir().unwrap();
        let missing = dir.path().join("missing.log");
        let link = dir.path().join("link.log");
        std::os::unix::fs::symlink(&missing, &link).unwrap();
        let err = open_stdio_file(&link).unwrap_err();
        assert_eq!(err.raw_os_error(), Some(libc::ELOOP), "{err}");
        assert!(!missing.exists());
    }

    #[test]
    fn hard_link_is_rejected() {
        let dir = tempfile::tempdir().unwrap();
        let original = dir.path().join("original.log");
        std::fs::write(&original, "original").unwrap();
        let link = dir.path().join("link.log");
        std::fs::hard_link(&original, &link).unwrap();
        let err = open_stdio_file(&link).unwrap_err();
        assert_eq!(err.kind(), io::ErrorKind::InvalidInput);
        assert!(
            err.to_string().contains("expected link count 1, got 2"),
            "{err}"
        );
        assert_eq!(std::fs::read_to_string(&original).unwrap(), "original");
        assert_eq!(std::fs::read_to_string(&link).unwrap(), "original");
    }

    #[test]
    fn fifo_without_reader_returns_promptly() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("stdio.fifo");
        make_fifo(&path);

        let (tx, rx) = mpsc::channel();
        let opened = path.clone();
        // Do not join: a regression that blocks in open must fail this test
        // instead of hanging the suite.
        std::thread::spawn(move || {
            let result = open_stdio_file(&opened);
            let _ = tx.send(result);
        });
        let result = rx
            .recv_timeout(Duration::from_secs(5))
            .expect("open_stdio_file blocked on a FIFO with no reader");
        let err = result.unwrap_err();
        assert_eq!(err.raw_os_error(), Some(libc::ENXIO), "{err}");
    }

    #[test]
    fn fifo_with_reader_is_rejected() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("stdio.fifo");
        make_fifo(&path);
        let _reader = OpenOptions::new()
            .read(true)
            .custom_flags(libc::O_NONBLOCK)
            .open(&path)
            .unwrap();
        let err = open_stdio_file(&path).unwrap_err();
        assert_eq!(err.kind(), io::ErrorKind::InvalidInput);
        assert!(err.to_string().contains("fifo"), "{err}");
    }

    #[test]
    fn unix_socket_is_rejected() {
        // sockaddr_un.sun_path holds about 100 bytes, so a long TMPDIR makes
        // bind fail before the test reaches open_stdio_file.
        let dir = tempfile::Builder::new().tempdir_in("/tmp").unwrap();
        let path = dir.path().join("stdio.sock");
        assert!(
            path.as_os_str().len() < 100,
            "socket path is too long for bind: {}",
            path.display()
        );
        let _listener = std::os::unix::net::UnixListener::bind(&path).unwrap();
        assert!(open_stdio_file(&path).is_err());
        assert!(path.exists());
    }

    #[test]
    fn directory_is_rejected() {
        let dir = tempfile::tempdir().unwrap();
        let err = open_stdio_file(dir.path()).unwrap_err();
        assert_eq!(err.raw_os_error(), Some(libc::EISDIR), "{err}");
    }

    #[test]
    fn dev_null_opens_and_fd_flags_are_append_blocking() {
        let dev_null = open_stdio_file(Path::new("/dev/null")).unwrap();
        assert_append_and_blocking(&dev_null);

        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("out.log");
        let file = open_stdio_file(&path).unwrap();
        assert_append_and_blocking(&file);
    }

    #[test]
    fn parent_directory_symlink_opens() {
        let dir = tempfile::tempdir().unwrap();
        let real = dir.path().join("real");
        std::fs::create_dir(&real).unwrap();
        let link = dir.path().join("link");
        std::os::unix::fs::symlink(&real, &link).unwrap();
        let path = link.join("out.log");
        let file = open_stdio_file(&path).unwrap();
        assert_append_and_blocking(&file);
        assert!(real.join("out.log").is_file());
    }

    #[test]
    fn symlink_stdio_falls_back_to_inherit() {
        const CHILD_ENV: &str = "PROCMGR_STDIO_FALLBACK_CHILD";
        if std::env::var_os(CHILD_ENV).is_some() {
            symlink_stdio_fallback_child();
            return;
        }

        // The child inherits this process's stdout, so the marker must reach the parent.
        let out = std::process::Command::new(std::env::current_exe().unwrap())
            .args([
                "--exact",
                "platform::unix::spawn::stdio::tests::symlink_stdio_falls_back_to_inherit",
                "--nocapture",
            ])
            .env(CHILD_ENV, "1")
            .output()
            .unwrap();
        assert!(
            out.status.success(),
            "{}",
            String::from_utf8_lossy(&out.stderr)
        );
        let stdout = String::from_utf8_lossy(&out.stdout);
        assert!(
            stdout.contains("fallback_marker"),
            "inherited stdout did not carry the marker: {stdout}"
        );
    }

    fn symlink_stdio_fallback_child() {
        let dir = tempfile::tempdir().unwrap();
        let target = dir.path().join("target.log");
        std::fs::write(&target, "original").unwrap();
        let link = dir.path().join("link.log");
        std::os::unix::fs::symlink(&target, &link).unwrap();
        let (sh, flag) = test_helpers::shell_cmd();
        let status = std::process::Command::new(sh)
            .arg(flag)
            .arg("echo fallback_marker")
            .stdout(command_stdio(link.to_str().unwrap()))
            .status()
            .unwrap();
        assert!(status.success());
        assert_eq!(std::fs::read_to_string(&target).unwrap(), "original");
    }
}
