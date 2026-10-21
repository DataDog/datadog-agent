// HotSpot JVM memory pools and GC pauses, from the HotSpot USDT probes
// (provider "hotspot") of libjvm.so:
//
//   gc__begin(bool full)        -> start of a stop-the-world pause
//   gc__end()                   -> end of that pause
//   mem__pool__gc__end(char *manager, size_t manager_len, char *pool, size_t pool_len,
//                      size_t init, size_t used, size_t committed, size_t max)
//                               -> state of one memory pool at the end of a collection
//
// The probes are attached system-wide (no PID filter) by system-probe, at the
// file offsets read from the .note.stapsdt ELF notes of every libjvm.so found
// on the host. The location of each USDT argument is described by the spec
// stored in usdt_specs, whose index is patched per program clone (usdt.h).
//
// Every handler is idempotent: the same probe site can be attached twice
// (e.g. the same libjvm.so inode seen through two overlayfs mounts), so running
// a handler twice for one probe hit must not change the result.

#include "ktypes.h"
#include "bpf_metadata.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "map-defs.h"
#include "bpf_telemetry.h"
#include "usdt.h"

#include "jvm-memory-kern-user.h"

BPF_HASH_MAP(jvm_pools, jvm_pool_key_t, jvm_pool_value_t, 8192)
BPF_HASH_MAP(jvm_gc, __u32, jvm_gc_stats_t, 1024)

// Maximum depth of the PID namespace walk, see tgid_in_pidns.
#define JVM_MAX_PIDNS_LEVEL 16

// mem__pool__gc__end argument indexes (0-based)
#define MEM_POOL_ARG_POOL_NAME 2
#define MEM_POOL_ARG_INIT 4
#define MEM_POOL_ARG_USED 5
#define MEM_POOL_ARG_COMMITTED 6
#define MEM_POOL_ARG_MAX 7

// Layout of task_struct before Linux 4.19, which introduced task_struct.thread_pid.
struct pid_link___jvm_old {
    struct hlist_node node;
    struct pid *pid;
} __attribute__((preserve_access_index));

struct task_struct___jvm_old {
    struct pid_link___jvm_old pids[3];
} __attribute__((preserve_access_index));

// Returns the struct pid of the thread group leader of task.
static __always_inline struct pid *get_task_tgid_pid(struct task_struct *task) {
    struct task_struct *leader = BPF_CORE_READ(task, group_leader);
    if (bpf_core_field_exists(leader->thread_pid)) {
        return BPF_CORE_READ(leader, thread_pid);
    }
    struct task_struct___jvm_old *old_leader = (void *)leader;
    return BPF_CORE_READ(old_leader, pids[0].pid); // PIDTYPE_PID
}

// Looks up the tgid of the current task in the PID namespace whose inode
// number is pidns_ino. That namespace is the one of the procfs used by
// system-probe; it may be an ancestor of the PID namespace of the task (a JVM
// in a container), which bpf_get_ns_current_pid_tgid cannot resolve: that
// helper only matches the namespace of the task itself.
//
// struct pid holds one struct upid per namespace level, from the root (0) to
// the namespace of the task (pid->level).
static __always_inline bool tgid_in_pidns(__u64 pidns_ino, __u32 *tgid) {
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    struct pid *pid = get_task_tgid_pid(task);
    if (!pid) {
        return false;
    }

    unsigned int level = BPF_CORE_READ(pid, level);

    // The CO-RE relocations are resolved out of the loop, which clang cannot
    // unroll otherwise. struct upid lost a member in Linux 4.15, so its size
    // is relocated too.
    __u64 upid_size = bpf_core_type_size(struct upid);
    __u64 upid_nr_off = bpf_core_field_offset(struct upid, nr);
    __u64 upid_ns_off = bpf_core_field_offset(struct upid, ns);
    __u64 ns_inum_off = bpf_core_field_offset(struct pid_namespace, ns.inum);
    void *numbers = (void *)pid + bpf_core_field_offset(struct pid, numbers);

    // Note: clang cannot unroll the loop if it exits early on the level check.
#pragma unroll
    for (int i = 0; i < JVM_MAX_PIDNS_LEVEL; i++) {
        if (i <= level) {
            void *upid = numbers + i * upid_size;
            void *pidns = NULL;
            unsigned int inum = 0;
            if (bpf_probe_read_kernel(&pidns, sizeof(pidns), upid + upid_ns_off) < 0 || !pidns ||
                bpf_probe_read_kernel(&inum, sizeof(inum), pidns + ns_inum_off) < 0) {
                return false;
            }
            if (inum == (unsigned int)pidns_ino) {
                int nr = 0;
                if (bpf_probe_read_kernel(&nr, sizeof(nr), upid + upid_nr_off) < 0) {
                    return false;
                }
                *tgid = (__u32)nr;
                return true;
            }
        }
    }
    return false;
}

// Returns the tgid of the current task, in the PID namespace of the procfs
// used by system-probe. Returns false when the task is not visible in that
// namespace: the event must be ignored.
static __always_inline bool get_tgid(__u32 *tgid) {
    // 0 when the procfs used by system-probe is in the root PID namespace,
    // otherwise the inode number of its PID namespace.
    __u64 pidns_ino = 0;
    LOAD_CONSTANT("jvm_pidns_ino", pidns_ino);
    if (pidns_ino == 0) {
        *tgid = bpf_get_current_pid_tgid() >> 32;
        return true;
    }
    return tgid_in_pidns(pidns_ino, tgid);
}

// Lookup-or-init of the GC row of a JVM. The insertion uses BPF_NOEXIST so
// that two CPUs racing on the first event of a JVM cannot overwrite each other.
static __always_inline jvm_gc_stats_t *get_or_create_gc_stats(__u32 tgid) {
    jvm_gc_stats_t *stats = bpf_map_lookup_elem(&jvm_gc, &tgid);
    if (stats) {
        return stats;
    }
    jvm_gc_stats_t zero = {};
    bpf_map_update_with_telemetry(jvm_gc, &tgid, &zero, BPF_NOEXIST, -EEXIST);
    return bpf_map_lookup_elem(&jvm_gc, &tgid);
}

SEC("uprobe/hotspot_gc_begin")
int uprobe__hotspot_gc_begin(struct pt_regs *ctx) {
    __u32 tgid = 0;
    if (!get_tgid(&tgid)) {
        return 0;
    }

    // The flag is a 1-byte argument (e.g. 1@%dil, 1@$1): usdt_arg always
    // writes a full long, which is then truncated.
    long full = 0;
    usdt_arg(ctx, 0, &full); // 0 on error

    jvm_gc_stats_t *stats = get_or_create_gc_stats(tgid);
    if (!stats) {
        return 0;
    }
    // Pauses run on the single VMThread of the JVM, one at a time, so the row
    // is never updated concurrently by two pauses of the same JVM.
    stats->pause_full = (full & 0xff) != 0;
    stats->pause_start_ns = bpf_ktime_get_ns();
    return 0;
}

SEC("uprobe/hotspot_gc_end")
int uprobe__hotspot_gc_end(struct pt_regs *ctx) {
    __u32 tgid = 0;
    if (!get_tgid(&tgid)) {
        return 0;
    }

    jvm_gc_stats_t *stats = bpf_map_lookup_elem(&jvm_gc, &tgid);
    // No pause in flight: the pause started before the probes were attached,
    // or this is a duplicate invocation for the same probe hit.
    if (!stats || stats->pause_start_ns == 0) {
        return 0;
    }

    __u64 now = bpf_ktime_get_ns();
    __u64 start = stats->pause_start_ns;
    stats->pause_start_ns = 0;
    if (now > start) {
        stats->pause_time_ns += now - start;
    }
    stats->pauses++;
    if (stats->pause_full) {
        stats->full_pauses++;
    }
    stats->last_pause_end_ns = now;
    return 0;
}

SEC("uprobe/hotspot_mem_pool_gc_end")
int uprobe__hotspot_mem_pool_gc_end(struct pt_regs *ctx) {
    __u32 tgid = 0;
    if (!get_tgid(&tgid)) {
        return 0;
    }

    // The memory manager name (argument 0) is deliberately ignored: every
    // memory manager reports every pool of the JVM, so keying on it would
    // report each pool several times.
    long pool_name = 0;
    if (usdt_arg(ctx, MEM_POOL_ARG_POOL_NAME, &pool_name) < 0 || pool_name == 0) {
        return 0;
    }

    jvm_pool_key_t key = {};
    key.tgid = tgid;
    // HotSpot builds the name with strlen(), so it is NUL-terminated. The
    // buffer is zeroed first so that the bytes after the name are deterministic.
    long name_len = bpf_probe_read_user_str_with_telemetry(key.name, sizeof(key.name), (void *)pool_name);
    if (name_len <= 1) {
        // read error, or empty name
        return 0;
    }

    long init = 0, used = 0, committed = 0, max = 0;
    if (usdt_arg(ctx, MEM_POOL_ARG_INIT, &init) < 0 ||
        usdt_arg(ctx, MEM_POOL_ARG_USED, &used) < 0 ||
        usdt_arg(ctx, MEM_POOL_ARG_COMMITTED, &committed) < 0 ||
        usdt_arg(ctx, MEM_POOL_ARG_MAX, &max) < 0) {
        return 0;
    }

    // Absolute values, last writer wins: idempotent.
    jvm_pool_value_t value = {
        .init = (__u64)init,
        .used = (__u64)used,
        .committed = (__u64)committed,
        .max = (__u64)max,
        .timestamp = bpf_ktime_get_ns(),
    };
    bpf_map_update_with_telemetry(jvm_pools, &key, &value, BPF_ANY);
    return 0;
}

char _license[] SEC("license") = "GPL";
