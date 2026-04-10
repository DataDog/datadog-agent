#ifndef _HELPERS_SPAN_OTEL_H_
#define _HELPERS_SPAN_OTEL_H_

#include "maps.h"
#include "process.h"

// --- OTel Thread Local Context Record (per OTel spec PR #4947) ---
// Targets native applications using ELF TLSDESC (C, C++, Rust, Java/JNI, etc.).
// Supported architectures: x86_64 (fsbase), ARM64 (tpidr_el0 / uw.tp_value).
// The otel_tls BPF map is populated by user-space after parsing the ELF dynsym table
// for the `otel_thread_ctx_v1` TLS symbol. No eRPC registration is used.
// Go runtime support uses pprof labels instead (see span_go.h).

int __attribute__((always_inline)) unregister_otel_tls() {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    bpf_map_delete_elem(&otel_tls, &tgid);

    return 0;
}

// Convert 8 bytes in W3C (big-endian / network byte order) to a native-endian u64.
static u64 __attribute__((always_inline)) otel_bytes_to_u64(const u8 *bytes) {
    return ((u64)bytes[0] << 56) | ((u64)bytes[1] << 48) |
           ((u64)bytes[2] << 40) | ((u64)bytes[3] << 32) |
           ((u64)bytes[4] << 24) | ((u64)bytes[5] << 16) |
           ((u64)bytes[6] << 8)  | ((u64)bytes[7]);
}

// Read the thread pointer (TLS base) from the current task_struct.
// x86_64: task_struct->thread.fsbase
// ARM64:  task_struct->thread.uw.tp_value (tp_value at offset 0 within uw)
static u64 __attribute__((always_inline)) read_thread_pointer() {
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    u64 thread_offset = get_task_struct_thread_offset();

#if defined(__x86_64__)
    u64 tp_field_offset = get_thread_struct_fsbase_offset();
#elif defined(__aarch64__)
    u64 tp_field_offset = get_thread_struct_uw_offset();
#else
    return 0;
#endif

    u64 tp = 0;
    int ret = bpf_probe_read_kernel(&tp, sizeof(tp),
                                     (void *)task + thread_offset + tp_field_offset);
    if (ret < 0) {
        return 0;
    }
    return tp;
}

// Per-CPU scratch buffer for OTel span attributes.
// Avoids placing the 258-byte otel_span_attrs_t on the stack.
BPF_PERCPU_ARRAY_MAP(otel_span_attrs_gen, struct otel_span_attrs_t, 1)

// Try to fill span context from an OTel Thread Local Context Record.
// Returns 1 on success, 0 otherwise.
// Only attempts TLS resolution for native runtimes (not Go).
static int __attribute__((always_inline)) fill_span_context_otel(struct span_context_t *span) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct otel_tls_t *otls = bpf_map_lookup_elem(&otel_tls, &tgid);
    if (!otls) {
        return 0;
    }

    // Only resolve TLS-based context for native runtimes.
    // Go uses pprof labels instead (see span_go.h / fill_span_context_go).
    if (otls->runtime != OTEL_RUNTIME_NATIVE) {
        return 0;
    }

    // Read the thread pointer from the kernel task_struct.
    u64 tp = read_thread_pointer();
    if (tp == 0) {
        return 0;
    }

    // The TLSDESC TLS variable is a pointer to the active Thread Local Context Record.
    // Read the pointer at [thread_pointer + tls_offset].
    void *record_ptr = NULL;
    int ret = bpf_probe_read_user(&record_ptr, sizeof(record_ptr),
                               (void *)(tp + otls->tls_offset));
    if (ret < 0 || record_ptr == NULL) {
        return 0;
    }

    // Read the OTel Thread Local Context Record (28-byte fixed header).
    struct otel_thread_ctx_record_t record = {};
    ret = bpf_probe_read_user(&record, sizeof(record), record_ptr);
    if (ret < 0) {
        return 0;
    }

    // The record is only valid when the valid field is exactly 1.
    if (record.valid != 1) {
        return 0;
    }

    // Convert W3C byte order (big-endian) to native-endian span_context_t.
    // OTel trace-id: bytes[0..7] = high 64 bits, bytes[8..15] = low 64 bits.
    span->trace_id[1] = otel_bytes_to_u64(&record.trace_id[0]);  // Hi
    span->trace_id[0] = otel_bytes_to_u64(&record.trace_id[8]);  // Lo
    span->span_id = otel_bytes_to_u64(record.span_id);

    // If the record has custom attributes, read them and store in the otel_span_attrs map.
    if (record.attrs_data_size > 0) {
        u32 zero = 0;
        struct otel_span_attrs_t *attrs_val = bpf_map_lookup_elem(&otel_span_attrs_gen, &zero);
        if (attrs_val) {
            u16 attrs_size = record.attrs_data_size;
            if (attrs_size > OTEL_ATTRS_MAX_SIZE) {
                attrs_size = OTEL_ATTRS_MAX_SIZE;
            }

            __builtin_memset(attrs_val, 0, sizeof(*attrs_val));
            attrs_val->size = attrs_size;

            // Read attrs_data from right after the 28-byte fixed header.
            ret = bpf_probe_read_user(attrs_val->data, attrs_size & 0xff,
                                      record_ptr + sizeof(struct otel_thread_ctx_record_t));
            if (ret >= 0) {
                struct otel_span_attrs_key_t attrs_key = {
                    .span_id = span->span_id,
                    .trace_id = { span->trace_id[0], span->trace_id[1] },
                };
                bpf_map_update_elem(&otel_span_attrs, &attrs_key, attrs_val, BPF_ANY);
                span->has_extra_attrs = 1;
            }
        }
    }

    return 1;
}

#endif
