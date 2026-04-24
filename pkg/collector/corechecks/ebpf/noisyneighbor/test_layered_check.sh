#!/bin/bash
# test_layered_check.sh — Validates the 3-layer noisy neighbor check end-to-end.
#
# Prerequisites:
#   - Linux kernel 6.2+ with cgroup v2 and PSI enabled
#   - Root access (eBPF + cgroup manipulation)
#   - system-probe running with noisy_neighbor module enabled
#   - stress-ng installed (apt install stress-ng / dnf install stress-ng)
#   - jq installed
#
# Usage:
#   sudo ./test_layered_check.sh [--socket /path/to/sysprobe.sock]

set -euo pipefail

# ─── Configuration ────────────────────────────────────────────────────────────

SYSPROBE_SOCKET="${DD_SYSPROBE_SOCKET:-/opt/datadog-agent/run/sysprobe.sock}"
CGROUP_ROOT="/sys/fs/cgroup"
TEST_PREFIX="nn_test"
VICTIM_CG="${TEST_PREFIX}_victim"
NOISY_CG="${TEST_PREFIX}_noisy"
QUIET_CG="${TEST_PREFIX}_quiet"
# CPU quota in microseconds per 100ms period
VICTIM_QUOTA=800000  # 800% (8 cores worth) — enough headroom for threads to preempt each other
NOISY_QUOTA=800000   # 800% (8 cores worth)
QUIET_QUOTA=50000    # 50% of one core
STRESS_DURATION=30   # seconds to run workloads
SETTLE_TIME=8        # seconds to wait for metrics to accumulate

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# ─── Argument parsing ────────────────────────────────────────────────────────

while [[ $# -gt 0 ]]; do
    case $1 in
        --socket) SYSPROBE_SOCKET="$2"; shift 2 ;;
        *) echo "Unknown arg: $1"; exit 1 ;;
    esac
done

# ─── Helpers ──────────────────────────────────────────────────────────────────

pass() { echo -e "  ${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "  ${RED}✗ FAIL${NC}: $1"; FAILURES=$((FAILURES + 1)); }
info() { echo -e "${BLUE}▸${NC} $1"; }
warn() { echo -e "${YELLOW}⚠${NC} $1"; }
header() { echo -e "\n${BLUE}━━━ $1 ━━━${NC}"; }

FAILURES=0
PIDS_TO_KILL=()

sysprobe_get() {
    curl -sf --unix-socket "$SYSPROBE_SOCKET" "http://unix/noisy_neighbor$1" 2>/dev/null
}

sysprobe_post() {
    local endpoint="$1"
    local body="$2"
    curl -sf --unix-socket "$SYSPROBE_SOCKET" \
        -X POST -H "Content-Type: application/json" \
        -d "$body" \
        "http://unix/noisy_neighbor$endpoint" 2>/dev/null
}

get_cgroup_inode() {
    stat -c '%i' "${CGROUP_ROOT}/$1" 2>/dev/null
}

read_psi() {
    local cg_path="${CGROUP_ROOT}/$1/cpu.pressure"
    if [[ -f "$cg_path" ]]; then
        grep '^some' "$cg_path" | grep -oP 'avg10=\K[0-9.]+'
    else
        echo "0"
    fi
}

read_throttle() {
    local stat_path="${CGROUP_ROOT}/$1/cpu.stat"
    if [[ -f "$stat_path" ]]; then
        grep 'throttled_usec' "$stat_path" | awk '{print $2}'
    else
        echo "0"
    fi
}

# ─── Preflight checks ────────────────────────────────────────────────────────

header "Preflight Checks"

# Root check
if [[ $EUID -ne 0 ]]; then
    fail "Must run as root"
    exit 1
fi
pass "Running as root"

# Kernel version
KVER=$(uname -r | cut -d. -f1-2)
KMAJOR=$(echo "$KVER" | cut -d. -f1)
KMINOR=$(echo "$KVER" | cut -d. -f2)
if [[ "$KMAJOR" -lt 6 ]] || { [[ "$KMAJOR" -eq 6 ]] && [[ "$KMINOR" -lt 2 ]]; }; then
    fail "Kernel $KVER < 6.2 (required for BPF task storage kfuncs)"
    exit 1
fi
pass "Kernel version $KVER >= 6.2"

# cgroup v2
if ! mount | grep -q 'cgroup2'; then
    fail "cgroup v2 not mounted"
    exit 1
fi
pass "cgroup v2 available"

# PSI enabled
if [[ ! -f "${CGROUP_ROOT}/cpu.pressure" ]]; then
    fail "PSI not enabled (no cpu.pressure at cgroup root)"
    exit 1
fi
pass "PSI enabled"

# stress-ng
if ! command -v stress-ng &>/dev/null; then
    fail "stress-ng not installed"
    exit 1
fi
pass "stress-ng available"

# jq
if ! command -v jq &>/dev/null; then
    fail "jq not installed"
    exit 1
fi
pass "jq available"

# system-probe socket
if [[ ! -S "$SYSPROBE_SOCKET" ]]; then
    fail "system-probe socket not found at $SYSPROBE_SOCKET"
    exit 1
fi
pass "system-probe socket at $SYSPROBE_SOCKET"

# noisy_neighbor module responsive
if ! sysprobe_get "/check" >/dev/null; then
    fail "noisy_neighbor module not responding (GET /check failed)"
    exit 1
fi
pass "noisy_neighbor module responding"

# ─── Cleanup function ────────────────────────────────────────────────────────

cleanup() {
    info "Cleaning up..."
    for pid in "${PIDS_TO_KILL[@]}"; do
        kill "$pid" 2>/dev/null || true
        wait "$pid" 2>/dev/null || true
    done
    # Clear watchlist
    sysprobe_post "/watchlist" '{"cgroup_ids":[]}' >/dev/null 2>&1 || true
    # Remove test cgroups (must kill all processes first)
    for cg in "$VICTIM_CG" "$NOISY_CG" "$QUIET_CG"; do
        if [[ -d "${CGROUP_ROOT}/${cg}" ]]; then
            # Kill any remaining processes
            if [[ -f "${CGROUP_ROOT}/${cg}/cgroup.procs" ]]; then
                while read -r pid; do
                    kill -9 "$pid" 2>/dev/null || true
                done < "${CGROUP_ROOT}/${cg}/cgroup.procs"
            fi
            sleep 0.5
            rmdir "${CGROUP_ROOT}/${cg}" 2>/dev/null || true
        fi
    done
}
trap cleanup EXIT

# ─── Create test cgroups ──────────────────────────────────────────────────────

header "Setting Up Test Cgroups"

for cg in "$VICTIM_CG" "$NOISY_CG" "$QUIET_CG"; do
    if [[ -d "${CGROUP_ROOT}/${cg}" ]]; then
        rmdir "${CGROUP_ROOT}/${cg}" 2>/dev/null || true
    fi
    mkdir -p "${CGROUP_ROOT}/${cg}"
    # Enable cpu controller
    echo "+cpu +cpuset" > "${CGROUP_ROOT}/${cg}/cgroup.subtree_control" 2>/dev/null || true
done

# Set CPU quotas (period=100ms)
echo "100000 ${VICTIM_QUOTA}" > "${CGROUP_ROOT}/${VICTIM_CG}/cpu.max"
echo "100000 ${NOISY_QUOTA}"  > "${CGROUP_ROOT}/${NOISY_CG}/cpu.max"
echo "100000 ${QUIET_QUOTA}"  > "${CGROUP_ROOT}/${QUIET_CG}/cpu.max"

VICTIM_INODE=$(get_cgroup_inode "$VICTIM_CG")
NOISY_INODE=$(get_cgroup_inode "$NOISY_CG")
QUIET_INODE=$(get_cgroup_inode "$QUIET_CG")

info "Created cgroups:"
info "  victim: ${VICTIM_CG} (inode: ${VICTIM_INODE}, quota: ${VICTIM_QUOTA}us/100ms)"
info "  noisy:  ${NOISY_CG}  (inode: ${NOISY_INODE}, quota: ${NOISY_QUOTA}us/100ms)"
info "  quiet:  ${QUIET_CG}  (inode: ${QUIET_INODE}, quota: ${QUIET_QUOTA}us/100ms)"

# ─── Test 1: Empty watchlist produces no eBPF stats ──────────────────────────

header "Test 1: Empty Watchlist (fast-exit gate)"

# Clear watchlist and flush any stale data
sysprobe_post "/watchlist" '{"cgroup_ids":[]}'
sleep 1
sysprobe_get "/check" > /dev/null  # flush

# Start a workload in the victim cgroup (exec inside cgroup so workers inherit)
bash -c "echo \$\$ > ${CGROUP_ROOT}/${VICTIM_CG}/cgroup.procs && exec stress-ng --cpu 2 --timeout ${STRESS_DURATION}s --quiet" &
STRESS_PID=$!
PIDS_TO_KILL+=("$STRESS_PID")

sleep "$SETTLE_TIME"

# Fetch stats — should be empty since watchlist is empty
STATS=$(sysprobe_get "/check")
CGROUP_COUNT=$(echo "$STATS" | jq '(.cgroup_stats // []) | length')

if [[ "$CGROUP_COUNT" -eq 0 ]] || [[ "$CGROUP_COUNT" == "null" ]]; then
    pass "No cgroup stats with empty watchlist (fast-exit gate working)"
else
    fail "Got $CGROUP_COUNT cgroup stats with empty watchlist (fast-exit gate not working)"
fi

# ─── Test 2: Watchlist activation produces stats ──────────────────────────────

header "Test 2: Watchlist Activation"

# Add victim cgroup to watchlist
sysprobe_post "/watchlist" "{\"cgroup_ids\":[${VICTIM_INODE}]}"
info "Added victim cgroup (inode $VICTIM_INODE) to watchlist"

sleep "$SETTLE_TIME"

STATS=$(sysprobe_get "/check")
VICTIM_STATS=$(echo "$STATS" | jq "(.cgroup_stats // [])[] | select(.CgroupID == $VICTIM_INODE)")

if [[ -n "$VICTIM_STATS" ]]; then
    EVENT_COUNT=$(echo "$VICTIM_STATS" | jq '.EventCount')
    pass "Got stats for watched cgroup (EventCount: $EVENT_COUNT)"
else
    fail "No stats for watched cgroup after watchlist activation"
fi

# ─── Test 3: Self-preemption detection ────────────────────────────────────────

header "Test 3: Self-Preemption (many threads, same cgroup)"

# Run many more threads than CPUs with high quota → threads preempt each other
kill "$STRESS_PID" 2>/dev/null; wait "$STRESS_PID" 2>/dev/null || true
PIDS_TO_KILL=()

bash -c "echo \$\$ > ${CGROUP_ROOT}/${VICTIM_CG}/cgroup.procs && exec stress-ng --cpu 64 --timeout ${STRESS_DURATION}s --quiet" &
STRESS_PID=$!
PIDS_TO_KILL+=("$STRESS_PID")

# Ensure watchlist is set
sysprobe_post "/watchlist" "{\"cgroup_ids\":[${VICTIM_INODE}]}"

sleep "$SETTLE_TIME"
# Flush stale data
sysprobe_get "/check" > /dev/null
sleep "$SETTLE_TIME"

STATS=$(sysprobe_get "/check")
VICTIM_STATS=$(echo "$STATS" | jq "(.cgroup_stats // [])[] | select(.CgroupID == $VICTIM_INODE)")

if [[ -n "$VICTIM_STATS" ]]; then
    SELF_PREEMPT=$(echo "$VICTIM_STATS" | jq '.SelfPreemptionCount')
    FOREIGN_PREEMPT=$(echo "$VICTIM_STATS" | jq '.ForeignPreemptionCount')
    info "Self preemptions: $SELF_PREEMPT, Foreign preemptions: $FOREIGN_PREEMPT"

    if [[ "$SELF_PREEMPT" -gt 0 ]]; then
        pass "Self-preemptions detected (64 threads competing for CPUs)"
    else
        fail "No self-preemptions detected (expected with 64 threads competing for CPUs)"
    fi
else
    fail "No stats for victim cgroup"
fi

# ─── Test 4: Foreign preemption + preemptor identification ────────────────────

header "Test 4: Foreign Preemption + Neighbor Identification"

# Start aggressive workload in noisy cgroup
bash -c "echo \$\$ > ${CGROUP_ROOT}/${NOISY_CG}/cgroup.procs && exec stress-ng --cpu 64 --timeout ${STRESS_DURATION}s --quiet" &
NOISY_PID=$!
PIDS_TO_KILL+=("$NOISY_PID")

# Watch both cgroups
sysprobe_post "/watchlist" "{\"cgroup_ids\":[${VICTIM_INODE},${NOISY_INODE}]}"

sleep "$SETTLE_TIME"
# Flush stale
sysprobe_get "/check" > /dev/null
sleep "$SETTLE_TIME"

STATS=$(sysprobe_get "/check")
VICTIM_STATS=$(echo "$STATS" | jq "(.cgroup_stats // [])[] | select(.CgroupID == $VICTIM_INODE)")
PREEMPTOR_STATS=$(echo "$STATS" | jq '.preemptor_stats // []')

if [[ -n "$VICTIM_STATS" ]]; then
    FOREIGN_PREEMPT=$(echo "$VICTIM_STATS" | jq '.ForeignPreemptionCount')
    info "Victim foreign preemptions: $FOREIGN_PREEMPT"

    if [[ "$FOREIGN_PREEMPT" -gt 0 ]]; then
        pass "Foreign preemptions detected on victim (noisy neighbor signal)"
    else
        warn "No foreign preemptions on victim — CPUs may not overlap. This is topology-dependent."
    fi
else
    fail "No stats for victim cgroup"
fi

# Check preemptor identification (Layer 3)
PREEMPTOR_COUNT=$(echo "$PREEMPTOR_STATS" | jq 'length')
if [[ "$PREEMPTOR_COUNT" -gt 0 ]]; then
    # Check if noisy cgroup appears as a preemptor of victim
    NOISY_AS_PREEMPTOR=$(echo "$PREEMPTOR_STATS" | jq "[.[] | select(.VictimCgroupID == $VICTIM_INODE and .PreemptorCgroupID == $NOISY_INODE)] | length")
    if [[ "$NOISY_AS_PREEMPTOR" -gt 0 ]]; then
        PREEMPT_COUNT=$(echo "$PREEMPTOR_STATS" | jq ".[] | select(.VictimCgroupID == $VICTIM_INODE and .PreemptorCgroupID == $NOISY_INODE) | .Count")
        pass "Layer 3: Identified noisy cgroup as preemptor of victim (count: $PREEMPT_COUNT)"
    else
        warn "Noisy cgroup not identified as preemptor — may be on different CPUs"
    fi
else
    info "No preemptor stats (foreign preemptions may not have occurred)"
fi

# ─── Test 4b: Impacted signal precondition ────────────────────────────────────

# The noisy_neighbor.impacted signal (computed agent-side) fires when
# ForeignPreemptionCount >= min_foreign_preemptions_impact (default 10).
# Verify the eBPF data would trigger it.
if [[ -n "$VICTIM_STATS" ]]; then
    FOREIGN_PREEMPT=$(echo "$VICTIM_STATS" | jq '.ForeignPreemptionCount')
    IMPACT_THRESHOLD=10  # default min_foreign_preemptions_impact
    if [[ "$FOREIGN_PREEMPT" -ge "$IMPACT_THRESHOLD" ]]; then
        pass "Foreign preemptions ($FOREIGN_PREEMPT) >= threshold ($IMPACT_THRESHOLD) — impacted signal would be 1.0"
    elif [[ "$FOREIGN_PREEMPT" -gt 0 ]]; then
        info "Foreign preemptions ($FOREIGN_PREEMPT) below threshold ($IMPACT_THRESHOLD) — impacted signal would be 0.0 (topology-dependent)"
    else
        info "No foreign preemptions — impacted signal would be 0.0"
    fi
fi

# ─── Test 5: Latency histogram buckets ────────────────────────────────────────

header "Test 5: Latency Histogram Buckets"

# Re-fetch or use existing stats
if [[ -n "$VICTIM_STATS" ]]; then
    B_LT100=$(echo "$VICTIM_STATS" | jq '.LatencyBucketLt100us')
    B_100_1M=$(echo "$VICTIM_STATS" | jq '.LatencyBucket100us1ms')
    B_1M_10M=$(echo "$VICTIM_STATS" | jq '.LatencyBucket1ms10ms')
    B_GT10M=$(echo "$VICTIM_STATS" | jq '.LatencyBucketGt10ms')
    EVENT_COUNT=$(echo "$VICTIM_STATS" | jq '.EventCount')
    BUCKET_SUM=$((B_LT100 + B_100_1M + B_1M_10M + B_GT10M))

    info "Buckets: <100us=$B_LT100, 100us-1ms=$B_100_1M, 1ms-10ms=$B_1M_10M, >10ms=$B_GT10M"
    info "Bucket sum: $BUCKET_SUM, EventCount: $EVENT_COUNT"

    if [[ "$BUCKET_SUM" -eq "$EVENT_COUNT" ]]; then
        pass "Histogram bucket sum equals EventCount"
    else
        fail "Histogram bucket sum ($BUCKET_SUM) != EventCount ($EVENT_COUNT)"
    fi

    if [[ "$BUCKET_SUM" -gt 0 ]]; then
        pass "Histogram buckets are populated"
    else
        fail "All histogram buckets are zero"
    fi
else
    fail "No victim stats to check buckets"
fi

# ─── Test 6: CPU migration tracking ──────────────────────────────────────────

header "Test 6: CPU Migration Tracking"

if [[ -n "$VICTIM_STATS" ]]; then
    MIGRATIONS=$(echo "$VICTIM_STATS" | jq '.CpuMigrations')
    info "CPU migrations: $MIGRATIONS"
    if [[ "$MIGRATIONS" -ge 0 ]]; then
        pass "CPU migration counter present (value: $MIGRATIONS)"
    fi
else
    fail "No victim stats to check migrations"
fi

# ─── Test 7: PSI canary signal (Layer 1 simulation) ──────────────────────────

header "Test 7: Layer 1 Canary Signals (PSI + Throttle)"

PSI_VAL=$(read_psi "$VICTIM_CG")
THROTTLE_VAL=$(read_throttle "$VICTIM_CG")
info "Victim cgroup PSI avg10: $PSI_VAL"
info "Victim cgroup throttled_usec: $THROTTLE_VAL"

# With 64+64 threads running, PSI should be elevated on the victim cgroup
if (( $(echo "$PSI_VAL > 0" | bc -l) )); then
    pass "PSI is non-zero for contended cgroup ($PSI_VAL%)"
else
    warn "PSI is zero — workload may not be generating enough pressure"
fi

if [[ "$THROTTLE_VAL" -gt 0 ]]; then
    pass "Throttle counter is non-zero ($THROTTLE_VAL usec)"
else
    warn "No throttling detected"
fi

# ─── Test 8: Watchlist clear stops stats collection ───────────────────────────

header "Test 8: Watchlist Clear"

# Clear watchlist
sysprobe_post "/watchlist" '{"cgroup_ids":[]}'
info "Cleared watchlist"

# Flush existing data
sysprobe_get "/check" > /dev/null
sleep "$SETTLE_TIME"

STATS=$(sysprobe_get "/check")
CGROUP_COUNT=$(echo "$STATS" | jq '(.cgroup_stats // []) | length')

if [[ "$CGROUP_COUNT" -eq 0 ]] || [[ "$CGROUP_COUNT" == "null" ]]; then
    pass "No stats after clearing watchlist"
else
    fail "Still getting $CGROUP_COUNT cgroup stats after clearing watchlist"
fi

# ─── Test 9: Quiet cgroup (no contention baseline) ───────────────────────────

header "Test 9: Quiet Cgroup Baseline"

# Run minimal workload in quiet cgroup
bash -c "echo \$\$ > ${CGROUP_ROOT}/${QUIET_CG}/cgroup.procs && exec stress-ng --cpu 1 --timeout ${STRESS_DURATION}s --quiet" &
QUIET_PID=$!
PIDS_TO_KILL+=("$QUIET_PID")

# Watch quiet cgroup
sysprobe_post "/watchlist" "{\"cgroup_ids\":[${QUIET_INODE}]}"
sleep "$SETTLE_TIME"
sysprobe_get "/check" > /dev/null
sleep "$SETTLE_TIME"

STATS=$(sysprobe_get "/check")
QUIET_STATS=$(echo "$STATS" | jq "(.cgroup_stats // [])[] | select(.CgroupID == $QUIET_INODE)")

if [[ -n "$QUIET_STATS" ]]; then
    Q_FOREIGN=$(echo "$QUIET_STATS" | jq '.ForeignPreemptionCount')
    Q_SELF=$(echo "$QUIET_STATS" | jq '.SelfPreemptionCount')
    Q_LATENCY=$(echo "$QUIET_STATS" | jq '.SumLatenciesNs')
    Q_EVENTS=$(echo "$QUIET_STATS" | jq '.EventCount')

    info "Quiet cgroup: events=$Q_EVENTS, latency=${Q_LATENCY}ns, foreign=$Q_FOREIGN, self=$Q_SELF"

    if [[ "$Q_FOREIGN" -eq 0 ]] || [[ "$Q_FOREIGN" -lt 10 ]]; then
        pass "Quiet cgroup has minimal foreign preemptions ($Q_FOREIGN)"
    else
        warn "Quiet cgroup has unexpected foreign preemptions ($Q_FOREIGN) — host may be busy"
    fi
else
    info "No stats for quiet cgroup (may not have been scheduled on watched CPUs)"
fi

# ─── Test 10: Watchlist update replaces (doesn't append) ─────────────────────

header "Test 10: Watchlist Replacement"

# Set watchlist to victim only
sysprobe_post "/watchlist" "{\"cgroup_ids\":[${VICTIM_INODE}]}"
sleep 1

# Replace with noisy only
sysprobe_post "/watchlist" "{\"cgroup_ids\":[${NOISY_INODE}]}"
sleep "$SETTLE_TIME"
sysprobe_get "/check" > /dev/null
sleep "$SETTLE_TIME"

STATS=$(sysprobe_get "/check")
VICTIM_IN_STATS=$(echo "$STATS" | jq "[(.cgroup_stats // [])[] | select(.CgroupID == $VICTIM_INODE)] | length")
NOISY_IN_STATS=$(echo "$STATS" | jq "[(.cgroup_stats // [])[] | select(.CgroupID == $NOISY_INODE)] | length")

if [[ "$VICTIM_IN_STATS" -eq 0 ]]; then
    pass "Victim cgroup NOT in stats after watchlist replacement"
else
    fail "Victim cgroup still in stats after being removed from watchlist"
fi

if [[ "$NOISY_IN_STATS" -gt 0 ]]; then
    pass "Noisy cgroup IS in stats after watchlist replacement"
else
    warn "Noisy cgroup not in stats (may not have generated events yet)"
fi

# ─── Summary ─────────────────────────────────────────────────────────────────

header "Summary"

if [[ "$FAILURES" -eq 0 ]]; then
    echo -e "${GREEN}All tests passed!${NC}"
else
    echo -e "${RED}${FAILURES} test(s) failed.${NC}"
fi

exit "$FAILURES"
