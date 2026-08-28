#ifndef _STRUCTS_SPAN_CONTEXT_H_
#define _STRUCTS_SPAN_CONTEXT_H_

#include "constants/custom.h"

// --- Go pprof labels support ---
// dd-trace-go sets pprof labels on goroutines. The eBPF code traverses
// TLS → runtime.g → runtime.m → curg → labels to store them in a ring buffer.
// Labels are later parsed in user-space to alleviate eBPF work.

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

// One raw Go pprof label (key/value) as copied from user memory. key_len/val_len
// are the real Go string lengths.
struct go_label_pair_t {
    u16 key_len;
    u16 val_len;
    char key[GO_LABELS_CTX_KEY_SIZE];
    char val[GO_LABELS_CTX_VAL_SIZE];
};

// A snapshot of a goroutine's pprof labels, stored in the go_labels_ctx ring at
// id % GO_LABELS_CTX_MAX_ENTRIES, exactly like the syscall_ctx design.
struct go_labels_ctx_entry_t {
    u32 id;
    struct go_label_pair_t pairs[GO_LABELS_CTX_MAX_PAIRS];
};

// Per-CPU scratch buffer for reading Go label headers off the stack.
struct go_labels_scratch_t {
    // Only ever holds the {key, value} string header pair currently being
    // copied out of the labels slice.
    struct go_string_t pair[2];
    // The map bucket (Go <1.24) and the slice header (Go >=1.24) formats are
    // mutually exclusive: a given binary only ever uses one of them.
    union {
        struct go_map_bucket_t bucket;
        struct go_slice_t slice;
    };
};

#endif
