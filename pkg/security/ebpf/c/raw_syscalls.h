#ifndef _RAW_SYSCALLS_H
#define _RAW_SYSCALLS_H

#include "defs.h"

struct _tracepoint_raw_syscalls_sys_enter {
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

struct bpf_map_def SEC("maps/sys_exit_progs") sys_exit_progs = {
    .type = BPF_MAP_TYPE_PROG_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 64,
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

// used as a fallback, because tracepoints are not enable when using a ia32 userspace application with a x64 kernel
// cf. https://elixir.bootlin.com/linux/latest/source/arch/x86/include/asm/ftrace.h#L106
int __attribute__((always_inline)) handle_sys_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_ANY);
    if (!syscall)
        return 0;

    bpf_tail_call(args, &sys_exit_progs, syscall->type);
    return 0;
}

SEC("tracepoint/raw_syscalls/sys_exit")
int sys_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    u64 fallback;
    LOAD_CONSTANT("tracepoint_raw_syscall_fallback", fallback);
    if (fallback) {
        handle_sys_exit(args);
    }

    // won't be call in case of fallback use
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

#endif
