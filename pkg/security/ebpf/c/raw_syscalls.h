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

struct bpf_map_def SEC("maps/concurrent_syscalls") concurrent_syscalls = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(long),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

#define CONCURRENT_SYSCALLS_COUNTER 0

struct process_syscall_t {
    char comm[TASK_COMM_LEN];
    int pid;
    int id;
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

    struct bpf_map_def *noisy_processes = select_buffer(&noisy_processes_fb, &noisy_processes_bb, SYSCALL_MONITOR_KEY);
    if (noisy_processes == NULL)
        return 0;

    u64 zero = 0;
    u64 *count = bpf_map_lookup_or_try_init(noisy_processes, &syscall, &zero);
    if (count == NULL) {
        return 0;
    }

    __sync_fetch_and_add(count, 1);

    u32 key = CONCURRENT_SYSCALLS_COUNTER;
    long *concurrent_syscalls_counter = bpf_map_lookup_elem(&concurrent_syscalls, &key);
    if (concurrent_syscalls_counter == NULL)
        return 0;

    __sync_fetch_and_add(concurrent_syscalls_counter, 1);

    return 0;
}

SEC("tracepoint/raw_syscalls/sys_exit")
int sys_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    u64 enabled;
    LOAD_CONSTANT("syscall_monitor", enabled);
    if (enabled) {
        u32 key = CONCURRENT_SYSCALLS_COUNTER;
        long *concurrent_syscalls_counter = bpf_map_lookup_elem(&concurrent_syscalls, &key);
        if (concurrent_syscalls_counter == NULL)
            return 0;

        __sync_fetch_and_add(concurrent_syscalls_counter, -1);
        if (*concurrent_syscalls_counter < 0) {
            __sync_fetch_and_add(concurrent_syscalls_counter, 1);
        }
    }

    return 0;
}

#define MAX_PATH_LEN 256

struct exec_path {
    char filename[MAX_PATH_LEN];
};

struct bpf_map_def SEC("maps/exec_count_fb") exec_count_fb = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct exec_path),
    .value_size = sizeof(u64),
    .max_entries = 2048,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/exec_count_bb") exec_count_bb = {
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

    struct bpf_map_def *exec_count = select_buffer(&exec_count_fb, &exec_count_bb, SYSCALL_MONITOR_KEY);
    if (exec_count == NULL)
        return 0;

    u64 zero = 0;
    u64 *count = bpf_map_lookup_or_try_init(exec_count, &key, &zero);
    if (count == NULL) {
        return 0;
    }

    __sync_fetch_and_add(count, 1);

    return 0;
}

#endif
