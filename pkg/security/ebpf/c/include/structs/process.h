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
    u64 fork_timestamp;
    u64 exit_timestamp;
    u64 user_session_id;
    u64 fork_flags;
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

struct span_tls_t {
    u64 format;
    u64 max_threads;
    void *base;
};

// OTel Thread Local Context Record (per OTel spec PR #4947).
// This is the fixed 28-byte header that OTel SDKs publish via ELF TLSDESC.
// Targets native applications (C, C++, Rust, Java/JNI, .NET/FFI, etc.) on x86_64 and ARM64.
// Go runtime support uses pprof labels instead (see span_go.h).
struct otel_thread_ctx_record_t {
    u8 trace_id[16];     // W3C Trace Context byte order (big-endian)
    u8 span_id[8];       // W3C Trace Context byte order (big-endian)
    u8 valid;            // must be 1 for the record to be considered valid
    u8 _reserved;        // padding for alignment
    u16 attrs_data_size; // size of custom attributes data (not read)
};

// OTel TLSDESC-based TLS registration for a process.
// The tls_offset is discovered by user-space by parsing the ELF dynsym table for
// the `otel_thread_ctx_v1` TLS symbol, then pushed to the otel_tls BPF map.
// x86_64: reads fsbase from task_struct->thread.fsbase
// ARM64:  reads tp_value from task_struct->thread.uw.tp_value
struct otel_tls_t {
    s64 tls_offset; // signed offset from thread pointer to the TLS variable
    u32 runtime;    // enum otel_runtime_language
    u32 _pad;
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
