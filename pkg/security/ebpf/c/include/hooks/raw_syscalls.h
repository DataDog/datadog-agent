#ifndef _HOOKS_RAW_SYSCALLS_H
#define _HOOKS_RAW_SYSCALLS_H

#include "helpers/syscalls.h"
#include "helpers/buffer_selector.h"
#include "default_syscalls.h"

SEC("tracepoint/raw_syscalls/sys_enter")
int sys_enter(struct _tracepoint_raw_syscalls_sys_enter *args) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;

    // handle kill actions when enforcement is performed through the raw syscall tracepoint
    if (is_enforcement_raw_syscall_enabled()) {
        send_signal(pid);
    }

    u64 enabled;
    LOAD_CONSTANT("syscall_monitor_enabled", enabled);

    if (!enabled) {
        return 0;
    }

    // The children of kthreadd will be ignored userspace side
    if (IS_KTHREADD(pid)) {
        return 0;
    }

    // skip the noisy baseline syscalls; userspace will reinject them via
    // defaultSyscallSerializers when building the event
    if (is_default_syscall((unsigned long)args->id)) {
        return 0;
    }

    u32 idx = ((unsigned long)args->id) / 64;
    u64 bit = 1ULL << (((unsigned long)args->id) % 64);

    u64 cgroup_id = get_current_cgroup_id();
    if (!cgroup_id) {
        return 0;
    }

    struct syscall_monitor_key_t key = {
        .cgroup_id = cgroup_id, // assume that inode recycling is limited
        .idx = idx,
    };

    struct bpf_map_def *syscall_monitor = select_buffer(&fb_syscall_monitor, &bb_syscall_monitor, SYSCALL_MONITOR_KEY);
    if (syscall_monitor == NULL) {
        return 0;
    }

    u64 *value = bpf_map_lookup_elem(syscall_monitor, &key);
    if (!value) {
        bpf_map_update_elem(syscall_monitor, &key, &bit, BPF_ANY);
    } else {
        *value |= bit;
    }

    return 0;
}

// used as a fallback, because tracepoints are not enable when using a ia32 userspace application with a x64 kernel
// cf. https://elixir.bootlin.com/linux/latest/source/arch/x86/include/asm/ftrace.h#L106
int __attribute__((always_inline)) handle_sys_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_ANY);
    if (!syscall) {
        return 0;
    }

    bpf_tail_call_compat(args, &sys_exit_progs, syscall->type);
    return 0;
}

SEC("tracepoint/raw_syscalls/sys_exit")
int sys_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    handle_sys_exit(args);
    return 0;
}

#endif
