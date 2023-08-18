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

#endif
