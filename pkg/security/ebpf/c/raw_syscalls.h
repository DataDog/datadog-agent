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

struct bpf_map_def SEC("maps/noisy_processes_buffer") noisy_processes_buffer = {
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

    u32 zero = 0;
    u32 *back_buffer;
    if (!(back_buffer = bpf_map_lookup_elem(&noisy_processes_buffer, (void*) &zero)))
        return 0;

    u64 count = 0;
    u64 *current_count;
    
    if (*back_buffer) {
        current_count = bpf_map_lookup_elem(&noisy_processes_bb, &syscall);
    } else {
        current_count = bpf_map_lookup_elem(&noisy_processes_fb, &syscall);
    }

    if (current_count) {
        count = *current_count;
    }
    count++;

    if (*back_buffer) {
        bpf_map_update_elem(&noisy_processes_bb, &syscall, &count, BPF_ANY);
    } else {
        bpf_map_update_elem(&noisy_processes_fb, &syscall, &count, BPF_ANY);        
    }

    return 0;
}

#endif