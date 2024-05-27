#ifndef _HOOKS_RAW_SYSCALLS_H
#define _HOOKS_RAW_SYSCALLS_H

#include "structs/security_profile.h"
#include "helpers/activity_dump.h"
#include "helpers/raw_syscalls.h"
#include "helpers/syscalls.h"

SEC("tracepoint/raw_syscalls/sys_enter")
int sys_enter(struct _tracepoint_raw_syscalls_sys_enter *args) {
    struct syscall_monitor_entry_t zero = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    u64 now = bpf_ktime_get_ns();

    send_signal(pid);

    struct syscall_monitor_event_t event = {};
    struct proc_cache_t *proc_cache_entry = fill_process_context(&event.process);
    fill_container_context(proc_cache_entry, &event.container);

    // check if this event should trigger a syscall drift event
    if (is_anomaly_syscalls_enabled() && event.container.container_id[0] != 0) {
        // fetch the profile for the current container
        struct security_profile_t *profile = bpf_map_lookup_elem(&security_profiles, &event.container);
        if (profile) {
            u64 cookie = profile->cookie;
            struct security_profile_syscalls_t *syscalls = bpf_map_lookup_elem(&secprofs_syscalls, &cookie);
            if (syscalls) {
                // fetch the current syscall monitor entry
                struct syscall_monitor_entry_t *entry = fetch_sycall_monitor_entry(&zero, pid, now, SYSCALL_MONITOR_TYPE_DRIFT);
                if (entry == NULL) {
                    // should never happen
                    return 0;
                }
                // is the current syscall in the profile ?
                if (!syscall_mask_contains(syscalls->syscalls, args->id)) {
                    syscall_monitor_entry_insert(entry, args->id);
                }
                // send an event if need be
                event.event.flags = EVENT_FLAGS_ANOMALY_DETECTION_EVENT;
                send_or_skip_syscall_monitor_event(args, &event, entry, &zero, SYSCALL_MONITOR_TYPE_DRIFT);
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
                return 0;
            }
            // insert the current syscall in the map
            syscall_monitor_entry_insert(entry, args->id);
            // send an event if need be
            event.event.flags = EVENT_FLAGS_ACTIVITY_DUMP_SAMPLE;
            send_or_skip_syscall_monitor_event(args, &event, entry, &zero, SYSCALL_MONITOR_TYPE_DUMP);
        }
    }

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
    u64 fallback;
    LOAD_CONSTANT("tracepoint_raw_syscall_fallback", fallback);
    if (fallback) {
        handle_sys_exit(args);
    }
    return 0;
}

#endif
