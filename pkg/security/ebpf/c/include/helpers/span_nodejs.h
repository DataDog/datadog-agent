#ifndef _HELPERS_SPAN_NODEJS_H_
#define _HELPERS_SPAN_NODEJS_H_

#include "span_otel.h"

// --- Node.js thread context (OTEP 4947, nodejs_v1 schema) ---
//
// The Node.js writer hangs each record off the asynchronous context rather than
// off the thread, and publishes in a thread-local of its own where to start
// walking for one:
//
//   cped_slot -> AsyncContextFrame (a JS Map)
//             -> its OrderedHashMap
//             -> the one bucket the identity hash designates
//             -> the entry whose key is the ALS instance
//             -> its value, the JS object wrapping the record
//             -> the record
//
// Every offset the walk needs comes from user space (see otel_v8_layout_t), and
// the identity hash bounds the only search to one bucket, whose chain is walked
// a fixed number of times.

// Tag of a V8 pointer to a heap object; an untagged word is a Smi. Both hold
// only for a 64-bit V8 built without pointer compression, which is what user
// space checks for by refusing a process whose tagged size is not 8.
#define OTEL_V8_HEAP_OBJECT_TAG 1

// Fields of an OrderedHashMap, in tagged words after its header: the element
// counts, then one entry index per bucket, then the entries themselves, three
// words each.
#define OTEL_V8_OHM_NUM_BUCKETS_INDEX 2
#define OTEL_V8_OHM_BUCKETS_INDEX 3
#define OTEL_V8_OHM_ENTRY_WORDS 3
// V8 keeps an OrderedHashMap at half load, so its entry table holds twice its
// bucket count.
#define OTEL_V8_OHM_LOAD_FACTOR 2
// An AsyncContextFrame holds one entry per AsyncLocalStorage in use, so more
// buckets than this means the walk landed on something else.
#define OTEL_V8_OHM_MAX_BUCKETS 1024

// How far the chain of one bucket is followed. Overshooting drops the context,
// not the event.
#define OTEL_NODEJS_MAX_CHAIN 4

static u64 __attribute__((always_inline)) otel_v8_untag(u64 value) {
    return value & ~(u64)OTEL_V8_HEAP_OBJECT_TAG;
}

static int __attribute__((always_inline)) otel_v8_is_heap_object(u64 value) {
    return (value & OTEL_V8_HEAP_OBJECT_TAG) == OTEL_V8_HEAP_OBJECT_TAG;
}

// Decodes a Smi, or returns -1 for a word that is not one. -1 doubles as the
// end-of-chain and empty-bucket marker V8 itself stores, so a word that should
// have been a Smi and is not ends the walk instead of steering it.
static s64 __attribute__((always_inline)) otel_v8_smi(u64 value) {
    if ((value & 0xffffffff) != 0) {
        return -1;
    }
    return (s64)value >> 32;
}

static int __attribute__((always_inline)) otel_v8_read_word(u64 addr, u64 *out) {
    return bpf_probe_read_user(out, sizeof(*out), (void *)addr);
}

// Walks the chain of one bucket looking for the entry keyed by the ALS instance,
// and yields its value. Returns 1 once value holds it, 0 otherwise.
static int __attribute__((always_inline)) otel_nodejs_lookup_als(
        struct otel_v8_layout_t *v8, u64 entries, u64 capacity, s64 entry_index,
        u64 als, u64 *value) {
    u64 entry[OTEL_V8_OHM_ENTRY_WORDS];

#pragma unroll
    for (int i = 0; i < OTEL_NODEJS_MAX_CHAIN; i++) {
        // Bounding against the entry table is what keeps a bad read from
        // steering the next one.
        if (entry_index < 0 || (u64)entry_index >= capacity) {
            return 0;
        }

        // key, value and next index are adjacent: one read for the whole entry.
        u64 addr = entries + (u64)entry_index * OTEL_V8_OHM_ENTRY_WORDS * v8->tagged_size;
        if (bpf_probe_read_user(entry, sizeof(entry), (void *)addr)) {
            return 0;
        }

        if (entry[0] == als) {
            *value = entry[1];
            return 1;
        }
        entry_index = otel_v8_smi(entry[2]);
    }

    return 0;
}

// Walks from the discovery struct to the record the active asynchronous context
// holds. Returns 1 once record_ptr holds it, 0 when the context holds none or a
// step of the walk did not read.
static int __attribute__((always_inline)) otel_nodejs_record_ptr(
        struct otel_v8_layout_t *v8, struct otel_nodejs_ctx_t *nctx, void **record_ptr) {
    u64 frame = 0;
    if (otel_v8_read_word(nctx->cped_slot, &frame)) {
        return 0;
    }
    // No asynchronous context yet.
    if (frame == nctx->undefined_addr || !otel_v8_is_heap_object(frame)) {
        return 0;
    }

    u64 table = 0;
    if (otel_v8_read_word(otel_v8_untag(frame) + v8->js_map_table_offset, &table) ||
        !otel_v8_is_heap_object(table)) {
        return 0;
    }
    u64 fields = otel_v8_untag(table) + v8->ordered_hash_map_header_size;

    u64 buckets_word = 0;
    if (otel_v8_read_word(fields + OTEL_V8_OHM_NUM_BUCKETS_INDEX * v8->tagged_size, &buckets_word)) {
        return 0;
    }
    // V8 keeps the bucket count a power of two, which the mask below needs.
    s64 num_buckets = otel_v8_smi(buckets_word);
    if (num_buckets <= 0 || num_buckets > OTEL_V8_OHM_MAX_BUCKETS || (num_buckets & (num_buckets - 1)) != 0) {
        return 0;
    }

    // The published identity hash of the key selects the bucket, which is what
    // spares the walk the whole table.
    u64 bucket = (u64)nctx->als_identity_hash & (u64)(num_buckets - 1);
    u64 head_word = 0;
    if (otel_v8_read_word(fields + (OTEL_V8_OHM_BUCKETS_INDEX + bucket) * v8->tagged_size, &head_word)) {
        return 0;
    }

    // The handle is a slot the garbage collector keeps pointing at the ALS
    // instance, so the instance address has to be read now.
    u64 als = 0;
    if (otel_v8_read_word(nctx->als_handle, &als)) {
        return 0;
    }

    u64 entries = fields + (OTEL_V8_OHM_BUCKETS_INDEX + (u64)num_buckets) * v8->tagged_size;
    u64 capacity = (u64)num_buckets * OTEL_V8_OHM_LOAD_FACTOR;

    // No entry is an asynchronous context carrying no thread context, the
    // common case outside of a span.
    u64 value = 0;
    if (!otel_nodejs_lookup_als(v8, entries, capacity, otel_v8_smi(head_word), als, &value)) {
        return 0;
    }
    if (value == nctx->undefined_addr || !otel_v8_is_heap_object(value)) {
        return 0;
    }

    // The value is the JS object wrapping the record.
    u64 wrapper = 0;
    if (otel_v8_read_word(otel_v8_untag(value) + v8->wrapped_object_offset, &wrapper)) {
        return 0;
    }
    // Cleared when the isolate is torn down.
    if (wrapper == 0) {
        return 0;
    }
    if (wrapper < OTEL_MIN_USER_ADDR || (wrapper & (sizeof(u64) - 1)) != 0) {
        return 0;
    }

    // Unlike the wrapper, the record is a byte buffer the writer allocates:
    // nothing says it is aligned.
    u64 record = 0;
    if (otel_v8_read_word(wrapper + v8->native_wrap_fields_offset, &record)) {
        return 0;
    }
    if (record < OTEL_MIN_USER_ADDR) {
        return 0;
    }

    *record_ptr = (void *)record;
    return 1;
}

// Fills span from the record the current thread's asynchronous context holds.
// Returns 1 on success, 0 when there is nothing to read.
int __attribute__((always_inline)) fill_span_context_nodejs(
        struct span_context_t *span, struct otel_tls_t *otls) {
    // User space refuses to register a process whose V8 is built with pointer
    // compression; every step of the walk assumes otherwise.
    if (otls->v8.tagged_size != sizeof(u64)) {
        return 0;
    }

    u64 tsd_base = read_thread_pointer();
    if (tsd_base == 0) {
        return 0;
    }

    u64 var_addr = otel_tls_var_addr(otls, tsd_base);
    if (var_addr == 0) {
        return 0;
    }

    struct otel_nodejs_ctx_t nctx = {};
    if (bpf_probe_read_user(&nctx, sizeof(nctx), (void *)var_addr)) {
        return 0;
    }
    // Left zeroed until the writer runs on this thread, which it never does on
    // a thread that runs no JS -- a libuv thread pool worker, say.
    if (nctx.cped_slot == 0) {
        return 0;
    }

    void *record_ptr = NULL;
    if (!otel_nodejs_record_ptr(&otls->v8, &nctx, &record_ptr)) {
        return 0;
    }

    // Unlike the native writer, this one clears valid for good when the span
    // ends: the asynchronous contexts that inherited the record keep offering
    // it, and valid is what tells them from the live one.
    return otel_fill_from_record(span, record_ptr);
}

#endif
