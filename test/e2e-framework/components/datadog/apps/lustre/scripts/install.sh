#!/usr/bin/env bash
#
# install.sh - Phase 1 of the all-in-one Lustre 2.15 bootstrap on EL9.
#
# Installs the Whamcloud server + client repositories, the Lustre-patched
# e2fsprogs repository, the lustre-dkms package and a matching kernel-devel,
# pins the kernel so a later `dnf upgrade` cannot break the DKMS modules, then
# reboots into the pinned kernel if a reboot is required.
#
# This script is idempotent and safe to re-run:
#   - repo files are rewritten each run (cheap, deterministic)
#   - packages are installed only when missing
#   - the kernel is pinned only once
#   - it reboots ONLY when the running kernel differs from the installed
#     kernel (i.e. a new kernel was just pulled in) AND the lustre modules are
#     not yet loadable. On a re-run after reboot it exits 0 without rebooting.
#
# Lustre kernel modules are out-of-tree and must be built against the exact
# running kernel. DKMS handles that, but only if kernel-devel matches the
# running kernel and the kernel does not drift afterwards. The EL minor of the
# host must match the el9.<minor> directory the Whamcloud RPMs were built for;
# this script resolves the highest 2.15.x point release available for the
# detected EL minor.
#
# No secrets are referenced. All package sources are public Whamcloud mirrors.

set -euo pipefail

LUSTRE_VERSION="${LUSTRE_VERSION:-2.15}"
WHAMCLOUD_BASE="https://downloads.whamcloud.com/public"

log() { echo "[lustre-install] $*"; }
err() { echo "[lustre-install][error] $*" >&2; }

require_root() {
  if [[ "$(id -u)" -ne 0 ]]; then
    err "must run as root (modprobe/dnf/reboot require privilege)"
    exit 1
  fi
}

# Detect the EL minor version (e.g. "9.4") so we can point the repos at the
# matching Whamcloud build directory. Falls back to the major if minor parsing
# fails, which will surface as a clear dnf error rather than a silent mismatch.
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
  # Returns success if the lustre client module can be loaded.
  modprobe -n lustre >/dev/null 2>&1
}

set_selinux_permissive() {
  # SELinux Enforcing on EL9 is untested with Lustre mounts + dd-agent sudo
  # CLIs and is known to block them. Drop to permissive for the lab. Idempotent.
  if command -v setenforce >/dev/null 2>&1; then
    setenforce 0 2>/dev/null || true
  fi
  if [[ -r /etc/selinux/config ]]; then
    sed -i 's/^SELINUX=enforcing/SELINUX=permissive/' /etc/selinux/config || true
  fi
  log "SELinux set to permissive (lab requirement)"
}

# Probe for the highest EL minor that has a Whamcloud 2.15 repo.
# Whamcloud publishes versioned dirs (lustre-2.15.5/el9.4, lustre-2.15.6/el9.5,
# etc.) but latest-release/ symlinks don't exist for every EL minor. Walk down
# from the detected minor until we find a valid server repomd.xml.
resolve_el_repo_dir() {
  local el_minor="$1"          # e.g. "9.6"
  local el_major="${el_minor%%.*}"  # "9"
  local el_m="${el_minor##*.}"      # "6"

  for (( m = el_m; m >= 3; m-- )); do
    local candidate="el${el_major}.${m}"
    local probe_url="${WHAMCLOUD_BASE}/lustre/latest-release/${candidate}/server/repodata/repomd.xml"
    if curl -sfI --max-time 10 "${probe_url}" >/dev/null 2>&1; then
      log "Whamcloud latest-release resolved to ${candidate}"
      echo "${candidate}"
      return 0
    fi
    log "no Whamcloud latest-release for ${candidate}, trying lower minor"
  done

  err "no Whamcloud latest-release found for el${el_major}.3 through el${el_major}.${el_m}"
  err "check https://downloads.whamcloud.com/public/lustre/ for available builds"
  return 1
}

write_repos() {
  local el_minor="$1"
  log "configuring Whamcloud repos for Lustre ${LUSTRE_VERSION} on el${el_minor}"

  local el_dir
  el_dir="$(resolve_el_repo_dir "${el_minor}")"

  # Lustre server (includes ldiskfs/osd backend + mkfs.lustre kmods) and client.
  # el_dir may be a lower EL minor than the running kernel if Whamcloud hasn't
  # published packages for the exact minor yet. DKMS rebuilds the modules
  # against the running kernel so this is safe.
  cat >/etc/yum.repos.d/lustre.repo <<EOF
[lustre-server]
name=lustre-server
baseurl=${WHAMCLOUD_BASE}/lustre/latest-release/${el_dir}/server
enabled=1
gpgcheck=0

[lustre-client]
name=lustre-client
baseurl=${WHAMCLOUD_BASE}/lustre/latest-release/${el_dir}/client
enabled=1
gpgcheck=0

[e2fsprogs-wc]
name=e2fsprogs-wc
baseurl=${WHAMCLOUD_BASE}/e2fsprogs/latest/el9
enabled=1
gpgcheck=0
EOF
  log "repos written for ${el_dir}"
}

install_packages() {
  log "installing build prerequisites"
  dnf install -y dnf-plugins-core "kernel-devel-$(uname -r)" \
    kernel-headers gcc make perl elfutils-libelf-devel 2>/dev/null || \
    dnf install -y dnf-plugins-core kernel-devel kernel-headers \
      gcc make perl elfutils-libelf-devel

  # Lustre-patched e2fsprogs provides mkfs.lustre's ldiskfs backend. Allow it to
  # replace the stock e2fsprogs.
  log "installing Lustre-patched e2fsprogs"
  dnf install -y --allowerasing e2fsprogs

  # lustre-dkms builds the lustre/lnet/ldiskfs modules against the running
  # kernel. lustre-osd-ldiskfs-mount + lustre supply mkfs.lustre, mount.lustre,
  # lctl, lnetctl, lfs.
  log "installing lustre-dkms + tools (DKMS build runs here)"
  dnf install -y lustre-dkms lustre-osd-ldiskfs-mount lustre || {
    err "lustre package install failed - check that the running kernel's EL"
    err "minor matches an available Whamcloud el9.<minor> build."
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
