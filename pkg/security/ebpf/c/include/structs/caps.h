#ifndef _STRUCTS_CAPS_H_
#define _STRUCTS_CAPS_H_

struct capabilities_context_t {
    u64 cap_as_mask; // bitmask of capabilities that are being checked in the current task context
    u64 override_creds_depth; // depth of override_creds calls, used to track whether capability checks are performed against overridden credentials
};

struct capabilities_usage_t {
    u64 attempted; // bitmask of the capabilities that a process attempted to use
    u64 used; // bitmask of the capabilities that a process successfully used
};

struct capabilities_usage_key_t {
    u64 cookie; // cookie to proc_cache
    u64 tgid;
};

struct capabilities_usage_entry_t {
    struct capabilities_usage_t usage;
    u64 last_sent_ns; // timestamp of the last time this entry was sent to userspace
    u8 dirty; // indicates that the entry has been updated since the last time it was sent to userspace
};

#endif // _STRUCTS_CAPS_H_
