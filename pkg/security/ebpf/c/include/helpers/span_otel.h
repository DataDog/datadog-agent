#ifndef _HELPERS_SPAN_OTEL_H_
#define _HELPERS_SPAN_OTEL_H_

#include "maps.h"
#include "process.h"

#include "thread_pointer.h"

// --- OTel Thread Local Context Record (OTEP 4947) ---
// Reads the trace context that native runtimes (C, C++, Rust, Java/JNI, ...)
// publish per thread through the otel_thread_ctx_v1 ELF TLS variable, whose
// access model user space has already resolved into struct otel_tls_t.

int __attribute__((always_inline)) unregister_otel_tls() {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    bpf_map_delete_elem(&otel_tls, &tgid);

    return 0;
}

// A forked child keeps the parent's address space, so its TLS copy still holds
// the record.
static int __attribute__((always_inline)) inherit_otel_tls(u32 ppid, u32 pid) {
    struct otel_tls_t *parent = bpf_map_lookup_elem(&otel_tls, &ppid);
    if (!parent) {
        return 0;
    }

    // copy to stack for older kernel verifiers
    struct otel_tls_t on_stack_otls = *parent;
    bpf_map_update_elem(&otel_tls, &pid, &on_stack_otls, BPF_ANY);

    return 0;
}

// Convert 8 bytes in W3C order (big-endian) to a native-endian u64.
static u64 __attribute__((always_inline)) otel_bytes_to_u64(const u8 *bytes) {
    return ((u64)bytes[0] << 56) | ((u64)bytes[1] << 48) |
           ((u64)bytes[2] << 40) | ((u64)bytes[3] << 32) |
           ((u64)bytes[4] << 24) | ((u64)bytes[5] << 16) |
           ((u64)bytes[6] << 8)  | ((u64)bytes[7]);
}

// Reads the otel_thread_ctx_v1 TLS variable of the current thread, which holds a
// pointer to the active Thread Local Context Record: directly at
// tsd_base + tls_offset for static TLS, through the DTV (see otel_dtv_info_t)
// for dynamic TLS. Mirrors tls_read in DataDog's opentelemetry-ebpf-profiler
// fork (support/ebpf/tsd.h, PR #1229).
static int __attribute__((always_inline)) otel_tls_read(
        struct otel_tls_t *otls, u64 tsd_base, void **out) {
    u64 tls_block = tsd_base;

    if (otls->module_id != 0) {
        u64 dtv_ptr = 0;
        if (bpf_probe_read_user(&dtv_ptr, sizeof(dtv_ptr),
                                (void *)(tsd_base + otls->dtv_info.offset))) {
            return -1;
        }

        // DTV layout: [generation, module1_block, module2_block, ...], so module
        // N's block sits N entries in.
        u64 dtv_entry_offset = (u64)otls->module_id * otls->dtv_info.multiplier;
        if (bpf_probe_read_user(&tls_block, sizeof(tls_block),
                                (void *)(dtv_ptr + dtv_entry_offset))) {
            return -1;
        }
    }

    return bpf_probe_read_user(out, sizeof(*out), (void *)(tls_block + otls->tls_offset));
}

// Mint an id for one staged otel_span_attrs entry, resolved from user space.
// Mirrors mint_go_labels_id(), including why the __sync_fetch_and_add() result is
// discarded; two cores can therefore mint the same id, at worst losing one of
// the two snapshots.
static u32 __attribute__((always_inline)) mint_otel_span_attrs_id(void) {
    u32 key = 0;
    u32 *id = bpf_map_lookup_elem(&otel_attrs_gen_id, &key);
    if (!id) {
        return 0;
    }
    __sync_fetch_and_add(id, 1);
    u32 minted = *id;
    if (minted == 0) {
        // The u32 counter wrapped and 0 is the "no id" sentinel, skip it.
        __sync_fetch_and_add(id, 1);
        minted = *id;
    }
    return minted;
}

static struct otel_span_attrs_t * __attribute__((always_inline)) lookup_otel_span_attrs_entry(u32 id) {
    u32 key = id % OTEL_SPAN_ATTRS_MAX_ENTRIES;
    return bpf_map_lookup_elem(&otel_span_attrs, &key);
}

// Fills span from the current thread's OTel context record. Returns 1 on
// success, 0 when there is nothing to read.
int __attribute__((always_inline)) fill_span_context_otel(struct span_context_t *span) {
    if (!span) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct otel_tls_t *otls = bpf_map_lookup_elem(&otel_tls, &tgid);
    if (!otls) {
        return 0;
    }

    // Go runtimes publish their context through pprof labels instead.
    if (otls->runtime != OTEL_RUNTIME_NATIVE) {
        return 0;
    }

    u64 tsd_base = read_thread_pointer();
    if (tsd_base == 0) {
        return 0;
    }

    void *record_ptr = NULL;
    if (otel_tls_read(otls, tsd_base, &record_ptr) || record_ptr == NULL) {
        return 0;
    }

    // valid is checked on both sides of the copy below: the instrumented thread
    // clears it while it updates the record, so a torn read is rejected.
    u8 valid_before = 0;
    int ret = bpf_probe_read_user(&valid_before, sizeof(valid_before),
                                  record_ptr + OTEL_THREAD_CTX_VALID_OFFSET);
    if (ret < 0 || valid_before != 1) {
        return 0;
    }

    struct otel_thread_ctx_record_t record = {};
    ret = bpf_probe_read_user(&record, sizeof(record), record_ptr);
    if (ret < 0) {
        return 0;
    }

    u8 valid_after = 0;
    ret = bpf_probe_read_user(&valid_after, sizeof(valid_after),
                              record_ptr + OTEL_THREAD_CTX_VALID_OFFSET);
    if (ret < 0 || record.valid != 1 || valid_after != 1) {
        return 0;
    }

    // The W3C trace id is big-endian: bytes[0..7] are its high 64 bits.
    span->trace_id[1] = otel_bytes_to_u64(&record.trace_id[0]);  // Hi
    span->trace_id[0] = otel_bytes_to_u64(&record.trace_id[8]);  // Lo
    span->span_id = otel_bytes_to_u64(record.span_id);

    if (record.attrs_data_size > 0) {
        // Clamp, then re-derive through a mask, as in store_go_label(): clang
        // otherwise folds the clamp away and the verifier only sees the u16 range,
        // and the +1 keeps umin >= 1, which 4.14 requires.
        u64 attrs_size = record.attrs_data_size;
        if (attrs_size > OTEL_ATTRS_MAX_SIZE) {
            attrs_size = OTEL_ATTRS_MAX_SIZE;
        }
        barrier_var(attrs_size);
        attrs_size = ((attrs_size - 1) & (OTEL_ATTRS_MAX_SIZE - 1)) + 1;

        // The attributes are read straight into the ring slot: handing the entry to
        // bpf_map_update_elem() would not load before 4.18, where
        // ARG_PTR_TO_MAP_VALUE had to be PTR_TO_STACK, and the entry is far too
        // large to bounce through the 512-byte stack.
        u32 attrs_id = mint_otel_span_attrs_id();
        struct otel_span_attrs_t *entry = lookup_otel_span_attrs_entry(attrs_id);
        if (attrs_id != 0 && entry != NULL) {
            // id is cleared here and stamped below, so a reader never matches an
            // entry whose data is still the previous snapshot's.
            entry->id = 0;
            entry->size = attrs_size;

            // Bytes past attrs_size keep the previous snapshot's data; user space
            // only ever reads data[:size].
            ret = bpf_probe_read_user(entry->data, attrs_size,
                                      record_ptr + sizeof(struct otel_thread_ctx_record_t));
            if (ret >= 0) {
                entry->id = attrs_id;
                span->extra_attrs_id = attrs_id;
            }
        }
    }

    return 1;
}

#endif
