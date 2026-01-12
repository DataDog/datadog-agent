#ifndef _HELPERS_CAPS_H_
#define _HELPERS_CAPS_H_

static __attribute__((always_inline)) void send_capabilities_usage_event(void *ctx, struct capabilities_usage_key_t *key, struct capabilities_usage_entry_t *entry) {
    u64 now = bpf_ktime_get_ns();
    int should_send = is_dirty(entry) && period_reached_or_new_entry(entry, now);
    if (!should_send) {
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
    set_last_sent_ns(entry, now);

    struct capabilities_event_t event = {
        .caps_usage = entry->usage,
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

#endif /* _HELPERS_CAPS_H_ */
