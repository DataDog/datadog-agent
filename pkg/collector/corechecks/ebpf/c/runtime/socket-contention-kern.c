#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "compiler.h"
#include <asm-generic/errno-base.h>
#include "map-defs.h"
#include "socket-contention-kern-user.h"
#include "bpf_metadata.h"

#define SOCKET_CONTENTION_AGGREGATE_KEY 0

struct tstamp_data {
    __u64 timestamp;
    __u64 lock;
    __u32 flags;
};

BPF_HASH_MAP(tstamp, __u32, struct tstamp_data, 1024)
BPF_PERCPU_ARRAY_MAP(tstamp_cpu, struct tstamp_data, 1)
BPF_HASH_MAP(socket_contention_stats, __u32, struct socket_contention_stats, 1)

/* lock contention flags from include/trace/events/lock.h */
#define LCB_F_SPIN  (1U << 0)
#define LCB_F_READ  (1U << 1)
#define LCB_F_WRITE (1U << 2)

/*
 * Returns the scratch timestamp slot for the current contention event.
 * Spin/rw locks stay on-CPU, so we can use a single per-CPU entry; sleeping
 * locks may resume on a different CPU, so they need per-task state instead.
 */
static __always_inline struct tstamp_data *get_tstamp_elem(__u32 flags)
{
    __u32 pid;
    struct tstamp_data *pelem;

    /* Use per-cpu array map for spinlock and rwlock */
    if (flags == (LCB_F_SPIN | LCB_F_READ) || flags == LCB_F_SPIN ||
        flags == (LCB_F_SPIN | LCB_F_WRITE)) {
        __u32 idx = 0;

        pelem = bpf_map_lookup_elem(&tstamp_cpu, &idx);
        if (pelem && pelem->lock)
            pelem = NULL;
        return pelem;
    }

    pid = bpf_get_current_pid_tgid();
    pelem = bpf_map_lookup_elem(&tstamp, &pid);
    if (pelem && pelem->lock)
        return NULL;

    if (!pelem) {
        struct tstamp_data zero = {};

        if (bpf_map_update_elem(&tstamp, &pid, &zero, BPF_NOEXIST) < 0)
            return NULL;

        pelem = bpf_map_lookup_elem(&tstamp, &pid);
        if (!pelem)
            return NULL;
    }

    return pelem;
}

SEC("tp_btf/contention_begin")
int tp_contention_begin(__u64 *ctx)
{
    struct tstamp_data *pelem;

    /* contention_begin passes the contended lock pointer in ctx[0] and the lock flags in ctx[1]. */
    pelem = get_tstamp_elem((__u32)ctx[1]);
    if (!pelem)
        return 0;

    pelem->timestamp = bpf_ktime_get_ns();
    pelem->lock = ctx[0];
    pelem->flags = (__u32)ctx[1];
    return 0;
}

SEC("tp_btf/contention_end")
int tp_contention_end(__u64 *ctx)
{
    __u32 pid = 0, idx = 0;
    __u32 key = SOCKET_CONTENTION_AGGREGATE_KEY;
    struct tstamp_data *pelem;
    struct socket_contention_stats *stats = bpf_map_lookup_elem(&socket_contention_stats, &key);
    __u64 duration;
    bool need_delete = false;

    pelem = bpf_map_lookup_elem(&tstamp_cpu, &idx);
    if (pelem && pelem->lock) {
        if (pelem->lock != ctx[0])
            return 0;
    } else {
        pid = bpf_get_current_pid_tgid();
        pelem = bpf_map_lookup_elem(&tstamp, &pid);
        if (!pelem || pelem->lock != ctx[0])
            return 0;
        need_delete = true;
    }

    duration = bpf_ktime_get_ns() - pelem->timestamp;
    if ((__s64)duration < 0) {
        goto cleanup;
    }

    if (!stats) {
        struct socket_contention_stats zero = {};

        bpf_map_update_elem(&socket_contention_stats, &key, &zero, BPF_NOEXIST);
        stats = bpf_map_lookup_elem(&socket_contention_stats, &key);
        if (!stats)
            goto cleanup;
    }

    if (stats->count == 0) {
        stats->total_time_ns = duration;
        stats->min_time_ns = duration;
        stats->max_time_ns = duration;
        stats->count = 1;
        stats->flags = pelem->flags;
    } else {
        __sync_fetch_and_add(&stats->total_time_ns, duration);
        __sync_fetch_and_add(&stats->count, 1);

        if (stats->max_time_ns < duration)
            stats->max_time_ns = duration;
        if (stats->min_time_ns > duration)
            stats->min_time_ns = duration;
        stats->flags = pelem->flags;
    }

cleanup:
    pelem->lock = 0;
    if (need_delete)
        bpf_map_delete_elem(&tstamp, &pid);
    return 0;
}

char _license[] SEC("license") = "GPL";
