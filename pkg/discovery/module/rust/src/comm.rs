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

/// Infrastructure processes and command-line helpers, matched against the
/// kernel comm name (not the generated service name). Executable names longer
/// than MAX_COMM_LEN must use the kernel's truncated name.
static IGNORE_COMMS: phf::Set<&'static [u8]> = phf_set! {
    b"ModemManager",
    b"NetworkManager",
    b"accounts-daemon",
    b"amazon-ssm-agen",
    b"atd",
    b"auditd",
    b"bash",
    b"cat",
    b"chronyd",
    b"cilium-agent",
    b"conmon",
    b"containerd",
    b"crond",
    b"dbus-broker",
    b"dbus-daemon",
    b"dhclient",
    b"do-agent",
    b"dockerd",
    b"droplet-agent",
    b"grep",
    b"irqbalance",
    b"kubelet",
    b"livenessprobe",
    b"local-volume-pr",
    b"polkitd",
    b"process-agent",
    b"security-agent",
    b"sh",
    b"sleep",
    b"sshd",
    b"sshd-auth",
    b"sshd-bd",
    b"sshd-session",
    b"sshd-sftponly",
    b"sshd.dssh",
    b"sshd.pam",
    b"sshd2",
    b"sshd_sftp",
    b"ssm-agent-worke",
    b"sssd",
    b"sssd_be",
    b"sssd_nss",
    b"sssd_pac",
    b"sssd_pam",
    b"sssd_ssh",
    b"sudo",
    b"system-probe",
    b"system-probe-li",
    b"systemd",
    b"tail",
    b"tee",
    b"trace-agent",
    b"udisksd",
    b"unattended-upgr",
    b"zsh",
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
    fn test_infrastructure_and_helpers() {
        for comm in [
            b"bash\n".as_slice(),
            b"sh\n",
            b"conmon\n",
            b"dbus-daemon\n",
            b"process-agent\n",
            b"tail\n",
            b"sshd-session\n",
            b"sshd.pam\n",
            b"sshd.dssh\n",
        ] {
            assert!(should_ignore_comm_bytes(comm), "{comm:?}");
        }
    }

    #[test]
    fn test_truncated_infrastructure_comm() {
        for executable in [
            "system-probe-lite",
            "amazon-ssm-agent",
            "ssm-agent-worker",
            "unattended-upgrades",
        ] {
            // Match the bytes actually read from /proc/<pid>/comm.
            let comm: Vec<_> = executable.bytes().take(MAX_COMM_LEN).collect();
            assert!(should_ignore_comm_bytes(&comm), "{executable}");
        }
    }

    #[test]
    fn test_helpers_require_exact_match() {
        // These names can identify applications rather than OS helpers.
        for comm in [
            b"agent\n".as_slice(),
            b"cron\n",
            b"dash\n",
            b"catalog\n",
            b"bash-api\n",
            b"sshd-exporter\n",
        ] {
            assert!(!should_ignore_comm_bytes(comm), "{comm:?}");
        }
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
