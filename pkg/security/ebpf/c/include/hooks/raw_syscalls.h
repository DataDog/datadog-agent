#ifndef _HOOKS_RAW_SYSCALLS_H
#define _HOOKS_RAW_SYSCALLS_H

#include "helpers/approvers.h"
#include "helpers/process.h"
#include "helpers/span.h"
#include "helpers/span_fill.h"
#include "helpers/syscall_usage.h"
#include "helpers/syscalls.h"

SEC("tracepoint/raw_syscalls/sys_enter")
int sys_enter(struct _tracepoint_raw_syscalls_sys_enter *args) {
    u32 pid = bpf_get_current_pid_tgid() >> 32;

    // handle kill action
    if (is_raw_syscall_kill_enabled()) {
        send_signal(pid);
    }

    return 0;
}

// Workload profiles v2 syscall sampler. It runs on every syscall of the host and only records the
// syscall in the process bitmap: syscall_usage_ticker and the exec/exit hooks send the events.
SEC("tracepoint/raw_syscalls/sys_enter")
int sys_enter_syscall_usage(struct _tracepoint_raw_syscalls_sys_enter *args) {
    record_syscall_usage(bpf_get_current_pid_tgid() >> 32, args->id);
    return 0;
}

struct syscall_usage_ticker_ctx_t {
    struct bpf_perf_event_data *ctx;
    u64 now;
    u64 refresh_period;
    u64 max_events;
    // lives in a map: a counter on the stack makes the verifier unroll the callback on 6.7+ kernels
    u64 *sent;
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
        *ticker_ctx->sent += send_syscall_usage_event(ticker_ctx->ctx, *(u32 *)key, usage, SYSCALL_MONITOR_REASON_PERIOD, SYSCALL_USAGE_EVENT_TICKER_SLOT);
        if (ticker_ctx->max_events > 0 && *ticker_ctx->sent >= ticker_ctx->max_events) {
            // the entries left are still due on the next tick, and keep their syscalls until then
            return 1;
        }
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
    u64 max_events = 0;
    LOAD_CONSTANT("syscall_usage_max_events_per_tick", max_events);

    u32 key = 0;
    u64 *sent = bpf_map_lookup_elem(&syscall_usage_ticker_sent, &key);
    if (sent == NULL) {
        return 0;
    }
    *sent = 0;

    struct syscall_usage_ticker_ctx_t ticker_ctx = {
        .ctx = ctx,
        .now = bpf_ktime_get_ns(),
        .refresh_period = sample_refresh_period_ns,
        .max_events = max_events,
        .sent = sent,
    };

    bpf_for_each_map_elem(&syscall_usage, &for_each_syscall_usage_cb, &ticker_ctx, 0);

    return 0;
}

// used as a fallback, because tracepoints are not enable when using a ia32 userspace application with a x64 kernel
// cf. https://elixir.bootlin.com/linux/latest/source/arch/x86/include/asm/ftrace.h#L106
static __always_inline int handle_sys_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
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
