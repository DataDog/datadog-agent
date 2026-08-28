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

// --- OTel Thread Local Context Record support (OTEP 4947) ---

// Fixed header of a record, as the instrumented process publishes it.
struct otel_thread_ctx_record_t {
    u8 trace_id[16];     // W3C Trace Context byte order (big-endian)
    u8 span_id[8];       // W3C Trace Context byte order (big-endian)
    u8 valid;            // must be 1 for the record to be considered valid
    u8 _reserved;        // padding for alignment
    u16 attrs_data_size; // size of custom attributes data following this header
};

// Cap on the attrs_data bytes staged per record: the spec allows up to 65535,
// but typical records are <= 64 bytes.
#define OTEL_ATTRS_MAX_SIZE 256

// Entry of the otel_span_attrs ring: a record's raw attrs_data bytes, encoded as
// repeated [key(u8), length(u8), val(u8[length])] for user space to parse.
//
// id is stamped last, once the data is complete, and user space drops an entry
// whose id no longer matches the one the event carried, meaning the ring slot has
// been reused. Same scheme as go_labels_ctx_entry_t.
//
// padding is explicit because cilium/ebpf only reads a map value straight into
// its Go mirror when the mirror's binary.Size matches its unsafe.Sizeof, and
// otherwise falls back to binary.Decode, which rejects the value as
// under-consumed.
struct otel_span_attrs_t {
    u32 id;                          // id of the staged snapshot, 0 while incomplete
    u16 size;                        // actual size of attrs_data
    u16 padding;                     // keeps the entry free of implicit padding
    u8  data[OTEL_ATTRS_MAX_SIZE];   // raw attribute bytes
};

// Offset of otel_thread_ctx_record_t.valid (16-byte trace id + 8-byte span id).
#define OTEL_THREAD_CTX_VALID_OFFSET 24

// otel_dtv_info_t describes how to walk the Dynamic Thread Vector (DTV) for a
// process's libc. DTV access is always indirect: the thread pointer plus
// otel_dtv_info_t.offset yields a pointer to the DTV array, which must itself
// be dereferenced (indexed by module_id) before reading the module's TLS
// block. Mirrors DTVInfo in DataDog's opentelemetry-ebpf-profiler fork
// (support/ebpf/types.h, PR #1229).
struct otel_dtv_info_t {
    s64 offset;     // offset of the DTV pointer from the thread pointer
    u32 multiplier; // size of one DTV entry in bytes (16 glibc / 8 musl)
    u32 _pad;
};

// V8 layout constants the Node.js writer publishes in its OTEP 4719 process
// context, so the reader needs to know nothing of how V8 was built. Zero unless
// the runtime is OTEL_RUNTIME_NODEJS.
struct otel_v8_layout_t {
    u16 tagged_size;                  // width of a tagged word; only 8 is supported
    u16 js_map_table_offset;          // JSMap -> backing OrderedHashMap
    u16 ordered_hash_map_header_size; // header before the OrderedHashMap fields
    u16 wrapped_object_offset;        // JSObject -> the native wrapper of a record
    u16 native_wrap_fields_offset;    // native wrapper -> the record pointer
    u16 _pad[3];
};

// OTel TLS registration for a process, written once by user space after
// classifying the access model of its otel_thread_ctx_v1 (or
// otel_thread_ctx_nodejs_v1) thread-local.
struct otel_tls_t {
    u32 runtime;                     // enum otel_runtime_language
    u32 module_id;                   // TLS module ID for dynamic TLS, or 0 for static TLS
    s64 tls_offset;                  // TP-relative (static TLS, module_id==0) or
                                     // in-module offset (dynamic TLS, module_id!=0)
    struct otel_dtv_info_t dtv_info; // unused when module_id == 0
    struct otel_v8_layout_t v8;      // unused unless runtime == OTEL_RUNTIME_NODEJS
};

// Discovery struct the Node.js writer publishes in its otel_thread_ctx_nodejs_v1
// thread-local: where to start walking for a record. See span_nodejs.h.
struct otel_nodejs_ctx_t {
    u64 cped_slot;          // isolate slot holding the live AsyncContextFrame
    u64 als_handle;         // handle to the AsyncLocalStorage the frame is keyed by
    u32 als_identity_hash;  // its identity hash, which picks a single bucket
    u32 _pad;
    u64 undefined_addr;     // this isolate's undefined, i.e. "no context here"
};

#endif
