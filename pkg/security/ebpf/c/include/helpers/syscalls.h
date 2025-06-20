#ifndef _HELPERS_SYSCALLS_H_
#define _HELPERS_SYSCALLS_H_

#include "constants/custom.h"
#include "maps.h"

#include "events.h"
#include "activity_dump.h"
#include "span.h"
#include <uapi/linux/filter.h>


#define SYSCALL_CTX_STR_TYPE 1
#define SYSCALL_CTX_INT_TYPE 2

#define SYSCALL_CTX_ARG(type, pos) (type << (pos * 2))
#define SYSCALL_CTX_ARG_STR(pos) SYSCALL_CTX_ARG(SYSCALL_CTX_STR_TYPE, pos)
#define SYSCALL_CTX_ARG_INT(pos) SYSCALL_CTX_ARG(SYSCALL_CTX_INT_TYPE, pos)
#define SYSCALL_CTX_ARG_MASK(pos) (SYSCALL_CTX_ARG_STR(pos) | SYSCALL_CTX_ARG_INT(pos))

#define IS_SYSCALL_CTX_ARG(types, type, pos) (types & (type << (pos * 2)))
#define IS_SYSCALL_CTX_ARG_STR(types, pos) IS_SYSCALL_CTX_ARG(types, SYSCALL_CTX_STR_TYPE, pos)
#define IS_SYSCALL_CTX_ARG_INT(types, pos) IS_SYSCALL_CTX_ARG(types, SYSCALL_CTX_INT_TYPE, pos)

void __attribute__((always_inline)) collect_syscall_ctx(struct syscall_cache_t *syscall, u8 types, void *arg1, void *arg2, void *arg3) {
    u32 key = 0;
    u32 *id = bpf_map_lookup_elem(&syscall_ctx_gen_id, &key);
    if (!id) {
        return;
    }
    __sync_fetch_and_add(id, 1);

    key = *id % MAX_SYSCALL_CTX_ENTRIES;
    char *data = bpf_map_lookup_elem(&syscall_ctx, &key);
    if (!data) {
        return;
    }

    u32 *id_ptr = (u32 *)&data[0];
    id_ptr[0] = *id;

    u8 effective_types = 0;

    if (arg1) {
        effective_types |= (types & SYSCALL_CTX_ARG_MASK(0));
        if (IS_SYSCALL_CTX_ARG_STR(types, 0)) {
            bpf_probe_read_str(&data[5], MAX_SYSCALL_ARG_MAX_SIZE, arg1);
        } else {
            s64 *addr = (s64 *)&data[5];
            addr[0] = *(s64 *)arg1;
        }
    }

    if (arg2) {
        effective_types |= (types & SYSCALL_CTX_ARG_MASK(1));
        if (IS_SYSCALL_CTX_ARG_STR(types, 1)) {
            bpf_probe_read_str(&data[5 + MAX_SYSCALL_ARG_MAX_SIZE], MAX_SYSCALL_ARG_MAX_SIZE, arg2);
        } else {
            s64 *addr = (s64 *)&data[5 + MAX_SYSCALL_ARG_MAX_SIZE];
            addr[0] = *(s64 *)arg2;
        }
    }

    if (arg3) {
        effective_types |= (types & SYSCALL_CTX_ARG_MASK(2));
        if (IS_SYSCALL_CTX_ARG_STR(types, 2)) {
            bpf_probe_read_str(&data[5 + MAX_SYSCALL_ARG_MAX_SIZE * 2], MAX_SYSCALL_ARG_MAX_SIZE, arg3);
        } else {
            s64 *addr = (s64 *)&data[5 + MAX_SYSCALL_ARG_MAX_SIZE * 2];
            addr[0] = *(s64 *)arg3;
        }
    }

    data[4] = effective_types;

    syscall->ctx_id = *id;
}

void __attribute__((always_inline)) monitor_syscalls(u64 event_type, int delta) {
    u64 enabled;
    LOAD_CONSTANT("monitor_syscalls_map_enabled", enabled);

    if (!enabled) {
        return;
    }

    u32 key = 0;
    u32 *value = bpf_map_lookup_elem(&syscalls_stats_enabled, &key);
    if (value == NULL || !*value) {
        return;
    }

    key = event_type;
    struct syscalls_stats_t *stats = bpf_map_lookup_elem(&syscalls_stats, &key);
    if (stats == NULL) {
        return;
    }
    if (delta < 0 && !stats->active) {
        return;
    }
    stats->active = 1;

    __sync_fetch_and_add(&stats->count, delta);
}

struct policy_t __attribute__((always_inline)) fetch_policy(u64 event_type) {
    struct policy_t *policy = bpf_map_lookup_elem(&filter_policy, &event_type);
    if (policy) {
        return *policy;
    }
    struct policy_t empty_policy = {};
    return empty_policy;
}

// cache_syscall checks the event policy in order to see if the syscall struct can be cached
void __attribute__((always_inline)) cache_syscall(struct syscall_cache_t *syscall) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;

    // handle kill action
    send_signal(pid);

    bpf_map_update_elem(&syscalls, &pid_tgid, syscall, BPF_ANY);

    monitor_syscalls(syscall->type, 1);
}

struct syscall_cache_t *__attribute__((always_inline)) peek_task_syscall(u64 pid_tgid, u64 type) {
    struct syscall_cache_t *syscall = (struct syscall_cache_t *)bpf_map_lookup_elem(&syscalls, &pid_tgid);
    if (!syscall) {
        return NULL;
    }
    if (!type || syscall->type == type) {
        return syscall;
    }
    return NULL;
}

struct syscall_cache_t *__attribute__((always_inline)) peek_syscall(u64 type) {
    u64 key = bpf_get_current_pid_tgid();
    return peek_task_syscall(key, type);
}

struct syscall_cache_t *__attribute__((always_inline)) peek_syscall_with(int (*predicate)(u64 type)) {
    u64 key = bpf_get_current_pid_tgid();
    struct syscall_cache_t *syscall = (struct syscall_cache_t *)bpf_map_lookup_elem(&syscalls, &key);
    if (!syscall) {
        return NULL;
    }
    if (predicate(syscall->type)) {
        return syscall;
    }
    return NULL;
}

struct syscall_cache_t *__attribute__((always_inline)) pop_syscall_with(int (*predicate)(u64 type)) {
    u64 key = bpf_get_current_pid_tgid();
    struct syscall_cache_t *syscall = (struct syscall_cache_t *)bpf_map_lookup_elem(&syscalls, &key);
    if (!syscall) {
        return NULL;
    }
    if (predicate(syscall->type)) {
        bpf_map_delete_elem(&syscalls, &key);

        monitor_syscalls(syscall->type, -1);
        return syscall;
    }
    return NULL;
}

struct syscall_cache_t *__attribute__((always_inline)) pop_task_syscall(u64 pid_tgid, u64 type) {
    struct syscall_cache_t *syscall = (struct syscall_cache_t *)bpf_map_lookup_elem(&syscalls, &pid_tgid);
    if (!syscall) {
        return NULL;
    }
    u64 event_type = syscall->type; // fixes 4.14 verifier issue
    if (!type || event_type == type) {
        bpf_map_delete_elem(&syscalls, &pid_tgid);

        monitor_syscalls(event_type, -1);
        return syscall;
    }
    return NULL;
}

struct syscall_cache_t *__attribute__((always_inline)) pop_syscall(u64 type) {
    u64 key = bpf_get_current_pid_tgid();
    struct syscall_cache_t *syscall = pop_task_syscall(key, type);
#if defined(DEBUG_SYSCALLS)
    if (!syscall) {
        bpf_printk("Failed to pop syscall with type %d", type);
    }
#endif
    return syscall;
}

// the following functions must use the {peek,pop}_current_or_impersonated_exec_syscall to retrieve the syscall context
// because the task performing the exec syscall may change its pid in the flush_old_exec() kernel function

struct syscall_cache_t *__attribute__((always_inline)) peek_current_or_impersonated_exec_syscall() {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_EXEC);
    if (!syscall) {
        u64 pid_tgid = bpf_get_current_pid_tgid();
        u32 tgid = pid_tgid >> 32;
        u32 pid = pid_tgid;
        u64 *pid_tgid_execing_ptr = (u64 *)bpf_map_lookup_elem(&exec_pid_transfer, &tgid);
        if (!pid_tgid_execing_ptr) {
            return NULL;
        }
        u64 pid_tgid_execing = *pid_tgid_execing_ptr;
        u32 tgid_execing = pid_tgid_execing >> 32;
        u32 pid_execing = pid_tgid_execing;
        if (tgid != tgid_execing || pid == pid_execing) {
            return NULL;
        }
        // the current task is impersonating its thread group leader
        syscall = peek_task_syscall(pid_tgid_execing, EVENT_EXEC);
    }
    return syscall;
}

struct syscall_cache_t *__attribute__((always_inline)) pop_current_or_impersonated_exec_syscall() {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_EXEC);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;
    u32 pid = pid_tgid;
    u64 *pid_tgid_execing_ptr = (u64 *)bpf_map_lookup_elem(&exec_pid_transfer, &tgid);
    if (pid_tgid_execing_ptr) {
        u64 pid_tgid_execing = *pid_tgid_execing_ptr;
        struct syscall_cache_t *imp_syscall = pop_task_syscall(pid_tgid_execing, EVENT_EXEC);

        u32 tgid_execing = pid_tgid_execing >> 32;
        u32 pid_execing = pid_tgid_execing;
        if (tgid == tgid_execing && pid != pid_execing && !syscall) {
            // the current task is impersonating its thread group leader
            syscall = imp_syscall;
        }
    }

    return syscall;
}

int __attribute__((always_inline)) fill_exec_context() {
    struct syscall_cache_t *syscall = peek_current_or_impersonated_exec_syscall();
    if (!syscall) {
        return 0;
    }

    // call it here before the memory get replaced
    fill_span_context(&syscall->exec.span_context);

    return 0;
}

#endif
