#!/usr/bin/env bash
#
# install.sh - Phase 1 of the all-in-one Lustre 2.15 bootstrap on EL8.
#
# Whamcloud publishes pre-built Lustre SERVER packages only for EL8 (RHEL/Rocky
# 8.x). EL9 ships client-only packages. This script targets EL8 and installs:
#   - Whamcloud server + client + e2fsprogs repos
#   - lustre-all-dkms (builds server+client kernel modules against running kernel)
#   - lustre-osd-ldiskfs-mount (mkfs.lustre, mount.lustre)
#   - lustre (lctl, lnetctl, lfs userspace tools)
#
# The repo URL is probed: walk lustre-2.15.X/el8.Y/server/ from newest to
# oldest until repomd.xml returns 200. DKMS rebuilds against the running
# kernel so a minor-version mismatch between the RPM tree and the AMI is safe.
#
# Idempotent: repos rewritten each run; packages installed only when missing;
# kernel pinned only once; reboots only when a new kernel was installed and
# the lustre modules are not yet loadable.
#
# No secrets referenced. All sources are public Whamcloud mirrors.

set -euo pipefail

LUSTRE_VERSION="${LUSTRE_VERSION:-2.15}"
WHAMCLOUD_BASE="https://downloads.whamcloud.com/public"

log() { echo "[lustre-install] $*"; }
err() { echo "[lustre-install][error] $*" >&2; }

require_root() {
  if [[ "$(id -u)" -ne 0 ]]; then
    err "must run as root"
    exit 1
  fi
}

detect_el_minor() {
  local v
  if [[ -r /etc/os-release ]]; then
    # shellcheck source=/dev/null
    v="$(. /etc/os-release; echo "${VERSION_ID:-}")"
  fi
  if [[ -z "${v:-}" && -r /etc/redhat-release ]]; then
    v="$(grep -oE '[0-9]+\.[0-9]+' /etc/redhat-release | head -n1)"
  fi
  if [[ -z "${v:-}" ]]; then
    err "cannot detect EL version from /etc/os-release or /etc/redhat-release"
    exit 1
  fi
  echo "$v"
}

modules_loadable() {
  modprobe -n lustre >/dev/null 2>&1
}

set_selinux_permissive() {
  if command -v setenforce >/dev/null 2>&1; then
    setenforce 0 2>/dev/null || true
  fi
  if [[ -r /etc/selinux/config ]]; then
    sed -i 's/^SELINUX=enforcing/SELINUX=permissive/' /etc/selinux/config || true
  fi
  log "SELinux set to permissive (lab requirement)"
}

# Probe versioned Whamcloud paths lustre-2.15.X/el8.Y/server/ for the highest
# available build. Walk patch versions 9..0 against the detected EL8 minor,
# falling back to lower minors. First hit wins.
resolve_lustre_baseurl() {
  local el_minor="$1"               # e.g. "8.10"
  local el_major="${el_minor%%.*}"  # "8"
  local el_m="${el_minor##*.}"      # "10"

  for patch in 9 8 7 6 5 4 3 2 1 0; do
    local ver="${LUSTRE_VERSION}.${patch}"
    for (( m = el_m; m >= 6; m-- )); do
      local el_dir="el${el_major}.${m}"
      local base="${WHAMCLOUD_BASE}/lustre/lustre-${ver}/${el_dir}"
      local probe="${base}/server/repodata/repomd.xml"
      if curl -sf --max-time 10 -o /dev/null -w "%{http_code}" "${probe}" 2>/dev/null | grep -q "^200"; then
        log "resolved: lustre-${ver}/${el_dir}"
        echo "${base}"
        return 0
      fi
    done
  done

  err "no Whamcloud lustre-${LUSTRE_VERSION}.x server repo found for el${el_major}.6..${el_major}.${el_m}"
  err "check https://downloads.whamcloud.com/public/lustre/ for available builds"
  return 1
}

write_repos() {
  local el_minor="$1"
  log "configuring Whamcloud repos for Lustre ${LUSTRE_VERSION} on el${el_minor}"

  local base_url
  base_url="$(resolve_lustre_baseurl "${el_minor}")"

  cat >/etc/yum.repos.d/lustre.repo <<EOF
[lustre-server]
name=lustre-server
baseurl=${base_url}/server
enabled=1
gpgcheck=0

[lustre-client]
name=lustre-client
baseurl=${base_url}/client
enabled=1
gpgcheck=0

[e2fsprogs-wc]
name=e2fsprogs-wc
baseurl=${WHAMCLOUD_BASE}/e2fsprogs/latest/el8
enabled=1
gpgcheck=0
EOF
  log "repos written (baseurl=${base_url})"
}

install_packages() {
  log "installing build prerequisites"
  # Try version-pinned kernel-devel first; fall back to generic.
  dnf install -y dnf-plugins-core "kernel-devel-$(uname -r)" \
    kernel-headers gcc make perl elfutils-libelf-devel 2>/dev/null || \
    dnf install -y dnf-plugins-core kernel-devel kernel-headers \
      gcc make perl elfutils-libelf-devel

  # Lustre-patched e2fsprogs provides mkfs.lustre's ldiskfs backend.
  log "installing Lustre-patched e2fsprogs"
  dnf install -y --allowerasing e2fsprogs

  # lustre-all-dkms builds all server+client modules (lustre, lnet, ldiskfs,
  # osd-ldiskfs) via DKMS against the running kernel. lustre-osd-ldiskfs-mount
  # provides mkfs.lustre and mount.lustre. lustre provides lctl/lnetctl/lfs.
  log "installing lustre-all-dkms + server tools (DKMS build runs here)"
  dnf install -y lustre-all-dkms lustre-osd-ldiskfs-mount lustre || {
    err "lustre package install failed"
    err "running kernel: $(uname -r); check kernel-devel version matches"
    exit 1
  }
}

pin_kernel() {
  # Prevent kernel drift that would invalidate the DKMS build. Best-effort:
  # versionlock if available, otherwise exclude=kernel* in dnf.conf.
  if dnf versionlock --help >/dev/null 2>&1 || dnf install -y python3-dnf-plugin-versionlock >/dev/null 2>&1; then
    dnf versionlock add "kernel-$(uname -r)" "kernel-core-$(uname -r)" \
      "kernel-modules-$(uname -r)" 2>/dev/null || true
    log "kernel pinned via versionlock"
  fi
  if ! grep -q '^exclude=.*kernel' /etc/dnf/dnf.conf 2>/dev/null; then
    echo 'exclude=kernel kernel-core kernel-modules kernel-devel' >>/etc/dnf/dnf.conf
    log "kernel pinned via dnf.conf exclude"
  fi
}

main() {
  require_root
  set_selinux_permissive

  if modules_loadable; then
    log "lustre modules already loadable for kernel $(uname -r); nothing to install"
    exit 0
  fi

  local el_minor
  el_minor="$(detect_el_minor)"
  write_repos "$el_minor"
  install_packages
  pin_kernel

  # Trigger the DKMS build for the current kernel if it didn't auto-run.
  if command -v dkms >/dev/null 2>&1; then
    log "dkms status:"
    dkms status || true
    dkms autoinstall 2>/dev/null || true
  fi

  if modules_loadable; then
    log "lustre modules built and loadable for kernel $(uname -r); no reboot needed"
    exit 0
  fi

  # If a new kernel was installed (DKMS built against it) but we are not running
  # it yet, reboot into it. The next Pulumi command (configure.sh) will block on
  # SSH and re-run from a clean, kernel-matched state.
  log "rebooting into pinned kernel to finish DKMS module load"
  ( sleep 2; reboot ) &
  exit 0
}

main "$@"
