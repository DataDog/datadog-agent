#ifndef _HELPERS_SYSCALL_USAGE_H_
#define _HELPERS_SYSCALL_USAGE_H_

#include "constants/custom.h"
#include "maps.h"
#include "perf_ring.h"

#include "cgroup.h"
#include "process.h"

static __attribute__((always_inline)) struct syscall_usage_t *new_syscall_usage(u32 tgid) {
    struct pid_cache_t *pid_entry = get_pid_cache(tgid);
    if (pid_entry == NULL || pid_entry->cookie == 0) {
        // without an exec cookie, userspace cannot correlate the syscalls with a workload
        return NULL;
    }

    struct syscall_usage_t usage = {
        .cookie = pid_entry->cookie,
        .last_sent = bpf_ktime_get_ns(),
    };

    u64 cgroup_id = get_current_cgroup_id();
    if (cgroup_id == 0) {
        struct proc_cache_t *proc_cache_entry = get_proc_from_cookie(pid_entry->cookie);
        cgroup_id = proc_cache_entry ? proc_cache_entry->cgroup.path_key.ino : 0;
    }
    if (bpf_map_lookup_elem(&excluded_cgroups, &cgroup_id) != NULL) {
        usage.excluded = 1;
    }

    bpf_map_update_elem(&syscall_usage, &tgid, &usage, BPF_NOEXIST);
    return bpf_map_lookup_elem(&syscall_usage, &tgid);
}

// record_syscall_usage runs on every syscall of the host. Once a process used a syscall during the
// current period, it costs a single map lookup and a bit test.
static __attribute__((always_inline)) void record_syscall_usage(u32 tgid, long syscall_id) {
    if ((unsigned long)syscall_id >= SYSCALL_ENCODING_TABLE_SIZE * 8) {
        return;
    }
    u32 index = ((unsigned long)syscall_id / 8) & (SYSCALL_ENCODING_TABLE_SIZE - 1);
    u8 bit = 1 << ((unsigned long)syscall_id % 8);

    struct syscall_usage_t *usage = bpf_map_lookup_elem(&syscall_usage, &tgid);
    if (usage == NULL) {
        usage = new_syscall_usage(tgid);
        if (usage == NULL) {
            return;
        }
    }

    if (usage->excluded || (usage->period[index] & bit)) {
        return;
    }

    usage->period[index] |= bit;
    if (!(usage->seen[index] & bit)) {
        usage->seen[index] |= bit;
        usage->new_syscalls = 1;
    }
}

static __attribute__((always_inline)) int is_syscall_usage_period_empty(struct syscall_usage_t *usage) {
    u64 *words = (u64 *)usage->period;
    u64 used = 0;
#pragma unroll
    for (int i = 0; i < SYSCALL_ENCODING_TABLE_SIZE / sizeof(u64); i++) {
        used |= words[i];
    }
    return used == 0;
}

// send_syscall_usage_event sends the syscalls used by the process since the previous event, and
// starts a new period. It returns 1 when it tried to send an event.
static __attribute__((always_inline)) int send_syscall_usage_event(void *ctx, u32 tgid, struct syscall_usage_t *usage, u64 reason, u32 slot) {
    usage->new_syscalls = 0;
    usage->last_sent = bpf_ktime_get_ns();

    if (usage->excluded || is_syscall_usage_period_empty(usage)) {
        return 0;
    }

    struct pid_cache_t *pid_entry = get_pid_cache(tgid);
    if (pid_entry == NULL || pid_entry->cookie != usage->cookie) {
        // the entry outlived its process (missed exit or exec), start over
        __builtin_memset(usage->seen, 0, sizeof(usage->seen));
        __builtin_memset(usage->period, 0, sizeof(usage->period));
        usage->cookie = pid_entry ? pid_entry->cookie : 0;
        return 0;
    }

    struct syscall_monitor_event_t *event = bpf_map_lookup_elem(&syscall_usage_event, &slot);
    if (event == NULL) {
        return 0;
    }
    __builtin_memset(event, 0, sizeof(*event));

    u64 pid_tgid = ((u64)tgid << 32) | (u64)tgid; // Use tgid as tid
    struct proc_cache_t *proc_cache_entry = fill_process_context_with_pid_tgid(&event->process, pid_tgid);
    fill_cgroup_context(proc_cache_entry, &event->cgroup);

    // userspace excludes a host/systemd cgroup once it receives the first event of one of its processes
    u64 cgroup_id = event->cgroup.path_key.ino;
    if (bpf_map_lookup_elem(&excluded_cgroups, &cgroup_id) != NULL) {
        usage->excluded = 1;
        return 0;
    }

    __builtin_memcpy(event->syscalls, usage->period, sizeof(event->syscalls));
    __builtin_memset(usage->period, 0, sizeof(usage->period));

    event->event.flags = EVENT_FLAGS_ACTIVITY_DUMP_SAMPLE | EVENT_FLAGS_SAVED_BY_AD;
    event->event_reason = reason;
    if (send_event_ptr(ctx, EVENT_SYSCALLS, event) != 0) {
        // the event was dropped: put the syscalls back so that the next tick sends them again
        u64 *period = (u64 *)usage->period;
        u64 *dropped = (u64 *)event->syscalls;
#pragma unroll
        for (int i = 0; i < SYSCALL_ENCODING_TABLE_SIZE / sizeof(u64); i++) {
            period[i] |= dropped[i];
        }
        usage->new_syscalls = 1;
    }
    return 1;
}

// flush_syscall_usage is the last chance to report the syscalls of a process before its exit or exec
static __attribute__((always_inline)) void flush_syscall_usage(void *ctx, u32 tgid, u64 reason) {
    struct syscall_usage_t *usage = bpf_map_lookup_elem(&syscall_usage, &tgid);
    if (usage == NULL) {
        return;
    }

    send_syscall_usage_event(ctx, tgid, usage, reason, SYSCALL_USAGE_EVENT_HOOK_SLOT);
    bpf_map_delete_elem(&syscall_usage, &tgid);
}

#endif /* _HELPERS_SYSCALL_USAGE_H_ */
