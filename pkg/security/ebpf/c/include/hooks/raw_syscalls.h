#ifndef _HOOKS_RAW_SYSCALLS_H
#define _HOOKS_RAW_SYSCALLS_H

#include "helpers/activity_dump.h"
#include "helpers/raw_syscalls.h"
#include "helpers/security_profile.h"
#include "helpers/syscalls.h"

SEC("tracepoint/raw_syscalls/sys_enter")
int sys_enter(struct _tracepoint_raw_syscalls_sys_enter *args) {
    struct syscall_monitor_entry_t zero = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    u64 now = bpf_ktime_get_ns();
    struct syscall_monitor_event_t event = {};

    // check if this workload has a security profile
    if (is_anomaly_syscalls_enabled()) {
        evaluate_security_profile_syscalls(args, &event, args->id);
    }

    // are we dumping the syscalls of this process ?
    struct activity_dump_config *config = lookup_or_delete_traced_pid(pid, now, NULL);
    if (config) {
        if (!mask_has_event(config->event_mask, EVENT_SYSCALLS)) {
            // we're not tracing syscalls, ignore
            return 0;
        }
    } else {
        // we're not tracing this process, ignore
        return 0;
    }

    struct syscall_monitor_entry_t *entry = bpf_map_lookup_elem(&syscall_monitor, &pid);
    if (entry == NULL) {
        bpf_map_update_elem(&syscall_monitor, &pid, &zero, BPF_NOEXIST);
        entry = bpf_map_lookup_elem(&syscall_monitor, &pid);
        if (entry == NULL) {
            // should not happen, ignore
            return 0;
        }
    }

    // compute the offset of the current syscall
    u16 index = ((unsigned long) args->id) / 8;
    u8 bit = 1 << (((unsigned long) args->id) % 8);

    // check if this is a new syscall
    if ((entry->syscalls[index & (SYSCALL_ENCODING_TABLE_SIZE - 1)] & bit) == 0) {
        entry->dirty = 1;
        // insert new syscall
        entry->syscalls[index & (SYSCALL_ENCODING_TABLE_SIZE - 1)] |= bit;
    }

    // check if an event should be sent
    u8 should_send = 0;
    struct syscall_table_key_t key = {
        .id = args->id,
    };
    if (entry->dirty) {
        if (now > entry->last_sent + get_syscall_monitor_event_period()) {
            // it's been a while since we last sent something and the list of syscalls is dirty, send now
            should_send = 1;
            goto shoud_send_event;
        }
        key.syscall_key = EXIT_SYSCALL_KEY;
        if (is_syscall(&key)) {
            // a thread is about to exit and the list of syscalls is dirty, send now
            should_send = 1;
            goto shoud_send_event;
        }
        key.syscall_key = EXECVE_SYSCALL_KEY;
        if (is_syscall(&key)) {
            // a new process is about to exec, flush the existing syscalls now
            should_send = 1;
        }
    }

shoud_send_event:
    if (should_send) {

        // send an event now
        event.syscall_data.syscalls = *entry;
        event.event.flags = EVENT_FLAGS_ACTIVITY_DUMP_SAMPLE; // syscall events are used only by activity dumps

        // regardless if we successfully send the event, update the "last_sent" field to avoid spamming the perf map
        entry->last_sent = now;
        entry->dirty = 0;

        struct proc_cache_t *proc_cache_entry = fill_process_context(&event.process);
        fill_container_context(proc_cache_entry, &event.container);

        // remove last_sent and dirty from the event size, we don't care about these fields
        send_event_with_size_ptr(args, EVENT_SYSCALLS, &event, offsetof(struct syscall_monitor_event_t, syscall_data) + SYSCALL_ENCODING_TABLE_SIZE);
    }

    key.syscall_key = EXECVE_SYSCALL_KEY;
    if (is_syscall(&key)) {
        // reset syscalls map for the new process
        bpf_probe_read(&entry->syscalls[0], sizeof(entry->syscalls), &zero.syscalls[0]);
        entry->dirty = 1;
        entry->last_sent = now;
    }
    key.syscall_key = EXIT_SYSCALL_KEY;
    if (is_syscall(&key)) {
        // is the process exiting ?
        if (pid == (u32)pid_tgid) {
            // delete entry from map
            bpf_map_delete_elem(&syscall_monitor, &pid);
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
