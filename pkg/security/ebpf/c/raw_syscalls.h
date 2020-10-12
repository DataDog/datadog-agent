#ifndef _RAW_SYSCALLS_H
#define _RAW_SYSCALLS_H

#include "defs.h"

struct _tracepoint_raw_syscalls_sys_enter
{
  unsigned short common_type;
  unsigned char common_flags;
  unsigned char common_preempt_count;
  int common_pid;
  long id;
  unsigned long args[6];
};

struct process_syscall_t {
    char comm[TASK_COMM_LEN];
    int pid;
    int id;
};

struct bpf_map_def SEC("maps/buffer_selector") buffer_selector = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/noisy_processes_fb") noisy_processes_fb = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct process_syscall_t),
    .value_size = sizeof(u64),
    .max_entries = 2048,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/noisy_processes_bb") noisy_processes_bb = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct process_syscall_t),
    .value_size = sizeof(u64),
    .max_entries = 2048,
    .pinning = 0,
    .namespace = "",
};

SEC("tracepoint/raw_syscalls/sys_enter")
int sys_enter(struct _tracepoint_raw_syscalls_sys_enter *args) {
    struct process_syscall_t syscall = {};
    bpf_probe_read(&syscall.pid, sizeof(syscall.pid), &args->common_pid);
    bpf_probe_read(&syscall.id, sizeof(syscall.id), &args->id);
    bpf_get_current_comm(&syscall.comm, sizeof(syscall.comm));

    u32 selector_key = 0;
    u32 *buffer_id;
    if (!(buffer_id = bpf_map_lookup_elem(&buffer_selector, &selector_key)))
        return 0;

    u64 *count;
    if (*buffer_id) {
        count = bpf_map_lookup_elem(&noisy_processes_bb, &syscall);
    } else {
        count = bpf_map_lookup_elem(&noisy_processes_fb, &syscall);
    }

    if (count) {
        (*count)++;
    } else {
        u64 one = 1;
        count = &one;

        if (*buffer_id) {
            bpf_map_update_elem(&noisy_processes_bb, &syscall, count, BPF_ANY);
        } else {
            bpf_map_update_elem(&noisy_processes_fb, &syscall, count, BPF_ANY);
        }
    }

    return 0;
}

#define MAX_PATH_LEN 256

struct exec_path {
    char filename[MAX_PATH_LEN];
};

struct bpf_map_def SEC("maps/exec_count_one") exec_count_one = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct exec_path),
    .value_size = sizeof(u64),
    .max_entries = 2048,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/exec_count_two") exec_count_two = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct exec_path),
    .value_size = sizeof(u64),
    .max_entries = 2048,
    .pinning = 0,
    .namespace = "",
};

struct _tracepoint_sched_sched_process_exec
{
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    int data_loc_filename;
    pid_t pid;
    pid_t old_pid;
};

SEC("tracepoint/sched/sched_process_exec")
int sched_process_exec(struct _tracepoint_sched_sched_process_exec *ctx) {
    // prepare filename pointer
    unsigned short __offset = ctx->data_loc_filename & 0xFFFF;
    char *filename = (char *)ctx + __offset;

    struct exec_path key = {};
    bpf_probe_read_str(&key.filename, MAX_PATH_LEN, filename);

    u32 selector_key = 0;
    u32 *buffer_id;
    if (!(buffer_id = bpf_map_lookup_elem(&buffer_selector, &selector_key)))
        return 0;

    u64 *count;
    if (*buffer_id) {
        count = bpf_map_lookup_elem(&exec_count_one, &key);
    } else {
        count = bpf_map_lookup_elem(&exec_count_two, &key);
    }

    if (count) {
        (*count)++;
    } else {
        u64 one = 1;
        count = &one;

        if (*buffer_id) {
            bpf_map_update_elem(&exec_count_one, &key, count, BPF_ANY);
        } else {
            bpf_map_update_elem(&exec_count_two, &key, count, BPF_ANY);
        }
    }
    return 0;
}

#endif
