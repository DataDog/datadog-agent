#ifndef _HOOKS_OFFSET_GUESSER_H_
#define _HOOKS_OFFSET_GUESSER_H_

#include "constants/macros.h"

SEC("kprobe/get_pid_task")
int kprobe_get_pid_task_numbers(struct pt_regs *ctx) {
    struct pid *pid = (struct pid *) PT_REGS_PARM1(ctx);
    if (!pid) {
        return 0;
    }

    u64 pid_expected;
    LOAD_CONSTANT("pid_expected", pid_expected);

    u32 offset = 0, success = 0;

#pragma unroll
    for (int i = MIN_PID_OFFSET; i != MAX_PID_OFFSET; i++) {
        u32 root_nr = 0;

        int read_ok = bpf_probe_read(&root_nr, sizeof(root_nr), (void *)pid + offset);
        if (!read_ok && root_nr == pid_expected) {
            // found it twice, thus error
            if (success) {
                return 0;
            }
            success = offset;
        }

        offset++;
    }

    if (success) {
        u32 key = PID_OFFSET_INDEX;
        bpf_map_update_elem(&guessed_offsets, &key, &success, BPF_ANY);
    }

    return 0;
}

SEC("kprobe/get_pid_task")
int kprobe_get_pid_task_offset(struct pt_regs *ctx) {
    u64 expected_pid_ptr = (u64)PT_REGS_PARM1(ctx);
    if (!expected_pid_ptr) {
        return 0;
    }

    u64 task_ptr = bpf_get_current_task();

    u32 success = 0;
    u64 read_ptr = 0;

#pragma unroll
    for (int offset = MIN_PID_STRUCT_OFFSET; offset < MAX_PID_STRUCT_OFFSET; offset += sizeof(struct pid *)) {
        int read_ok = bpf_probe_read(&read_ptr, sizeof(read_ptr), (void *)task_ptr + offset);
        if (!read_ok && read_ptr == expected_pid_ptr) {
            // found it twice, thus error
            if (success) {
                return 0;
            }
            success = offset;
        }
    }

    if (success) {
        u32 key = PID_STRUCT_OFFSET_INDEX;
        bpf_map_update_elem(&guessed_offsets, &key, &success, BPF_ANY);
    }

    return 0;
}

#endif
