#ifndef _CONSTANTS_OFFSETS_PROCESS_H_
#define _CONSTANTS_OFFSETS_PROCESS_H_

#include "constants/macros.h"

u64 __attribute__((always_inline)) get_pid_level_offset() {
    u64 pid_level_offset;
    LOAD_CONSTANT("pid_level_offset", pid_level_offset);
    return pid_level_offset;
}

u64 __attribute__((always_inline)) get_pid_numbers_offset() {
    u64 pid_numbers_offset;
    LOAD_CONSTANT("pid_numbers_offset", pid_numbers_offset);
    return pid_numbers_offset;
}

u64 __attribute__((always_inline)) get_sizeof_upid() {
    u64 sizeof_upid;
    LOAD_CONSTANT("sizeof_upid", sizeof_upid);
    return sizeof_upid;
}

u64 __attribute__((always_inline)) get_task_struct_real_parent_offset() {
    u64 offset;
    LOAD_CONSTANT("task_struct_real_parent_offset", offset);
    return offset;
}

u64 __attribute__((always_inline)) get_task_struct_tgid_offset() {
    u64 offset;
    LOAD_CONSTANT("task_struct_tgid_offset", offset);
    return offset;
}

u64 __attribute__((always_inline)) get_task_struct_pid_offset() {
    u64 kernel_has_pid_link_struct;
    LOAD_CONSTANT("kernel_has_pid_link_struct", kernel_has_pid_link_struct);

    u64 task_struct_pid_offset;
    if (kernel_has_pid_link_struct) { // kernels < 4.19
        u64 task_struct_pid_link_offset;
        LOAD_CONSTANT("task_struct_pid_link_offset", task_struct_pid_link_offset);
        u64 pid_link_pid_offset;
        LOAD_CONSTANT("pid_link_pid_offset", pid_link_pid_offset);
        task_struct_pid_offset = task_struct_pid_link_offset + pid_link_pid_offset;
    } else {
        LOAD_CONSTANT("task_struct_pid_offset", task_struct_pid_offset);
    }

    return task_struct_pid_offset;
}

// OTel TLSDESC thread pointer access.
// Two offsets are summed to compute the address of the thread pointer within a task_struct:
//   x86_64: fsbase_addr   = (void *)task + thread_offset + fsbase_offset
//   ARM64:  tp_value_addr = (void *)task + thread_offset + uw_offset
// They are split because the BTF constant fetcher does not support dot-path
// traversal for named (non-anonymous) nested struct members.
u64 __attribute__((always_inline)) get_task_struct_thread_offset() {
    u64 offset;
    LOAD_CONSTANT("task_struct_thread_offset", offset);
    return offset;
}

#if defined(__x86_64__)
u64 __attribute__((always_inline)) get_thread_struct_fsbase_offset() {
    u64 offset;
    LOAD_CONSTANT("thread_struct_fsbase_offset", offset);
    return offset;
}
#elif defined(__aarch64__)
// thread_struct.uw.tp_value: tp_value is the first member of uw (offset 0),
// so the offset of uw within thread_struct gives us the tp_value address.
u64 __attribute__((always_inline)) get_thread_struct_uw_offset() {
    u64 offset;
    LOAD_CONSTANT("thread_struct_uw_offset", offset);
    return offset;
}
#endif

#endif
