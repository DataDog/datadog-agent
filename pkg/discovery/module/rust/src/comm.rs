// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use phf::phf_set;
use std::fs::File;
use std::io::Read;

use crate::procfs;

/// Maximum length of a comm name in /proc/<pid>/comm (excluding newline).
const MAX_COMM_LEN: usize = 15;

/// Exact command names to ignore
static IGNORE_COMMS: phf::Set<&'static [u8]> = phf_set! {
    b"chronyd",
    b"cilium-agent",
    b"containerd",
    b"dhclient",
    b"dockerd",
    b"kubelet",
    b"livenessprobe",
    b"local-volume-pr",
    b"sshd",
    b"systemd",
};

/// Family prefixes (before the first `-`) to ignore.
static IGNORE_FAMILIES: phf::Set<&'static [u8]> = phf_set! {
    b"systemd",
    b"datadog",
    b"containerd",
    b"docker",
};

/// Returns true if the process with the given PID should be ignored based on
/// its comm name. Returns true on read error (process likely gone).
pub fn should_ignore_comm(pid: i32) -> bool {
    let comm_path = procfs::root_path().join(pid.to_string()).join("comm");

    let Ok(mut file) = File::open(&comm_path) else {
        return true;
    };

    let mut buf = [0u8; MAX_COMM_LEN];
    let Ok(n) = file.read(&mut buf) else {
        return true;
    };

    let Some(comm_bytes) = buf.get(..n) else {
        return false;
    };

    should_ignore_comm_bytes(comm_bytes)
}

/// Returns true if the given comm bytes (as read from /proc/<pid>/comm)
/// match an ignored command name or family prefix.
fn should_ignore_comm_bytes(comm_bytes: &[u8]) -> bool {
    if let Some(dash_pos) = comm_bytes.iter().position(|&b| b == b'-')
        && dash_pos > 0
        && let Some(family_bytes) = comm_bytes.get(..dash_pos)
        && IGNORE_FAMILIES.contains(family_bytes)
    {
        return true;
    }

    let comm = comm_bytes.strip_suffix(b"\n").unwrap_or(comm_bytes);
    IGNORE_COMMS.contains(comm)
}

#[cfg(test)]
#[allow(clippy::unwrap_used, clippy::expect_used)]
mod tests {
    use super::*;

    #[test]
    fn test_ignore_comms_lengths() {
        for entry in IGNORE_COMMS.iter() {
            assert!(
                entry.len() <= MAX_COMM_LEN,
                "Ignore comm entry {entry:?} exceeds max comm length {MAX_COMM_LEN}"
            );
        }
    }

    #[test]
    fn test_ignore_families_lengths() {
        for entry in IGNORE_FAMILIES.iter() {
            assert!(
                entry.len() <= MAX_COMM_LEN,
                "Ignore family entry {entry:?} exceeds max comm length {MAX_COMM_LEN}"
            );
        }
    }

    #[test]
    fn test_exact_match_with_newline() {
        assert!(should_ignore_comm_bytes(b"sshd\n"));
        assert!(should_ignore_comm_bytes(b"kubelet\n"));
        assert!(should_ignore_comm_bytes(b"chronyd\n"));
        assert!(should_ignore_comm_bytes(b"dockerd\n"));
    }

    #[test]
    fn test_exact_match_without_newline() {
        // When the buffer is exactly full, there's no trailing newline.
        assert!(should_ignore_comm_bytes(b"containerd"));
        assert!(should_ignore_comm_bytes(b"sshd"));
    }

    #[test]
    fn test_family_prefix_match() {
        assert!(should_ignore_comm_bytes(b"docker-proxy\n"));
        assert!(should_ignore_comm_bytes(b"datadog-agent\n"));
        // "systemd-resolve" is 15 bytes, no newline (buffer full)
        assert!(should_ignore_comm_bytes(b"systemd-resolve"));
        // containerd-shim\n is 16 bytes; after reading 15 into the buffer
        // we'd get "containerd-shim" which matches the containerd family.
        assert!(should_ignore_comm_bytes(b"containerd-shim"));
    }

    #[test]
    fn test_non_ignored_commands() {
        assert!(!should_ignore_comm_bytes(b"node\n"));
        assert!(!should_ignore_comm_bytes(b"python3\n"));
        assert!(!should_ignore_comm_bytes(b"nginx\n"));
    }

    #[test]
    fn test_non_ignored_family() {
        // "java" is not in IGNORE_FAMILIES
        assert!(!should_ignore_comm_bytes(b"java-some\n"));
    }

    #[test]
    fn test_empty_input() {
        assert!(!should_ignore_comm_bytes(b""));
    }

    #[test]
    fn test_just_newline() {
        assert!(!should_ignore_comm_bytes(b"\n"));
    }

    #[test]
    fn test_leading_hyphen() {
        // dash_pos == 0, family check should be skipped
        assert!(!should_ignore_comm_bytes(b"-foo\n"));
    }
}
