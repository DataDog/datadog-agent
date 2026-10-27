#ifndef _HOOKS_RAW_SYSCALLS_H
#define _HOOKS_RAW_SYSCALLS_H

#include "structs/security_profile.h"
#include "helpers/activity_dump.h"
#include "helpers/approvers.h"
#include "helpers/process.h"
#include "helpers/raw_syscalls.h"
#include "helpers/span.h"
#include "helpers/span_fill.h"
#include "helpers/syscall_usage.h"
#include "helpers/syscalls.h"

static __attribute__((always_inline)) void monitor_syscall(struct _tracepoint_raw_syscalls_sys_enter *args, u32 pid) {
    struct syscall_monitor_entry_t zero = {};
    u64 now = bpf_ktime_get_ns();

    // syscall_monitor_event_t lives in a per-CPU map rather than on the stack.
    // We're reusing the SPAN_FILL per-CPU map instead of using another one, but we're
    // not filling the span context here: this event represents a batch of syscall,
    // the span context is not useful here
    struct syscall_monitor_event_t *event = SPAN_FILL_EVENT(struct syscall_monitor_event_t, EVENT_SYSCALLS);
    if (!event) {
        return;
    }

    struct proc_cache_t *proc_cache_entry = fill_process_context(&event->process);
    fill_cgroup_context(proc_cache_entry, &event->cgroup);

    u8 drift_active = 0;
    u8 dump_active = 0;
    u8 drift_reason = SYSCALL_MONITOR_REASON_NONE;
    u8 dump_reason = SYSCALL_MONITOR_REASON_NONE;

    // check if this event should trigger a syscall drift event
    if (is_anomaly_syscalls_enabled()) {
        // fetch the profile for the current cgroup
        struct security_profile_t *profile = bpf_map_lookup_elem(&security_profiles, &event->cgroup.path_key.ino);
        if (profile) {
            u64 cookie = profile->cookie;
            struct security_profile_syscalls_t *syscalls = bpf_map_lookup_elem(&secprofs_syscalls, &cookie);
            if (syscalls) {
                // fetch the current syscall monitor entry
                struct syscall_monitor_entry_t *entry = fetch_sycall_monitor_entry(&zero, pid, now, SYSCALL_MONITOR_TYPE_DRIFT);
                if (entry == NULL) {
                    // should never happen
                    return;
                }
                drift_active = 1;
                // is the current syscall in the profile ?
                if (!syscall_mask_contains(syscalls->syscalls, args->id)) {
                    syscall_monitor_entry_insert(entry, args->id);
                }
                drift_reason = syscall_monitor_should_send(args, entry, now);
            }
        }
    }

    // are we dumping the syscalls of this process ?
    struct activity_dump_config *config = lookup_or_delete_traced_pid(pid, now, NULL);
    if (config) {
        if (mask_has_event(config->event_mask, EVENT_SYSCALLS)) {
            // fetch the current syscall monitor entry
            struct syscall_monitor_entry_t *entry = fetch_sycall_monitor_entry(&zero, pid, now, SYSCALL_MONITOR_TYPE_DUMP);
            if (entry == NULL) {
                // should never happen
                return;
            }
            dump_active = 1;
            // insert the current syscall in the map
            syscall_monitor_entry_insert(entry, args->id);
            dump_reason = syscall_monitor_should_send(args, entry, now);
        }
    }

    // A NULL peek means another thread exited and dropped the entry: nothing to flush.
    if (drift_active) {
        struct syscall_monitor_entry_t *drift_entry = peek_syscall_monitor_entry(pid, SYSCALL_MONITOR_TYPE_DRIFT);
        if (drift_entry != NULL) {
            if (drift_reason) {
                event->event.flags = EVENT_FLAGS_ANOMALY_DETECTION_EVENT;
                event->event_reason = drift_reason;
                syscall_monitor_flush_entry(event, drift_entry, &zero, now, SYSCALL_MONITOR_TYPE_DRIFT);
                send_event_ptr(args, EVENT_SYSCALLS, event);
            }
            syscall_monitor_post_syscall(args, drift_entry, &zero, now, SYSCALL_MONITOR_TYPE_DRIFT);
        }
    }

    if (dump_active) {
        struct syscall_monitor_entry_t *dump_entry = peek_syscall_monitor_entry(pid, SYSCALL_MONITOR_TYPE_DUMP);
        if (dump_entry != NULL) {
            if (dump_reason) {
                event->event.flags = EVENT_FLAGS_ACTIVITY_DUMP_SAMPLE;
                event->event_reason = dump_reason;
                syscall_monitor_flush_entry(event, dump_entry, &zero, now, SYSCALL_MONITOR_TYPE_DUMP);
                send_event_ptr(args, EVENT_SYSCALLS, event);
            }
            syscall_monitor_post_syscall(args, dump_entry, &zero, now, SYSCALL_MONITOR_TYPE_DUMP);
        }
    }
}

// Workload profiles v2 syscall sampler. It runs on every syscall of the host and only records the
// syscall in the process bitmap: syscall_usage_ticker and the exec/exit hooks send the events.
static __attribute__((always_inline)) void sample_syscall(struct _tracepoint_raw_syscalls_sys_enter *args, u32 pid) {
    // Fast-exit high-frequency, low-signal syscalls (read/write/futex/poll/...). Userspace
    // seeds them into profiles so a KILL-default seccomp profile stays valid.
    if (is_sampling_ignored_syscall(args->id)) {
        return;
    }

    record_syscall_usage(pid, args->id);
}

SEC("tracepoint/raw_syscalls/sys_enter")
int sys_enter(struct _tracepoint_raw_syscalls_sys_enter *args) {
    u32 pid = bpf_get_current_pid_tgid() >> 32;

    // handle kill action
    if (is_raw_syscall_kill_enabled() || is_syscall_monitor_enabled()) {
        send_signal(pid);
    }

    if (is_syscall_monitor_enabled()) {
        monitor_syscall(args, pid);
    }

    if (is_event_sampling_syscalls_enabled()) {
        sample_syscall(args, pid);
    }

    return 0;
}

struct syscall_usage_ticker_ctx_t {
    struct bpf_perf_event_data *ctx;
    u64 now;
    u64 refresh_period;
};

static long for_each_syscall_usage_cb(struct bpf_map *map, const void *key, void *value, void *callback_ctx) {
    struct syscall_usage_ticker_ctx_t *ticker_ctx = (struct syscall_usage_ticker_ctx_t *)callback_ctx;
    struct syscall_usage_t *usage = (struct syscall_usage_t *)value;

    // an entry created on another CPU after the ticker read the time
    if (usage->excluded || ticker_ctx->now < usage->last_sent) {
        return 0;
    }

    u64 elapsed = ticker_ctx->now - usage->last_sent;
    if ((usage->new_syscalls && elapsed >= SYSCALL_USAGE_NEW_SYSCALL_FLUSH_DELAY) ||
        (ticker_ctx->refresh_period > 0 && elapsed >= ticker_ctx->refresh_period)) {
        send_syscall_usage_event(ticker_ctx->ctx, *(u32 *)key, usage, SYSCALL_MONITOR_REASON_PERIOD, SYSCALL_USAGE_EVENT_TICKER_SLOT);
    }

    return 0;
}

SEC("perf_event/cpu_clock")
int syscall_usage_ticker(struct bpf_perf_event_data *ctx) {
    // we want a single core to trigger syscall usage events
    if (bpf_get_smp_processor_id() > 0) {
        return 0;
    }

    u64 sample_refresh_period_ns = 0;
    LOAD_CONSTANT("sample_refresh_period_ns", sample_refresh_period_ns);

    struct syscall_usage_ticker_ctx_t ticker_ctx = {
        .ctx = ctx,
        .now = bpf_ktime_get_ns(),
        .refresh_period = sample_refresh_period_ns,
    };

    bpf_for_each_map_elem(&syscall_usage, &for_each_syscall_usage_cb, &ticker_ctx, 0);

    return 0;
}

// used as a fallback, because tracepoints are not enable when using a ia32 userspace application with a x64 kernel
// cf. https://elixir.bootlin.com/linux/latest/source/arch/x86/include/asm/ftrace.h#L106
int __attribute__((always_inline)) handle_sys_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_ANY);
    if (!syscall) {
        return 0;
    }

    bpf_tail_call_compat(args, &sys_exit_progs, syscall->type);
    return 0;
}

SEC("tracepoint/raw_syscalls/sys_exit")
int sys_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    handle_sys_exit(args);
    return 0;
}

#endif
