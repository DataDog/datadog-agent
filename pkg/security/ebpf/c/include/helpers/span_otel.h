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

    return 1;
}

#endif
