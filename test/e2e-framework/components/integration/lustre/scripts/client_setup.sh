#!/bin/bash
# Lustre client bootstrap: installs the prebuilt KABI client kmod, configures LNet
# (tcp0), mounts the server filesystem, and grants the dd-agent user passwordless
# sudo for the Lustre CLIs the check shells out to.
#
# The Lustre release and repo el-path are parametrized by the scenario:
#   - LUSTRE_CLIENT_VERSION (default 2.15.6): Whamcloud release; forms the repo
#     path lustre-<version>. Set to 2.16.1 (with el9.4) to test a newer client.
#   - LUSTRE_CLIENT_EL_PATH (default el8.10): el8.10 for AlmaLinux 8.10, el9.4 for
#     AlmaLinux 9.4. NOTE: 2.16.x client RPMs exist ONLY for el9.4 (no el8).
#
# NO REBOOT / NO KERNEL SWAP: the stock AlmaLinux GA kernel (EL8.10 5.14?/EL9.4
# 5.14.0-427 line) is KABI-compatible with the standard whamcloud
# kmod-lustre-client, so it loads on the RUNNING kernel. Single self-contained
# Pulumi command:remote exec (runs once, no re-invoke after reboot) — so it must
# never reboot. If modprobe fails because the kernel floated past the kmod build,
# the ERR trap dumps diagnostics and the script exits non-zero (surfaced in the
# Pulumi error); we do NOT attempt a patched-kernel/reboot flow for the client.
#
# Idempotent: safe to re-run. On any failure the ERR trap dumps diagnostics.
set -euo pipefail

LUSTRE_FSNAME="${LUSTRE_FSNAME:-lustrefs}"
LUSTRE_LNET_IFACE="${LUSTRE_LNET_IFACE:-eth0}"
LUSTRE_MOUNTPOINT="${LUSTRE_MOUNTPOINT:-/mnt/lustre}"
LUSTRE_SERVER_NID="${LUSTRE_SERVER_NID:?LUSTRE_SERVER_NID is required (e.g. 10.0.0.5@tcp)}"
LUSTRE_CLIENT_VERSION="${LUSTRE_CLIENT_VERSION:-2.15.6}"
LUSTRE_CLIENT_EL_PATH="${LUSTRE_CLIENT_EL_PATH:-el8.10}"
WC_BASE="https://downloads.whamcloud.com/public"

diag() {
	echo "===== lustre client setup FAILED, diagnostics =====" >&2
	uname -r >&2 || true
	rpm -qa 'kernel*' 'kmod-lustre-client*' 'lustre-client*' 2>/dev/null >&2 || true
	lsmod | grep -E 'lustre|lnet' >&2 || true
	modprobe -n -v lustre >&2 2>&1 || true
	command -v lfs >&2 || echo "lfs not found" >&2
	command -v mount.lustre >&2 || echo "mount.lustre not found" >&2
	lnetctl net show >&2 2>&1 || true
	echo "--- ping server NID ${LUSTRE_SERVER_NID} ---" >&2
	lctl ping "${LUSTRE_SERVER_NID}" >&2 2>&1 || true
	dmesg | tail -n 80 >&2 || true
}
trap diag ERR

# ----- Repo + client packages against the RUNNING kernel (no kernel pkgs) -----
cat >/etc/yum.repos.d/lustre-client.repo <<EOF
[lustre-client]
name=lustre-client
baseurl=${WC_BASE}/lustre/lustre-${LUSTRE_CLIENT_VERSION}/${LUSTRE_CLIENT_EL_PATH}/client/
enabled=1
gpgcheck=0
EOF
dnf install -y --nogpgcheck lustre-client kmod-lustre-client

# ----- Configure LNet over tcp0 + load modules on the running kernel -----
cat >/etc/modprobe.d/lnet.conf <<EOF
options lnet networks=tcp0(${LUSTRE_LNET_IFACE})
EOF
modprobe lnet
modprobe lustre
lnetctl lnet configure || true
lnetctl net add --net tcp0 --if "${LUSTRE_LNET_IFACE}" || true

# ----- Wait for the server, then mount -----
# Client-side wait: the server installs its patched kernel, reboots, and formats
# the targets via cloud-init + a boot service, which takes several minutes. The
# client is a separate host (not affected by the server's reboot), so it simply
# waits here — no server-side Pulumi command that could be severed by the reboot.

# 1) LNet reachability (server LNet up; survives the server reboot).
for attempt in $(seq 1 120); do
	if lctl ping "${LUSTRE_SERVER_NID}" >/dev/null 2>&1; then
		echo "server NID ${LUSTRE_SERVER_NID} reachable (attempt ${attempt}/120)"
		break
	fi
	echo "waiting for server NID ${LUSTRE_SERVER_NID} (attempt ${attempt}/120)"
	sleep 10
done
lctl ping "${LUSTRE_SERVER_NID}"

# 2) Mount with retries: the server filesystem targets may not be mounted the
# instant LNet answers, so retry the mount until it succeeds.
mkdir -p "${LUSTRE_MOUNTPOINT}"
for attempt in $(seq 1 60); do
	if mountpoint -q "${LUSTRE_MOUNTPOINT}"; then break; fi
	if mount -t lustre "${LUSTRE_SERVER_NID}:/${LUSTRE_FSNAME}" "${LUSTRE_MOUNTPOINT}" 2>/dev/null; then
		echo "mounted ${LUSTRE_FSNAME} at ${LUSTRE_MOUNTPOINT} (attempt ${attempt}/60)"
		break
	fi
	echo "waiting for server filesystem to accept mount (attempt ${attempt}/60)"
	sleep 10
done
mountpoint -q "${LUSTRE_MOUNTPOINT}" || \
	mount -t lustre "${LUSTRE_SERVER_NID}:/${LUSTRE_FSNAME}" "${LUSTRE_MOUNTPOINT}"
lfs df "${LUSTRE_MOUNTPOINT}"

# ----- Passwordless sudo for the check's allowlisted CLIs -----
lctl_path="$(command -v lctl || echo /usr/sbin/lctl)"
lnetctl_path="$(command -v lnetctl || echo /usr/sbin/lnetctl)"
lfs_path="$(command -v lfs || echo /usr/bin/lfs)"
cat >/etc/sudoers.d/dd-agent-lustre <<EOF
dd-agent ALL=(ALL) NOPASSWD: ${lctl_path},${lnetctl_path},${lfs_path}
EOF
chmod 0440 /etc/sudoers.d/dd-agent-lustre
visudo -cf /etc/sudoers.d/dd-agent-lustre

echo "lustre client ready: mounted ${LUSTRE_SERVER_NID}:/${LUSTRE_FSNAME} at ${LUSTRE_MOUNTPOINT}"
exit 0
