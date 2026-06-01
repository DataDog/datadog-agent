#!/usr/bin/env bash
#
# load.sh - Continuous Lustre workload for the all-in-one lab.
#
# Runs an endless loop of realistic filesystem operations under the client
# mount so the Datadog `lustre` check sees cumulative kernel counters MOVING
# between collection intervals. The check is pull-based: it reads counters via
# lctl/lnetctl/lfs each interval, so the workload does not need to be
# synchronized with the check - it only needs to keep the counters advancing.
#
# A stat that has never fired is ABSENT (not zero), so every iteration
# deliberately exercises each op family at least once:
#
#   WRITE (sequential, fsync)        -> filesystem.write*/osc.ost_write/
#                                       obdfilter.write*/oss.ost_*/job_stats.write*
#   READ (after dropping page cache) -> filesystem.read*/osc.read_bytes/
#                                       obdfilter.read*/job_stats.read*
#   metadata churn (touch/stat/mkdir -> filesystem.{open,close,getattr,unlink,
#     /rm/mv/ln/xattr)                  mkdir,rename,...}/mdc.mds_*/
#                                       mds.mdt.mds_reint_*/mds.mdt.exports.*/
#                                       job_stats.{open,unlink,mkdir,...}
#   fsync/sync                       -> filesystem.fsync/obdfilter.sync/oss.ost_sync
#   truncate / fallocate punch       -> filesystem.truncate/obdfilter.punch/osc.ost_punch
#   setfattr/getfattr                -> *.setxattr/getxattr/mds.mdt.mds_reint_setxattr
#   lfs setstripe (object create)    -> obdfilter.create/oss.ost_create/job_stats.create
#   rm of striped files (destroy)    -> obdfilter.destroy/oss.ost_destroy/job_stats.destroy
#   lfs df / statfs                  -> *.statfs / refreshes capacity gauges
#   all I/O + metadata               -> ldlm.services.*/ldlm.namespaces.pool.* (locks)
#
# Reads MUST drop the page cache first or OSC/OST read counters never move.
#
# Run as the lustre-load systemd unit (see lustre-load.service). The unit's
# ExecStart name is `lustre-load.sh` so jobstats (jobid_var=procname_uid) tag
# jobs as `lustre-load.<uid>` - a recognizable job_id.
#
# No secrets are referenced.

set -uo pipefail

MOUNT_POINT="${MOUNT_POINT:-/mnt/lustre}"
WORK_DIR="${MOUNT_POINT}/workload"
# Interval between full workload cycles. Tuned so the loop is steady rather than
# a busy spin; the Lustre check default min_collection_interval is well under
# this, so counters always advance between checks.
CYCLE_INTERVAL="${CYCLE_INTERVAL:-15}"
WRITE_MB="${WRITE_MB:-128}"
META_FILES="${META_FILES:-200}"

log() { echo "[lustre-load] $*"; }

wait_for_mount() {
  local tries=0
  until mountpoint -q "${MOUNT_POINT}"; do
    tries=$((tries+1))
    [[ $tries -gt 60 ]] && { log "mount ${MOUNT_POINT} never appeared; exiting"; exit 1; }
    log "waiting for ${MOUNT_POINT} to be mounted..."
    sleep 5
  done
}

drop_caches() {
  sync
  echo 3 > /proc/sys/vm/drop_caches 2>/dev/null || true
}

bulk_write() {
  # Striped file to drive OST object create + bulk write.
  lfs setstripe -c 1 -S 1M "${WORK_DIR}/bulk.dat" 2>/dev/null || true
  dd if=/dev/zero of="${WORK_DIR}/bulk.dat" bs=1M count="${WRITE_MB}" conv=fsync 2>/dev/null || true
}

bulk_read() {
  drop_caches
  dd if="${WORK_DIR}/bulk.dat" of=/dev/null bs=1M 2>/dev/null || true
}

metadata_churn() {
  local base="${WORK_DIR}/meta"
  mkdir -p "${base}"
  local i
  for i in $(seq 1 "${META_FILES}"); do
    local f="${base}/file.${i}"
    : > "${f}"
    stat "${f}" >/dev/null 2>&1
    setfattr -n user.lab -v "v${i}" "${f}" 2>/dev/null || true
    getfattr -n user.lab "${f}" >/dev/null 2>&1 || true
  done
  # rename + hardlink + nested dirs
  mkdir -p "${base}/sub"
  for i in $(seq 1 $((META_FILES/4))); do
    mv "${base}/file.${i}" "${base}/sub/moved.${i}" 2>/dev/null || true
    ln "${base}/sub/moved.${i}" "${base}/sub/link.${i}" 2>/dev/null || true
  done
  # truncate + sparse punch to exercise punch/truncate paths
  truncate -s 4M "${WORK_DIR}/sparse.dat" 2>/dev/null || true
  fallocate --punch-hole --keep-size -o 0 -l 1M "${WORK_DIR}/sparse.dat" 2>/dev/null || true
  # delete everything to drive unlink + OST object destroy
  rm -rf "${base}" 2>/dev/null || true
}

statfs_pass() {
  lfs df "${MOUNT_POINT}" >/dev/null 2>&1 || true
  lfs df -i "${MOUNT_POINT}" >/dev/null 2>&1 || true
}

cycle() {
  mkdir -p "${WORK_DIR}"
  bulk_write
  bulk_read
  metadata_churn
  statfs_pass
  rm -f "${WORK_DIR}/bulk.dat" "${WORK_DIR}/sparse.dat" 2>/dev/null || true
}

main() {
  wait_for_mount
  log "starting continuous workload on ${MOUNT_POINT} (cycle every ${CYCLE_INTERVAL}s)"
  while true; do
    cycle
    sleep "${CYCLE_INTERVAL}"
  done
}

main "$@"
