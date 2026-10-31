#ifndef _HELPERS_CAPS_H_
#define _HELPERS_CAPS_H_

// the caller decides whether the monitoring period applies: it rate-limits the periodic ticker,
// but a flush on exec or exit is the last chance to report an entry before it is deleted
static __attribute__((always_inline)) void send_capabilities_usage_event(void *ctx, struct capabilities_usage_key_t *key, struct capabilities_usage_entry_t *entry) {
    if (!is_dirty(entry)) {
        return;
    }

    struct pid_cache_t *pid_entry = get_pid_cache(key->tgid);
    if (!pid_entry) {
        return;
    }

    if (key->cookie != pid_entry->cookie) {
        // skip the entry if it corresponds to a previous process entry (e.g. a different executable)
        return;
    }

    reset_dirty(entry);
    set_last_sent_ns(entry, bpf_ktime_get_ns());

    struct capabilities_event_t event = {
        .caps_usage = entry->usage,
        .cookie = key->cookie,
    };

    u64 pid_tgid = ((u64)key->tgid << 32) | (u64)key->tgid; // Use tgid as tid
    struct proc_cache_t *proc_entry = fill_process_context_with_pid_tgid(&event.process, pid_tgid);
    fill_cgroup_context(proc_entry, &event.cgroup);

    // should we mark this event for activity dumps ?
    struct activity_dump_config *config = lookup_or_delete_traced_pid(event.process.pid, bpf_ktime_get_ns(), NULL);
    if (config && mask_has_event(config->event_mask, EVENT_CAPABILITIES)) {
        event.event.flags |= EVENT_FLAGS_ACTIVITY_DUMP_SAMPLE;
    }

    send_event(ctx, EVENT_CAPABILITIES, event);
}

static __attribute__((always_inline)) void flush_capabilities_usage(void *ctx, u32 tgid, u64 cookie) {
    u64 capabilities_monitoring_enabled = 0;
    LOAD_CONSTANT("capabilities_monitoring_enabled", capabilities_monitoring_enabled);
    if (!capabilities_monitoring_enabled) {
        return;
    }

    struct capabilities_usage_key_t key = {
        .cookie = cookie,
        .tgid = tgid,
    };

    struct capabilities_usage_entry_t *entry = bpf_map_lookup_elem(&capabilities_usage, &key);
    if (!entry) {
        return; // No entry to flush
    }

    send_capabilities_usage_event(ctx, &key, entry);

    bpf_map_delete_elem(&capabilities_usage, &key);
}

// capabilities contexts are only ever accessed by the thread they belong to, tid is the one of the current thread
static __attribute__((always_inline)) struct capabilities_context_t *get_capabilities_context(u32 tid) {
#if USE_SYSCALL_TASK_STORAGE == 1
    u64 use_syscall_task_storage;
    LOAD_CONSTANT("use_syscall_task_storage", use_syscall_task_storage);
    if (use_syscall_task_storage) { // deadcode elimination will remove one of these branches
        return (struct capabilities_context_t *)bpf_task_storage_get(&capabilities_contexts, bpf_get_current_task_btf(), NULL, 0);
    }
#endif
    return (struct capabilities_context_t *)bpf_map_lookup_elem(&capabilities_contexts, &tid);
}

// new_capabilities_context must only be called when the current thread has no context yet: the task storage
// variant doesn't overwrite an existing context
static __attribute__((always_inline)) void new_capabilities_context(u32 tid, struct capabilities_context_t *context) {
#if USE_SYSCALL_TASK_STORAGE == 1
    u64 use_syscall_task_storage;
    LOAD_CONSTANT("use_syscall_task_storage", use_syscall_task_storage);
    if (use_syscall_task_storage) { // deadcode elimination will remove one of these branches
        bpf_task_storage_get(&capabilities_contexts, bpf_get_current_task_btf(), context, BPF_LOCAL_STORAGE_GET_F_CREATE);
        return;
    }
#endif
    bpf_map_update_elem(&capabilities_contexts, &tid, context, BPF_ANY);
}

static __attribute__((always_inline)) void delete_capabilities_context(u32 tid) {
#if USE_SYSCALL_TASK_STORAGE == 1
    u64 use_syscall_task_storage;
    LOAD_CONSTANT("use_syscall_task_storage", use_syscall_task_storage);
    if (use_syscall_task_storage) { // deadcode elimination will remove one of these branches
        bpf_task_storage_delete(&capabilities_contexts, bpf_get_current_task_btf());
        return;
    }
#endif
    bpf_map_delete_elem(&capabilities_contexts, &tid);
}

// tids are reused: a leftover override depth would silently suppress the next thread's tracking
static __attribute__((always_inline)) void cleanup_capabilities_context(u32 tid) {
    u64 capabilities_monitoring_enabled = 0;
    LOAD_CONSTANT("capabilities_monitoring_enabled", capabilities_monitoring_enabled);
    if (!capabilities_monitoring_enabled) {
        return;
    }

#if USE_SYSCALL_TASK_STORAGE == 1
    u64 use_syscall_task_storage;
    LOAD_CONSTANT("use_syscall_task_storage", use_syscall_task_storage);
    if (use_syscall_task_storage) { // deadcode elimination will remove one of these branches
        // the task storage is freed along with the task
        return;
    }
#endif
    bpf_map_delete_elem(&capabilities_contexts, &tid);
}

#endif /* _HELPERS_CAPS_H_ */
