#ifndef _HELPERS_SYSCALLS_H_
#define _HELPERS_SYSCALLS_H_

#include "constants/custom.h"
#include "maps.h"

#include "events.h"
#include "activity_dump.h"
#include "span.h"

void __attribute__((always_inline)) collect_syscall_ctx(struct syscall_cache_t *syscall, const char *str1, const char *str2, s64 n1, s64 n2) {
    u32 key = 0;
    char *data = bpf_map_lookup_elem(&syscall_ctx_gen, &key);
    if (!data) {
        return;
    }

    if (str1) {
        bpf_probe_read_str(data, MAX_SYSCALL_STR_CTX, str1);
    } else {
        data[0] = 0;
    }

    if (str2) {
        bpf_probe_read_str(&data[MAX_SYSCALL_STR_CTX], MAX_SYSCALL_STR_CTX, str2);
    } else {
        data[MAX_SYSCALL_STR_CTX] = 0;
    }

    data[MAX_SYSCALL_CTX_SIZE-sizeof(s64)*2] = n1;
    data[MAX_SYSCALL_CTX_SIZE-sizeof(s64)] = n2;

    u32 id = rand32();
    syscall->ctx_id = id;

    bpf_map_update_elem(&syscall_ctx, &id, data, BPF_ANY);
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
    u64 key = bpf_get_current_pid_tgid();
    bpf_map_update_elem(&syscalls, &key, syscall, BPF_ANY);

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
#ifdef DEBUG
    if (!syscall) {
        bpf_printk("Failed to pop syscall with type %d", type);
    }
#endif
    return syscall;
}

int __attribute__((always_inline)) discard_syscall(struct syscall_cache_t *syscall) {
    u64 key = bpf_get_current_pid_tgid();
    bpf_map_delete_elem(&syscalls, &key);
    monitor_syscalls(syscall->type, -1);
    return 0;
}

int __attribute__((always_inline)) mark_as_discarded(struct syscall_cache_t *syscall) {
    syscall->discarded = 1;
    return 0;
}

int __attribute__((always_inline)) filter_syscall(struct syscall_cache_t *syscall, int (*check_approvers)(struct syscall_cache_t *syscall)) {
    if (syscall->policy.mode == NO_FILTER) {
        return 0;
    }

    char pass_to_userspace = syscall->policy.mode == ACCEPT ? 1 : 0;

    if (syscall->policy.mode == DENY) {
        pass_to_userspace = check_approvers(syscall);
    }

    u32 tgid = bpf_get_current_pid_tgid() >> 32;
    u64 *cookie = bpf_map_lookup_elem(&traced_pids, &tgid);
    if (cookie != NULL) {
        u64 now = bpf_ktime_get_ns();
        struct activity_dump_config *config = lookup_or_delete_traced_pid(tgid, now, cookie);
        if (config != NULL) {
            // is this event type traced ?
            if (mask_has_event(config->event_mask, syscall->type)
                && activity_dump_rate_limiter_allow(config, *cookie, now, 0)) {
                if (!pass_to_userspace) {
                    syscall->resolver.flags |= SAVED_BY_ACTIVITY_DUMP;
                }
                return 0;
            }
        }
    }

    return !pass_to_userspace;
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
