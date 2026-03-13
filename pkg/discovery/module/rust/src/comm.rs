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

/// Exact command names to ignore.
static IGNORE_COMMS: phf::Set<&str> = phf_set! {
    "chronyd",
    "cilium-agent",
    "containerd",
    "dhclient",
    "dockerd",
    "kubelet",
    "livenessprobe",
    "local-volume-pr",
    "sshd",
    "systemd",
};

/// Family prefixes (before the first `-`) to ignore.
static IGNORE_FAMILIES: phf::Set<&str> = phf_set! {
    "systemd",
    "datadog",
    "containerd",
    "docker",
};

/// Returns true if the process with the given PID should be ignored based on
/// its comm name. Returns true on read error (process likely gone).
pub fn should_ignore_comm(pid: i32) -> bool {
    let comm_path = procfs::root_path().join(pid.to_string()).join("comm");

    let mut file = match File::open(&comm_path) {
        Ok(f) => f,
        Err(_) => return true,
    };

    let mut buf = [0u8; MAX_COMM_LEN];
    let n = match file.read(&mut buf) {
        Ok(n) => n,
        Err(_) => return true,
    };

    let Some(comm_bytes) = buf.get(..n) else {
        return false;
    };

    // Check family prefix (before first `-`)
    if let Some(dash_pos) = comm_bytes.iter().position(|&b| b == b'-')
        && dash_pos > 0
        && let Some(family_bytes) = comm_bytes.get(..dash_pos)
        && let Ok(family) = std::str::from_utf8(family_bytes)
        && IGNORE_FAMILIES.contains(family)
    {
        return true;
    }

    // Trim trailing newline and check exact match
    let comm = match comm_bytes.last() {
        Some(&b'\n') => comm_bytes.get(..n.saturating_sub(1)).unwrap_or(comm_bytes),
        _ => comm_bytes,
    };

    if let Ok(comm_str) = std::str::from_utf8(comm) {
        IGNORE_COMMS.contains(comm_str)
    } else {
        false
    }
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
}
