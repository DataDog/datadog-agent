#ifndef _EXEC_H_
#define _EXEC_H_

#include "filters.h"
#include "syscalls.h"

struct bpf_map_def SEC("maps/exec_pid_inode") exec_pid_inode = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(u64),
    .max_entries = 255,
    .pinning = 0,
    .namespace = "",
};

int __attribute__((always_inline)) trace__sys_execveat() {
    struct syscall_cache_t syscall = {
        .type = EVENT_EXEC,
    };

    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE(execve) {
    return trace__sys_execveat();
}

SYSCALL_KPROBE(execveat) {
    return trace__sys_execveat();
}

int __attribute__((always_inline)) vfs_handle_exec_event(struct pt_regs *ctx, struct syscall_cache_t *syscall) {
    u64 ino = get_path_ino((struct path *)PT_REGS_PARM1(ctx));
    u64 pid = syscall->pid;
    bpf_map_update_elem(&exec_pid_inode, &pid, &ino, BPF_ANY);
    
    pop_syscall();

    return 0;
}

u64 __attribute__((always_inline)) pid_inode(u64 pid) {
    u64 *inode = (u64 *) bpf_map_lookup_elem(&exec_pid_inode, &pid);
    if (inode)
        return *inode;
    return 0;
}

#endif