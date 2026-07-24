#ifndef _HOOKS_CAPS_H_
#define _HOOKS_CAPS_H_

// Since kernel 6.13, override_creds/revert_creds are static inline and can no longer be hooked to
// track whether we are running under temporarily overridden credentials. On such kernels (which
// always ship BTF) we detect it directly: override_creds only swaps current->cred and leaves
// current->real_cred untouched, so cred != real_cred exactly while an override is in effect
// (commit_creds sets both, so they are otherwise equal). This requires the task_struct cred/real_cred
// offsets, which are only resolved through BTF; when they are unavailable the offsets are 0 and this
// returns false, in which case the override_creds/revert_creds depth counter below is relied upon.
static __attribute__((always_inline)) int is_in_creds_override() {
    u64 cred_offset = get_task_struct_cred_offset();
    u64 real_cred_offset = get_task_struct_real_cred_offset();
    if (cred_offset == 0 || real_cred_offset == 0) {
        return 0;
    }

    void *task = (void *)bpf_get_current_task();

    void *cred = NULL;
    bpf_probe_read(&cred, sizeof(cred), (char *)task + cred_offset);

    void *real_cred = NULL;
    bpf_probe_read(&real_cred, sizeof(real_cred), (char *)task + real_cred_offset);

    return cred != real_cred;
}

// On kernels < 6.13, override_creds/revert_creds are still out-of-line and hookable. They maintain a
// per-thread depth counter so that capability checks made under overridden credentials are skipped,
// which also covers kernels without BTF where is_in_creds_override() cannot resolve the cred offsets.
HOOK_ENTRY("override_creds")
int hook_override_creds(ctx_t *ctx) {
    u64 tgid_tid = bpf_get_current_pid_tgid();
    u32 tid = (u32)tgid_tid;

    struct capabilities_context_t *cap_context = bpf_map_lookup_elem(&capabilities_contexts, &tid);
    if (!cap_context) {
        struct capabilities_context_t new_context = {
            .cap_as_mask = 0, // No capabilities checked yet
            .override_creds_depth = 1, // We are entering an override_creds context
        };
        bpf_map_update_elem(&capabilities_contexts, &tid, &new_context, BPF_ANY);
    } else {
        cap_context->override_creds_depth++;
    }

    return 0;
}

HOOK_ENTRY("revert_creds")
int hook_revert_creds(ctx_t *ctx) {
    u64 tgid_tid = bpf_get_current_pid_tgid();
    u32 tid = (u32)tgid_tid;

    struct capabilities_context_t *cap_context = bpf_map_lookup_elem(&capabilities_contexts, &tid);
    if (!cap_context) {
        // unexpected but we handle it gracefully
        return 0;
    }

    if (cap_context->override_creds_depth > 0) {
        cap_context->override_creds_depth--;
        if (cap_context->override_creds_depth == 0) {
            // If we reached zero, we can remove the context
            bpf_map_delete_elem(&capabilities_contexts, &tid);
        }
    }

    return 0;
}

HOOK_ENTRY("security_capable")
int hook_security_capable(ctx_t *ctx) {
    u64 tgid_tid = bpf_get_current_pid_tgid();
    u32 tid = (u32)tgid_tid;
    struct capabilities_context_t *cap_context = bpf_map_lookup_elem(&capabilities_contexts, &tid);

    if (is_in_creds_override() || (cap_context && cap_context->override_creds_depth != 0)) {
        // do not track capabilities checked under temporarily overridden credentials
        return 0;
    }

    u64 cap = CTX_PARM3(ctx); // The capability being checked

    // capabilities are a per-thread attribute, but as our process model is process-based we use
    // the tgid to aggregate capabilities usage per process.
    u32 tgid = tgid_tid >> 32;
    struct pid_cache_t *pid_entry = get_pid_cache(tgid);
    if (!pid_entry) {
        return 0;
    }

    // Check if the process has a cookie set, if not, we cannot track capabilities
    if (!pid_entry->cookie) {
        return 0;
    }

    // Create a key for the capabilities usage map
    struct capabilities_usage_key_t key = {
        .cookie = pid_entry->cookie,
        .tgid = tgid,
    };

    // we can use a bitmask here because CAP_LAST_CAP is less than 64
    u64 cap_as_mask = 1ULL << cap;

    // Look up the capabilities usage entry for this process
    struct capabilities_usage_entry_t *entry = bpf_map_lookup_elem(&capabilities_usage, &key);
    if (!entry) {
        struct capabilities_usage_entry_t new_entry = {0};
        new_entry.usage.attempted = cap_as_mask;
        new_entry.usage.used = 0;
        update_dirty(&new_entry, 1); // Mark as dirty since we are creating a new entry
        bpf_map_update_elem(&capabilities_usage, &key, &new_entry, BPF_ANY);
    } else {
        update_dirty(entry, (entry->usage.attempted & cap_as_mask) == 0); // Mark as dirty if this capability was not previously attempted
        entry->usage.attempted |= cap_as_mask; // Mark the capability as checked
    }

    if (cap_context) {
        cap_context->cap_as_mask = cap_as_mask;
    } else {
        // If no context exists, we create a new one
        struct capabilities_context_t new_context = {
            .cap_as_mask = cap_as_mask,
        };
        bpf_map_update_elem(&capabilities_contexts, &tid, &new_context, BPF_ANY);
    }

    return 0;
}

HOOK_EXIT("security_capable")
int rethook_security_capable(ctx_t *ctx) {
    u64 tgid_tid = bpf_get_current_pid_tgid();
    u32 tid = (u32)tgid_tid;
    struct capabilities_context_t *cap_context = bpf_map_lookup_elem(&capabilities_contexts, &tid);
    if (!cap_context || !cap_context->cap_as_mask) {
        // unexpected, we should have a context at this point since we created one in hook_security_capable
        return 0;
    }

    if (is_in_creds_override() || cap_context->override_creds_depth != 0) {
        // do not track capabilities checked under temporarily overridden credentials
        return 0;
    }

    u64 cap_as_mask = cap_context->cap_as_mask; // The capability being checked as a bitmask
    bpf_map_delete_elem(&capabilities_contexts, &tid); // Free the context because we are done with it at this point

    int retval = CTX_PARMRET(ctx); // The return value of the capability check, (0 for success, !0 for failure)
    if (retval != 0) { // If the capability check was not successful, we do not need to update the used capabilities set
        return 0;
    }

    u32 tgid = tgid_tid >> 32;
    struct pid_cache_t *pid_entry = get_pid_cache(tgid);
    if (!pid_entry) {
        return 0;
    }

    // Check if the process has a cookie set, if not, we cannot track capabilities
    if (!pid_entry->cookie) {
        return 0;
    }

    // Create a key for the capabilities usage map
    struct capabilities_usage_key_t key = {
        .cookie = pid_entry->cookie,
        .tgid = tgid,
    };

    // Look up the capabilities usage entry for this process
    struct capabilities_usage_entry_t *entry = bpf_map_lookup_elem(&capabilities_usage, &key);
    if (!entry) {
        // unexpected, we should have an entry at this point since we created one in hook_security_capable
        return 0;
    }

    update_dirty(entry, (entry->usage.used & cap_as_mask) == 0); // Mark as dirty if this capability was not previously used
    entry->usage.used |= cap_as_mask;

    return 0;
}

struct callback_context_t {
    struct bpf_perf_event_data *ctx;
};

static long for_each_capabilities_usage_cb(struct bpf_map *map, const void *k, void *value, void *callback_ctx) {
    struct capabilities_usage_key_t *key = (struct capabilities_usage_key_t *)k;
    struct capabilities_usage_entry_t *entry = (struct capabilities_usage_entry_t *)value;
    struct bpf_perf_event_data *ctx = ((struct callback_context_t *)callback_ctx)->ctx;

    send_capabilities_usage_event(ctx, key, entry);

    return 0;
}

SEC("perf_event/cpu_clock")
int capabilities_usage_ticker(struct bpf_perf_event_data *ctx) {
    // we want a single core to trigger capabilities usage events
    if (bpf_get_smp_processor_id() > 0) {
        return 0;
    }

    struct callback_context_t callback_ctx = {
        .ctx = ctx,
    };

    bpf_for_each_map_elem(&capabilities_usage, &for_each_capabilities_usage_cb, &callback_ctx, 0);

    return 0;
};

#endif /* _HOOKS_CAPS_H_ */
