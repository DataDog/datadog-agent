#ifndef _STRUCTS_CAPS_H_
#define _STRUCTS_CAPS_H_

struct capabilities_context_t {
    u64 cap_as_mask; // bitmask of capabilities that are being checked in the current task context
    u64 override_creds_depth; // depth of override_creds calls, used to track if the capability checks are performed against user capabilities
};

struct capabilities_usage_t {
    u64 attempted; // bitmask of the capabilities that a process attempted to use
    u64 used; // bitmask of the capabilities that a process successfully used
};

struct capabilities_usage_key_t {
    u64 cookie; // cookie to proc_cache
    u64 tgid;
};

#define CAPABILITIES_USAGE_ENTRY_DIRTY_MASK 1ULL
#define CAPABILITIES_USAGE_ENTRY_LAST_SENT_MASK (~CAPABILITIES_USAGE_ENTRY_DIRTY_MASK)

struct capabilities_usage_entry_t {
    struct capabilities_usage_t usage;
    u64 data; // data is representing both the `dirty` flag and the `last_sent_ns` timestamp
};

__attribute__((always_inline)) bool is_dirty(struct capabilities_usage_entry_t *entry) {
    return (entry->data & CAPABILITIES_USAGE_ENTRY_DIRTY_MASK) != 0;
}

__attribute__((always_inline)) void update_dirty(struct capabilities_usage_entry_t *entry, bool dirty) {
    entry->data |= (dirty ? CAPABILITIES_USAGE_ENTRY_DIRTY_MASK : 0);
}

__attribute__((always_inline)) bool period_reached_or_new_entry(struct capabilities_usage_entry_t *entry, u64 now) {
    now = now & CAPABILITIES_USAGE_ENTRY_LAST_SENT_MASK; // Clear the dirty flag
    u64 last_sent_ns = entry->data & CAPABILITIES_USAGE_ENTRY_LAST_SENT_MASK;
    return last_sent_ns == 0 || ((now - last_sent_ns) >= get_capabilities_monitoring_period());
}

__attribute__((always_inline)) void reset_dirty(struct capabilities_usage_entry_t *entry) {
    entry->data &= ~CAPABILITIES_USAGE_ENTRY_DIRTY_MASK;
}

__attribute__((always_inline)) void set_last_sent_ns(struct capabilities_usage_entry_t *entry, u64 ts) {
    entry->data = (entry->data & CAPABILITIES_USAGE_ENTRY_DIRTY_MASK) | (ts & CAPABILITIES_USAGE_ENTRY_LAST_SENT_MASK);
}

#endif // _STRUCTS_CAPS_H_
