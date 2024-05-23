#ifndef _HELPERS_RAW_SYSCALLS_H_
#define _HELPERS_RAW_SYSCALLS_H_

#include "maps.h"

__attribute__((always_inline)) u8 is_syscall(struct syscall_table_key_t *key) {
    u8 *ok = bpf_map_lookup_elem(&syscall_table, key);
    return (u8)(ok != NULL);
}

__attribute__((always_inline)) u8 syscall_mask_contains(char syscalls[SYSCALL_ENCODING_TABLE_SIZE], long syscall_id) {
    u16 index = ((unsigned long) syscall_id) / 8;
    u8 bit = 1 << (((unsigned long) syscall_id) % 8);
    return (syscalls[index & (SYSCALL_ENCODING_TABLE_SIZE - 1)] & bit) > 0;
}

__attribute__((always_inline)) void syscall_monitor_entry_insert(struct syscall_monitor_entry_t *entry, long syscall_id) {
    u16 index = ((unsigned long) syscall_id) / 8;
    u8 bit = 1 << (((unsigned long) syscall_id) % 8);
    if ((entry->syscalls[index & (SYSCALL_ENCODING_TABLE_SIZE - 1)] & bit) == 0) {
        entry->dirty = 1;
        // insert new syscall
        entry->syscalls[index & (SYSCALL_ENCODING_TABLE_SIZE - 1)] |= bit;
    }
}

__attribute__((always_inline)) struct syscall_monitor_entry_t *fetch_sycall_monitor_entry(struct syscall_monitor_entry_t *zero, u32 pid, u64 now, u8 syscall_monitor_type) {
    struct syscall_monitor_key_t key = {
        .type = syscall_monitor_type,
        .pid = pid,
    };
    struct syscall_monitor_entry_t *entry = bpf_map_lookup_elem(&syscall_monitor, &key);
    if (entry == NULL) {
        bpf_map_update_elem(&syscall_monitor, &key, zero, BPF_NOEXIST);
        entry = bpf_map_lookup_elem(&syscall_monitor, &key);
        if (entry == NULL) {
            // should not happen, ignore
            return NULL;
        }
        // in order to prevent sending an event immediately, set the last_sent to now
        entry->last_sent = now;
    }
    return entry;
}

__attribute__((always_inline)) void delete_syscall_monitor_entry(u32 pid, u8 syscall_monitor_type) {
    struct syscall_monitor_key_t key = {
        .type = syscall_monitor_type,
        .pid = pid,
    };
    bpf_map_delete_elem(&syscall_monitor, &key);
}

__attribute__((always_inline)) void send_or_skip_syscall_monitor_event(struct _tracepoint_raw_syscalls_sys_enter *args, struct syscall_monitor_event_t *event, struct syscall_monitor_entry_t *entry, struct syscall_monitor_entry_t *zero, u8 syscall_monitor_type) {
    u8 should_send = 0;
    u64 now = bpf_ktime_get_ns();
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;

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
            should_send = 2;
            goto shoud_send_event;
        }
        key.syscall_key = EXECVE_SYSCALL_KEY;
        if (is_syscall(&key)) {
            // a new process is about to exec, flush the existing syscalls now
            should_send = 3;
        }
    }

shoud_send_event:
    if (should_send > 0) {

        // send an event now
        event->syscall_data.syscalls = *entry;

        // reset the syscalls mask for the drift monitor type
        if (syscall_monitor_type == SYSCALL_MONITOR_TYPE_DRIFT) {
            *entry = *zero;
        }

        // regardless if we successfully send the event, update the "last_sent" field to avoid spamming the perf map
        entry->last_sent = now;
        entry->dirty = 0;

        // fill span context
        fill_span_context(&event->span);

        // remove last_sent and dirty from the event size, we don't care about these fields
        send_event_with_size_ptr(args, EVENT_SYSCALLS, event, offsetof(struct syscall_monitor_event_t, syscall_data) + SYSCALL_ENCODING_TABLE_SIZE);
    }

    key.syscall_key = EXECVE_SYSCALL_KEY;
    if (is_syscall(&key)) {
        // reset syscalls map for the new process
        bpf_probe_read(&entry->syscalls[0], sizeof(entry->syscalls), &zero->syscalls[0]);
        entry->dirty = 1;
        entry->last_sent = now;
    }
    key.syscall_key = EXIT_SYSCALL_KEY;
    if (is_syscall(&key)) {
        // is the process exiting ?
        if (pid == (u32)pid_tgid) {
            // delete entry from map
            delete_syscall_monitor_entry(pid, syscall_monitor_type);
        }
    }
}

#endif
