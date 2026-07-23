#ifndef _STRUCTS_PROCESS_H_
#define _STRUCTS_PROCESS_H_

#include "constants/custom.h"
#include "events_context.h"
#include "dentry_resolver.h"

struct process_entry_t {
    struct file_t executable;

    u64 exec_timestamp;
    char tty_name[TTY_NAME_LEN];
    char comm[TASK_COMM_LEN];
};

struct proc_cache_t {
    struct cgroup_context_t cgroup;
    struct process_entry_t entry;
};

struct credentials_t {
    u32 uid;
    u32 gid;
    u32 euid;
    u32 egid;
    u32 fsuid;
    u32 fsgid;
    u32 auid;
    u32 is_auid_set;
    u64 cap_effective;
    u64 cap_permitted;
};

struct pid_cache_t {
    u64 cookie;
    u32 ppid;
    u32 padding;
    u64 fork_timestamp;
    u64 exit_timestamp;
    u64 user_session_id;
    u64 fork_flags;
    u32 sid;
    u32 padding_sid;
    struct credentials_t credentials;
};

struct args_envs_t {
    u64 id;
    u32 count; // argc/envc retrieved from the kernel
    u32 counter; // counter incremented while parsing args/envs
    u8 truncated;
};

struct args_envs_parsing_context_t {
    const char *args_start;
    u64 envs_offset;
    u64 parsing_offset;
    u32 args_count;
};

// linux_binprm_t contains content from the linux_binprm struct, which holds the arguments used for loading binaries
// We only need enough information from the executable field to be able to resolve the dentry.
struct linux_binprm_t {
    struct path_key_t interpreter;
};

struct str_array_buffer_t {
    char value[MAX_STR_BUFF_LEN];
};

union selinux_write_payload_t {
    // 1 for true, 0 for false, -1 (max) for error
    u32 bool_value;
    struct {
        u16 disable_value;
        u16 enforce_value;
    } status;
};

// --- Go pprof labels support ---
// dd-trace-go sets pprof labels on goroutines with keys "span id" and
// "local root span id" (decimal string values). The eBPF code traverses
// TLS → runtime.g → runtime.m → curg → labels to read them.

// Go runtime string header: {pointer, length}.
struct go_string_t {
    char *str;
    u64 len;
};

// Go runtime slice header: {array pointer, length, capacity}.
struct go_slice_t {
    void *array;
    u64 len;
    s64 cap;
};

// Go runtime map bucket (runtime.bmap) for map[string]string.
// Each bucket holds up to 8 key-value pairs.
#define GO_MAP_BUCKET_SIZE 8
struct go_map_bucket_t {
    char tophash[GO_MAP_BUCKET_SIZE];
    struct go_string_t keys[GO_MAP_BUCKET_SIZE];
    struct go_string_t values[GO_MAP_BUCKET_SIZE];
    void *overflow;
};

// Per-process offsets for reading Go pprof labels from eBPF.
// Populated by user-space after detecting a Go binary via tracer metadata.
struct go_labels_offsets_t {
    u32 m_offset;               // offset of 'm' field in runtime.g
    u32 curg;                   // offset of 'curg' field in runtime.m
    u32 labels;                 // offset of 'labels' field in runtime.g
    u32 hmap_count;             // offset of 'count' in runtime.hmap (0 for Go >=1.24)
    u32 hmap_log2_bucket_count; // offset of 'B' in runtime.hmap
    u32 hmap_buckets;           // offset of 'buckets' in runtime.hmap (0 = slice format)
    s32 tls_offset;             // TLS offset to G pointer (from thread pointer)
};

#endif
