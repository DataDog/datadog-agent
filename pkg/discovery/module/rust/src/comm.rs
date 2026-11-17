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

/// Infrastructure daemons and CLI helpers also hold listening sockets and log
/// files, sometimes inherited from an application. Exclude these known names
/// so those resources do not cause discovery to report additional services.
///
/// Entries are exact, case-sensitive /proc/<pid>/comm names, limited to
/// MAX_COMM_LEN bytes. Keep them specific: "agent", "cron", and "dash" are
/// also used by applications.
static IGNORE_COMMS: phf::Set<&'static [u8]> = phf_set! {
    // Host services and management agents.
    b"ModemManager", // Mobile broadband modem management.
    b"NetworkManager", // Host network configuration.
    b"accounts-daemon", // OS user-account management.
    b"amazon-ssm-agen", // AWS Systems Manager agent, truncated to 15 bytes.
    b"atd", // Scheduler for one-off jobs submitted with `at`.
    b"auditd", // Linux audit event collection.
    b"chronyd", // NTP clock synchronization.
    b"crond", // Cron job scheduler.
    b"dbus-broker", // D-Bus inter-process message bus.
    b"dbus-daemon", // D-Bus inter-process message bus.
    b"dhclient", // DHCP client.
    b"do-agent", // DigitalOcean host metrics collection.
    b"droplet-agent", // DigitalOcean Droplet Console access.
    b"irqbalance", // Distributes hardware interrupts across CPUs.
    b"polkitd", // Authorization for privileged OS operations.
    b"ssm-agent-worke", // AWS Systems Manager worker, truncated to 15 bytes.
    b"systemd",
    b"udisksd", // Disk and storage device management.
    b"unattended-upgr", // Automatic package updates (unattended-upgrades).

    // SSSD integrates the host with directory-based identity/authentication.
    b"sssd",
    b"sssd_be", // Directory backend.
    b"sssd_nss", // User/group lookup.
    b"sssd_pac", // Kerberos authorization data.
    b"sssd_pam", // PAM authentication.
    b"sssd_ssh", // SSH public-key lookup.

    // Container and Kubernetes infrastructure.
    b"cilium-agent", // Cilium container networking.
    b"conmon", // Podman/CRI-O container monitor; holds container log FDs.
    b"containerd",
    b"dockerd",
    b"kubelet",
    b"livenessprobe", // Kubernetes CSI storage-driver health probe.
    b"local-volume-pr", // Kubernetes local-volume-provisioner, truncated.

    // Datadog subprocesses expose their own ports and write Agent log files.
    b"process-agent",
    b"security-agent",
    b"system-probe",
    b"system-probe-li", // system-probe-lite, truncated to 15 bytes.
    b"trace-agent",

    // SSH variants without a hyphen need exact matches. Generated service
    // names can drop suffixes such as .pam; match comm here.
    b"sshd",
    b"sshd.dssh",
    b"sshd.pam",
    b"sshd2",
    b"sshd_sftp",

    // Shells and CLI helpers can inherit service sockets or keep redirected log
    // files open. Those resources qualify them for discovery even when another
    // process runs the application, producing services for wrappers/helpers.
    b"bash",
    b"cat",
    b"grep",
    b"sh",
    b"sudo",
    b"tail",
    b"tee",
    b"zsh",
};

/// Family prefixes (before the first `-`) to ignore.
static IGNORE_FAMILIES: phf::Set<&'static [u8]> = phf_set! {
    b"systemd",
    b"datadog",
    b"containerd",
    b"docker",
    b"sshd", // SSH daemon variants and session helpers, e.g. sshd-session.
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
