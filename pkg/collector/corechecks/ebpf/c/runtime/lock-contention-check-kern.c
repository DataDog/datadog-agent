#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "map-defs.h"
#include "lock-contention-check-kern-user.h"
#include "bpf_metadata.h"
#include "bpf_telemetry.h"

#define MAX_TSTAMP_ENTRIES 8192

/* Lock contention flags from include/trace/events/lock.h */
#define LCB_F_SPIN    (1U << 0)
#define LCB_F_READ    (1U << 1)
#define LCB_F_WRITE   (1U << 2)
#define LCB_F_RT      (1U << 3)
#define LCB_F_PERCPU  (1U << 4)
#define LCB_F_MUTEX   (1U << 5)

/* Per-task timestamp data stored in contention_begin, consumed in contention_end */
struct tstamp_data {
    __u64 timestamp_ns;
    __u64 lock;  /* lock address — non-zero means slot is occupied */
    __u32 flags;
};

/* Per-TID hash map for sleeping lock timestamps (mutex, rwsem, rt_mutex).
 * Must NOT be per-CPU: sleeping locks can migrate between CPUs, so
 * contention_end may run on a different CPU than contention_begin. */
BPF_HASH_MAP(tstamp, __u32, struct tstamp_data, MAX_TSTAMP_ENTRIES)

/* Per-CPU array for spinlock timestamps (one slot per CPU, preemption disabled) */
BPF_PERCPU_ARRAY_MAP(tstamp_cpu, struct tstamp_data, 1)

/* Per-CPU array of aggregated stats, indexed by lock_type_key_t */
BPF_PERCPU_ARRAY_MAP(lock_contention_stats, lock_contention_stats_t, LOCK_TYPE_MAX)

/* Classify LCB_F_* flags into lock_type_key_t */
static __always_inline __u32 classify_lock_type(__u32 flags) {
    if (flags & LCB_F_SPIN) {
        if (flags & LCB_F_PERCPU)
            return LOCK_TYPE_PCPU_SPINLOCK;
        if (flags & LCB_F_READ)
            return LOCK_TYPE_RWLOCK_READ;
        if (flags & LCB_F_WRITE)
            return LOCK_TYPE_RWLOCK_WRITE;
        return LOCK_TYPE_SPINLOCK;
    }
    if (flags & LCB_F_MUTEX)
        return LOCK_TYPE_MUTEX;
    if (flags & LCB_F_RT)
        return LOCK_TYPE_RT_MUTEX;
    if (flags & LCB_F_READ)
        return LOCK_TYPE_RWSEM_READ;
    if (flags & LCB_F_WRITE)
        return LOCK_TYPE_RWSEM_WRITE;
    /* flags == 0: pre-6.2 kernels where mutex has no dedicated flag */
    if (flags == 0)
        return LOCK_TYPE_MUTEX;
    return LOCK_TYPE_OTHER;
}

/* Get or create a timestamp element based on lock type.
 * Spinlocks/rwlocks use per-CPU array (preemption is disabled).
 * Sleeping locks use per-TID hash map. */
static __always_inline struct tstamp_data *get_tstamp_elem(__u32 flags) {
    struct tstamp_data *pelem;

    if (flags & LCB_F_SPIN) {
        __u32 idx = 0;
        pelem = bpf_map_lookup_elem(&tstamp_cpu, &idx);
        /* Do not overwrite for nested lock contention */
        if (pelem && pelem->lock)
            return NULL;
        return pelem;
    }

    __u32 tid = bpf_get_current_pid_tgid();
    pelem = bpf_map_lookup_elem(&tstamp, &tid);
    /* Do not overwrite for nested lock contention */
    if (pelem && pelem->lock)
        return NULL;

    if (pelem == NULL) {
        struct tstamp_data zero = {};
        if (bpf_map_update_elem(&tstamp, &tid, &zero, BPF_NOEXIST) < 0)
            return NULL;
        pelem = bpf_map_lookup_elem(&tstamp, &tid);
    }
    return pelem;
}

SEC("tp_btf/contention_begin")
int tracepoint__contention_begin(u64 *ctx)
{
    __u32 flags = (__u32)ctx[1];
    struct tstamp_data *pelem;

    pelem = get_tstamp_elem(flags);
    if (pelem == NULL)
        return 0;

    pelem->timestamp_ns = bpf_ktime_get_ns();
    pelem->lock = ctx[0];
    pelem->flags = flags;

    return 0;
}

SEC("tp_btf/contention_end")
int tracepoint__contention_end(u64 *ctx)
{
    struct tstamp_data *pelem;
    __u32 tid = 0, idx = 0;
    bool need_delete = false;
    __u64 duration;

    /*
     * contention_end does not carry flags, so we cannot know whether the
     * lock was a spinlock or sleeping lock from the tracepoint args alone.
     *
     * Strategy (same as upstream perf lock contention):
     * 1. Check per-CPU map first (spinlocks cannot sleep, so if there's
     *    an active entry it must be for this event).
     * 2. If no per-CPU entry, check per-TID hash (sleeping locks).
     * 3. Verify the lock address matches.
     */
    pelem = bpf_map_lookup_elem(&tstamp_cpu, &idx);
    if (pelem && pelem->lock) {
        if (pelem->lock != (__u64)ctx[0])
            return 0;
    } else {
        tid = bpf_get_current_pid_tgid();
        pelem = bpf_map_lookup_elem(&tstamp, &tid);
        if (!pelem || pelem->lock != (__u64)ctx[0])
            return 0;
        need_delete = true;
    }

    duration = bpf_ktime_get_ns() - pelem->timestamp_ns;
    if ((__s64)duration < 0) {
        pelem->lock = 0;
        if (need_delete)
            bpf_map_delete_elem(&tstamp, &tid);
        return 0;
    }

    /* Classify and update stats */
    __u32 lock_type = classify_lock_type(pelem->flags);
    lock_contention_stats_t *stats = bpf_map_lookup_elem(&lock_contention_stats, &lock_type);
    if (stats) {
        __sync_fetch_and_add(&stats->total_time_ns, duration);
        __sync_fetch_and_add(&stats->count, 1);
        /* max_time_ns: not atomic, but acceptable — worst case we miss
         * an update, which is fine for a gauge that resets each interval */
        if (stats->max_time_ns < duration)
            stats->max_time_ns = duration;
    }

    /* Clear the timestamp slot */
    pelem->lock = 0;
    if (need_delete)
        bpf_map_delete_elem(&tstamp, &tid);
    return 0;
}

char _license[] SEC("license") = "GPL";
