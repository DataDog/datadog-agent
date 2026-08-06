#ifndef _HELPERS_THREAD_POINTER_H_
#define _HELPERS_THREAD_POINTER_H_

#include "constants/offsets/process.h"

// Reads the current thread's TLS thread pointer (x86 fsbase / ARM64 tpidr) from
// the kernel task_struct via BTF-resolved offsets. It is the base both
// thread-context readers start from: the Go runtime.g lookup (span_go.h) and
// the OTel thread local context record lookup (span_otel.h).
static u64 __attribute__((always_inline)) read_thread_pointer() {
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    u64 thread_offset = get_task_struct_thread_offset();
    u64 tp_field_offset = get_thread_struct_tp_offset();

    // 0 means the offset did not resolved (those fields are never at the start of the struct)
    if (thread_offset == 0 || tp_field_offset == 0) {
        return 0;
    }

    u64 tp = 0;
    int ret = bpf_probe_read_kernel(&tp, sizeof(tp),
                                     (void *)task + thread_offset + tp_field_offset);
    if (ret < 0) {
        return 0;
    }
    return tp;
}

#endif
