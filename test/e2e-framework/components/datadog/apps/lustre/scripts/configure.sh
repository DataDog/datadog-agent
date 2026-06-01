#!/usr/bin/env bash
#
# configure.sh - Phase 2 of the all-in-one Lustre 2.15 bootstrap on EL9.
#
# Brings up a single-node Lustre filesystem over loopback LNet using
# loop-device backing files, then enables jobstats + changelogs, installs the
# dd-agent NOPASSWD sudoers drop-in, and starts a continuous I/O workload.
#
# Ordering (each step waits on the previous):
#   1. modprobe lustre / lnet / ldiskfs
#   2. configure LNet over loopback (tcp0 on lo)
#   3. mkfs.lustre the MGT, MDT, OST on loop-backed sparse files
#   4. mount MGS -> MDT -> OST -> client at ${MOUNT_POINT}
#   5. enable jobstats (jobid_var=procname_uid) so lustre.job_stats.* populate
#   6. register a changelog user on the MDT (for client changelog logs)
#   7. install /etc/sudoers.d/lustre-dd-agent (validated with visudo)
#   8. install + start the lustre-load systemd unit
#   9. run a warm-up I/O + metadata pass so kernel counters are already moving
#      before the first `datadog-agent check lustre` (a never-fired stat is
#      ABSENT, not zero).
#
# Idempotent: every step checks current state before mutating. Safe to re-run.
# Pass `teardown` as the first argument to stop the workload and unmount (used
# by the component's pulumi destroy hook).
#
# No secrets are referenced.

set -euo pipefail

FSNAME="${FSNAME:-lustre}"
MOUNT_POINT="${MOUNT_POINT:-/mnt/lustre}"
SCRIPT_DIR="${SCRIPT_DIR:-/opt/lustre-lab}"

# Loop-backed target sizes. Total ~ 7GB of sparse files; the gp3 root only
# allocates blocks that are actually written, so this fits comfortably on a
# 30-40GB root. MGT is tiny, MDT holds inodes, OST holds object data.
LUSTRE_DATA_DIR="/var/lib/lustre-lab"
MGT_IMG="${LUSTRE_DATA_DIR}/mgt.img"
MDT_IMG="${LUSTRE_DATA_DIR}/mdt.img"
OST_IMG="${LUSTRE_DATA_DIR}/ost.img"
MGT_SIZE="200M"
MDT_SIZE="2G"
OST_SIZE="5G"

# Loopback NID for the all-in-one node. Lustre uses lo for single-node dev.
LNET_NID="0@lo"

log() { echo "[lustre-configure] $*"; }
err() { echo "[lustre-configure][error] $*" >&2; }

require_root() {
  if [[ "$(id -u)" -ne 0 ]]; then
    err "must run as root"
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# teardown (pulumi destroy hook)
# ---------------------------------------------------------------------------
teardown() {
  log "tearing down Lustre lab"
  systemctl stop lustre-load.service 2>/dev/null || true
  umount "${MOUNT_POINT}" 2>/dev/null || true
  umount -t lustre -a 2>/dev/null || true
  for img in "${OST_IMG}" "${MDT_IMG}" "${MGT_IMG}"; do
    dev="$(losetup -j "$img" 2>/dev/null | cut -d: -f1)"
    if [[ -n "${dev:-}" ]]; then losetup -d "$dev" 2>/dev/null || true; fi
  done
  log "teardown complete"
  exit 0
}

# ---------------------------------------------------------------------------
# step 1: load kernel modules
# ---------------------------------------------------------------------------
load_modules() {
  log "loading kernel modules"
  modprobe lnet
  modprobe lustre
  modprobe ldiskfs 2>/dev/null || modprobe osd_ldiskfs 2>/dev/null || true
  if ! lsmod | grep -qE '^lustre'; then
    err "lustre module not loaded - DKMS build may have failed (see install.sh)"
    exit 1
  fi
  log "modules loaded: $(lsmod | grep -E '^(lustre|lnet|ldiskfs)' | awk '{print $1}' | paste -sd, -)"
}

# ---------------------------------------------------------------------------
# step 2: configure LNet over loopback
# ---------------------------------------------------------------------------
configure_lnet() {
  log "configuring LNet (tcp over lo)"
  lnetctl lnet configure 2>/dev/null || true
  # Add the loopback net if not already present. On a single node 0@lo is the
  # only NID; this lights up lustre.net.* / net.local.* (peer.* stays sparse).
  if ! lnetctl net show 2>/dev/null | grep -q 'lo'; then
    lnetctl net add --net lo 2>/dev/null || true
  fi
  lnetctl net show >/dev/null 2>&1 || {
    err "lnetctl net show failed"
    exit 1
  }
  log "LNet up: $(lctl list_nids 2>/dev/null | paste -sd, -)"
}

# ---------------------------------------------------------------------------
# step 3: format the targets on loop-backed files
# ---------------------------------------------------------------------------
make_backing_file() {
  local img="$1" size="$2"
  if [[ ! -f "$img" ]]; then
    log "creating backing file $img ($size)"
    truncate -s "$size" "$img"
  fi
}

format_targets() {
  mkdir -p "${LUSTRE_DATA_DIR}"
  make_backing_file "${MGT_IMG}" "${MGT_SIZE}"
  make_backing_file "${MDT_IMG}" "${MDT_SIZE}"
  make_backing_file "${OST_IMG}" "${OST_SIZE}"

  # mkfs.lustre is idempotent-ish: it refuses to reformat a Lustre target unless
  # forced. Detect an existing Lustre superblock and skip if present.
  is_formatted() { dumpe2fs -h "$1" 2>/dev/null | grep -q 'Filesystem volume name'; }

  if ! is_formatted "${MGT_IMG}"; then
    log "formatting MGT"
    mkfs.lustre --mgs --backfstype=ldiskfs --device-size=$((200*1024)) \
      --reformat "${MGT_IMG}"
  fi
  if ! is_formatted "${MDT_IMG}"; then
    log "formatting MDT for fs=${FSNAME}"
    mkfs.lustre --mdt --fsname="${FSNAME}" --index=0 --mgsnode="${LNET_NID}" \
      --backfstype=ldiskfs --device-size=$((2*1024*1024)) \
      --reformat "${MDT_IMG}"
  fi
  if ! is_formatted "${OST_IMG}"; then
    log "formatting OST for fs=${FSNAME}"
    mkfs.lustre --ost --fsname="${FSNAME}" --index=0 --mgsnode="${LNET_NID}" \
      --backfstype=ldiskfs --device-size=$((5*1024*1024)) \
      --reformat "${OST_IMG}"
  fi
}

# ---------------------------------------------------------------------------
# step 4: mount the targets and the client
# ---------------------------------------------------------------------------
mount_target() {
  local img="$1" mnt="$2"
  mkdir -p "$mnt"
  if mountpoint -q "$mnt"; then
    return 0
  fi
  log "mounting lustre target $img at $mnt"
  mount -t lustre -o loop "$img" "$mnt"
}

mount_all() {
  # Mount order matters: MGS -> MDT -> OST -> client.
  mount_target "${MGT_IMG}" "/lustre/mgt"
  mount_target "${MDT_IMG}" "/lustre/mdt"
  mount_target "${OST_IMG}" "/lustre/ost"

  mkdir -p "${MOUNT_POINT}"
  if ! mountpoint -q "${MOUNT_POINT}"; then
    log "mounting client at ${MOUNT_POINT}"
    mount -t lustre "${LNET_NID}:/${FSNAME}" "${MOUNT_POINT}"
  fi

  # Wait for all targets to report UP before declaring success.
  local tries=0
  until lctl dl 2>/dev/null | grep -q ' UP '; do
    tries=$((tries+1))
    [[ $tries -gt 30 ]] && { err "targets did not come UP"; lctl dl || true; exit 1; }
    sleep 2
  done
  log "devices UP:"
  lctl dl || true
  log "filesystem:"
  lfs df "${MOUNT_POINT}" || true
}

# ---------------------------------------------------------------------------
# step 5: enable jobstats so lustre.job_stats.* populate
# ---------------------------------------------------------------------------
enable_jobstats() {
  log "enabling jobstats (jobid_var=procname_uid)"
  lctl set_param -P jobid_var=procname_uid 2>/dev/null || \
    lctl set_param jobid_var=procname_uid 2>/dev/null || true
  # Make the MDT/OST collect per-job stats.
  lctl set_param -P mdt.*.job_cleanup_interval=600 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# step 6: register a changelog user (for client changelog logs)
# ---------------------------------------------------------------------------
register_changelog() {
  log "enabling + registering changelog user on MDT"
  lctl set_param -P mdd.*.changelog_mask=ALL 2>/dev/null || true
  # Only register once; re-registering creates extra cl<N> users.
  if ! lctl get_param -n "mdd.${FSNAME}-MDT0000.changelog_users" 2>/dev/null | grep -q 'cl'; then
    lctl --device "${FSNAME}-MDT0000" changelog_register 2>/dev/null || true
  fi
  lctl get_param "mdd.${FSNAME}-MDT0000.changelog_users" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# step 7: install the dd-agent NOPASSWD sudoers drop-in
# ---------------------------------------------------------------------------
install_sudoers() {
  local src="${SCRIPT_DIR}/lustre.sudoers"
  local dst="/etc/sudoers.d/lustre-dd-agent"
  if [[ ! -f "$src" ]]; then
    err "sudoers drop-in not found at $src"
    exit 1
  fi
  log "installing dd-agent sudoers drop-in"
  install -m 0440 -o root -g root "$src" "$dst"
  # Validate; a malformed sudoers file can lock out sudo entirely.
  if ! visudo -cf "$dst"; then
    err "sudoers drop-in failed validation; removing"
    rm -f "$dst"
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# step 8: install + start the continuous load workload
# ---------------------------------------------------------------------------
install_load_service() {
  log "installing lustre-load systemd unit"
  install -m 0644 "${SCRIPT_DIR}/lustre-load.service" /etc/systemd/system/lustre-load.service
  install -m 0755 "${SCRIPT_DIR}/load.sh" /usr/local/bin/lustre-load.sh
  systemctl daemon-reload
  systemctl enable --now lustre-load.service
  log "lustre-load service: $(systemctl is-active lustre-load.service 2>/dev/null || echo unknown)"
}

# ---------------------------------------------------------------------------
# step 9: warm-up pass so counters are moving before the first check
# ---------------------------------------------------------------------------
warmup() {
  log "running warm-up I/O + metadata pass"
  local d="${MOUNT_POINT}/warmup"
  mkdir -p "$d"
  dd if=/dev/zero of="$d/seed.dat" bs=1M count=64 conv=fsync 2>/dev/null || true
  sync
  echo 3 > /proc/sys/vm/drop_caches 2>/dev/null || true
  dd if="$d/seed.dat" of=/dev/null bs=1M 2>/dev/null || true
  for i in $(seq 1 50); do
    touch "$d/f.$i"; stat "$d/f.$i" >/dev/null 2>&1 || true
  done
  rm -f "$d"/f.* 2>/dev/null || true
  log "warm-up complete; counters should now be non-zero"
}

main() {
  if [[ "${1:-}" == "teardown" ]]; then
    teardown
  fi
  require_root
  load_modules
  configure_lnet
  format_targets
  mount_all
  enable_jobstats
  register_changelog
  install_sudoers
  install_load_service
  warmup
  log "Lustre all-in-one filesystem '${FSNAME}' is healthy and mounted at ${MOUNT_POINT}"
}

main "$@"
