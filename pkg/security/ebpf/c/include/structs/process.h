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
    u16 attrs_data_size; // size of custom attributes data following this header
};

// Maximum size of OTel custom attributes data stored in the otel_span_attrs map.
// The RFC allows up to 65535 bytes (u16), but typical records are <=64 bytes.
// 256 bytes is generous while keeping BPF map value size reasonable.
#define OTEL_ATTRS_MAX_SIZE 256

// Key for the otel_span_attrs map: uniquely identifies a span.
struct otel_span_attrs_key_t {
    u64 span_id;
    u64 trace_id[2];
};

// Value for the otel_span_attrs map: raw attrs_data bytes from the OTel record.
// Format per RFC: repeated [key(u8) + length(u8) + val(u8[length])].
struct otel_span_attrs_t {
    u16 size;                        // actual size of attrs_data
    u8  data[OTEL_ATTRS_MAX_SIZE];   // raw attribute bytes
};

// Offset of otel_thread_ctx_record_t.valid from the record base.
#define OTEL_THREAD_CTX_VALID_OFFSET 24

// OTel TLS lookup modes for otel_tls_t.mode.
// User-space identifies the mapped ELF object exporting the STT_TLS
// `otel_thread_ctx_v1` symbol and stores file-derived metadata here. eBPF then
// resolves the current process's runtime TLS module ID by walking the dynamic
// loader's live module list, matching the glibc tls-modid-bpf sample. Fully
// static no-loader executables use the main-module DTV convention.
#define OTEL_TLS_MODE_STATIC_MAIN 1
#define OTEL_TLS_MODE_LINK_MAP    2

// OTel TLS lookup status values used internally by the eBPF resolver.
#define OTEL_TLS_LOOKUP_OK 0
#define OTEL_TLS_LOOKUP_NO_PT_TLS 2
#define OTEL_TLS_LOOKUP_OFFSET_OUT_OF_RANGE 3
#define OTEL_TLS_LOOKUP_NO_R_DEBUG 5
#define OTEL_TLS_LOOKUP_R_DEBUG_READ_ERROR 6
#define OTEL_TLS_LOOKUP_LINK_MAP_READ_ERROR 7
#define OTEL_TLS_LOOKUP_LINK_MAP_NOT_FOUND 8
#define OTEL_TLS_LOOKUP_BAD_MODE 9
#define OTEL_TLS_LOOKUP_NO_THREAD_POINTER 10
#define OTEL_TLS_LOOKUP_DTV_READ_ERROR 11
#define OTEL_TLS_LOOKUP_TLS_BLOCK_UNAVAILABLE 12

#define OTEL_TLS_MAX_LINK_MAPS 256
#define OTEL_TLS_MAX_MODULE_ID 4096
#define OTEL_TLS_HASH_SLOTS 65536
#define OTEL_TLS_HASH_WORDS (OTEL_TLS_HASH_SLOTS / 64)

// OTel TLS registration for a process. The first block is written by
// user-space after reading only ELF files and procfs metadata. The final
// resolver fields are cached by eBPF after reading the target process's live
// loader state.
struct otel_tls_t {
    u64 dt_debug_value_addr;          // live address of main executable DT_DEBUG.d_un
    u64 target_load_bias;             // loader link_map l_addr to match
    u64 target_symbol_offset;         // STT_TLS st_value, offset in module TLS block
    u64 target_symbol_size;           // STT_TLS st_size
    u64 target_tls_memsz;             // defining module PT_TLS p_memsz
    u64 r_debug_r_map_offset;         // offset from struct r_debug to r_map
    u64 link_map_l_addr_offset;       // offset to loader module load bias/base
    u64 link_map_l_next_offset;       // offset to next loader module
    u64 link_map_l_real_offset;       // offset to canonical module node, or 0
    u64 link_map_l_tls_modid_offset;  // glibc _thread_db_link_map_l_tls_modid
    u64 link_map_l_tls_offset_offset; // glibc _thread_db_link_map_l_tls_offset
    s64 tcb_dtv_offset;               // signed offset from thread pointer to DTV pointer
    u64 dtv_entry_size;               // size of one DTV entry
    u64 dtv_entry_pointer_offset;     // offset within one DTV entry to TLS block pointer
    u64 tls_module_hash_seed;         // seed for tls_module_hash_bits
    u64 tls_module_hash_bits[OTEL_TLS_HASH_WORDS]; // file-derived PT_TLS module set

    u64 resolved_mod_id;              // runtime TLS module ID found by eBPF
    s64 resolved_static_tls_offset;   // glibc static TLS offset, when available
    s32 resolved_read_error;          // bpf_probe_read_user error for status
    u32 mode;                         // OTEL_TLS_MODE_* constant
    u32 reconstruct_module_ids;       // fallback: count known PT_TLS modules in loader order
    u32 tls_module_count;             // number of PT_TLS modules encoded in hash bits
    u32 runtime;                      // enum otel_runtime_language
    u32 resolved;                     // non-zero once eBPF wrote resolver fields
    u32 status;                       // OTEL_TLS_LOOKUP_* value
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
