#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "map-defs.h"
#include "syscall-latency-kern-user.h"
#include "bpf_metadata.h"
#include "bpf_telemetry.h"
#include "cgroup.h"

/* Reduced from 65536 to keep total per-TID map memory reasonable now that
 * each entry carries a 128-byte cgroup name (~2 MB at 16384 entries). */
#define MAX_TID_ENTRIES 16384

/* Per-thread entry record — one active syscall per thread at a time. */
struct tid_entry {
    __u64 timestamp_ns;
    __u8  slot;   /* syscall_slot_t value */
    __u8  pad[7]; /* align cgroup_name to 8-byte boundary */
    char  cgroup_name[CGROUP_NAME_LEN];
};

/*
 * Per-TID hash map: stores the entry timestamp and slot for in-flight syscalls.
 * A thread is in at most one syscall at a time, so one entry per TID suffices.
 */
BPF_HASH_MAP(tid_entry, __u32, struct tid_entry, MAX_TID_ENTRIES)

/*
 * Per-CPU hash map of aggregated stats, keyed by (cgroup_name, slot).
 * 4096 entries handles ~240 containers × 17 syscall slots with room to spare.
 * Per-CPU eliminates contention in the hot path; the Go side aggregates
 * across CPUs on read.
 */
BPF_PERCPU_HASH_MAP(syscall_stats, cgroup_stats_key_t, syscall_stats_t, 4096)

/*
 * Map syscall number to our internal slot, or SYSCALL_NOT_TRACKED.
 * Two arch-guarded switch tables — Clang compiles each to a jump table.
 *
 * x86_64: arch/x86/entry/syscalls/syscall_64.tbl
 * arm64:  include/uapi/asm-generic/unistd.h
 *
 * On arm64 some syscalls map to differently-named but semantically equivalent
 * variants (ppoll→POLL, pselect6→SELECT, epoll_pwait→EPOLL_WAIT).  The slot
 * names and metric names remain arch-independent.
 */
static __always_inline __u8 classify_syscall(__u64 nr)
{
#if defined(bpf_target_x86) || defined(__x86_64__)
    switch (nr) {
    case 0:   return SYSCALL_SLOT_READ;
    case 1:   return SYSCALL_SLOT_WRITE;
    case 7:   return SYSCALL_SLOT_POLL;
    case 9:   return SYSCALL_SLOT_MMAP;
    case 11:  return SYSCALL_SLOT_MUNMAP;
    case 17:  return SYSCALL_SLOT_PREAD64;
    case 18:  return SYSCALL_SLOT_PWRITE64;
    case 23:  return SYSCALL_SLOT_SELECT;
    case 42:  return SYSCALL_SLOT_CONNECT;
    case 43:  return SYSCALL_SLOT_ACCEPT;
    case 56:  return SYSCALL_SLOT_CLONE;
    case 59:  return SYSCALL_SLOT_EXECVE;
    case 202: return SYSCALL_SLOT_FUTEX;
    case 232: return SYSCALL_SLOT_EPOLL_WAIT;
    case 281: return SYSCALL_SLOT_EPOLL_PWAIT;
    case 288: return SYSCALL_SLOT_ACCEPT4;
    case 426: return SYSCALL_SLOT_IO_URING;
    default:  return SYSCALL_NOT_TRACKED;
    }
#elif defined(bpf_target_arm64) || defined(__aarch64__)
    switch (nr) {
    case 22:  return SYSCALL_SLOT_EPOLL_WAIT;  /* epoll_pwait on arm64 */
    case 63:  return SYSCALL_SLOT_READ;
    case 64:  return SYSCALL_SLOT_WRITE;
    case 67:  return SYSCALL_SLOT_PREAD64;
    case 68:  return SYSCALL_SLOT_PWRITE64;
    case 72:  return SYSCALL_SLOT_SELECT;      /* pselect6 */
    case 73:  return SYSCALL_SLOT_POLL;        /* ppoll */
    case 98:  return SYSCALL_SLOT_FUTEX;
    case 202: return SYSCALL_SLOT_ACCEPT;
    case 203: return SYSCALL_SLOT_CONNECT;
    case 215: return SYSCALL_SLOT_MUNMAP;
    case 220: return SYSCALL_SLOT_CLONE;
    case 221: return SYSCALL_SLOT_EXECVE;
    case 222: return SYSCALL_SLOT_MMAP;
    case 242: return SYSCALL_SLOT_ACCEPT4;
    case 426: return SYSCALL_SLOT_IO_URING;
    default:  return SYSCALL_NOT_TRACKED;
    }
#else
#error "syscall-latency-kern.c: unsupported architecture (only x86_64 and arm64)"
#endif
}

/*
 * raw_tracepoint/sys_enter
 *
 * args layout:
 *   ctx->args[0]  — struct pt_regs *regs
 *   ctx->args[1]  — long syscall_nr
 *
 * Using raw tracepoints (not tp_btf) so this works on kernels >= 4.17
 * without requiring BTF — broader coverage than lock contention.
 */
SEC("raw_tracepoint/sys_enter")
int raw_tp__sys_enter(struct bpf_raw_tracepoint_args *ctx)
{
    __u64 nr = ctx->args[1];
    __u8 slot = classify_syscall(nr);
    if (slot == SYSCALL_NOT_TRACKED)
        return 0;

    __u32 tid = bpf_get_current_pid_tgid();
    struct tid_entry entry = {
        .timestamp_ns = bpf_ktime_get_ns(),
        .slot         = slot,
    };
    get_cgroup_name(entry.cgroup_name, CGROUP_NAME_LEN);

    bpf_map_update_elem(&tid_entry, &tid, &entry, BPF_ANY);
    return 0;
}

/*
 * raw_tracepoint/sys_exit
 *
 * args layout:
 *   ctx->args[0]  — struct pt_regs *regs
 *   ctx->args[1]  — long return_value
 *
 * We ignore the return value; latency is independent of success/failure.
 */
SEC("raw_tracepoint/sys_exit")
int raw_tp__sys_exit(struct bpf_raw_tracepoint_args *ctx)
{
    __u32 tid = bpf_get_current_pid_tgid();
    struct tid_entry *entry = bpf_map_lookup_elem(&tid_entry, &tid);
    if (!entry)
        return 0;

    __u64 duration = bpf_ktime_get_ns() - entry->timestamp_ns;

    /* Build compound key before deleting the entry (entry pointer becomes
     * invalid after bpf_map_delete_elem). */
    cgroup_stats_key_t key = {};
    key.slot = entry->slot;
    bpf_memcpy(key.cgroup_name, entry->cgroup_name, CGROUP_NAME_LEN);

    /* Clear the entry before updating stats to minimise the window
     * in which a nested or preempted path could see stale data. */
    bpf_map_delete_elem(&tid_entry, &tid);

    /* Guard against clock skew (unlikely but defensive). */
    if ((__s64)duration < 0)
        return 0;

    syscall_stats_t *stats = bpf_map_lookup_elem(&syscall_stats, &key);
    if (!stats) {
        syscall_stats_t zero = {};
        bpf_map_update_elem(&syscall_stats, &key, &zero, BPF_NOEXIST);
        stats = bpf_map_lookup_elem(&syscall_stats, &key);
        if (!stats)
            return 0;
    }

    stats->total_time_ns += duration;
    stats->count         += 1;
    if (duration > SLOW_THRESHOLD_NS)
        stats->slow_count += 1;
    /* max_time_ns: not atomic, worst case we miss an update — acceptable
     * for a gauge that is reset each interval by the Go side. */
    if (stats->max_time_ns < duration)
        stats->max_time_ns = duration;

    return 0;
}

char _license[] SEC("license") = "GPL";
