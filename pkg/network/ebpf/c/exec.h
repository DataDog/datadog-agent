#ifndef _EXEC_H_
#define _EXEC_H_

#include "syscalls.h"
#include "container.h"
#include "process-types.h"

struct bpf_map_def SEC("maps/pid_ignored") pid_ignored = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 16738,
    .pinning = 0,
    .namespace = "",
};

struct _tracepoint_sched_process_fork {
    __u64 pad;

    char parent_comm[16];
    pid_t parent_pid;
    char child_comm[16];
    pid_t child_pid;
};

proc_cache_t __attribute__((always_inline)) *get_proc_from_cookie(u32 cookie) {
    if (!cookie) {
        return NULL;
    }

    return bpf_map_lookup_elem(&proc_cache, &cookie);
}

int __attribute__((always_inline)) trace__sys_execveat(struct pt_regs *ctx, const char **argv, const char **env) {
    struct syscall_cache_t syscall = { .type = EVENT_EXEC };
    cache_syscall(&syscall);

    return 0;
}

SEC("kprobe/sys_execve")
int BPF_KPROBE(kprobe__sys_execve, const char * filename, const char ** argv, const char ** env) {
    return trace__sys_execveat(ctx, argv, env);
}

SEC("kprobe/sys_execveat")
int BPF_KPROBE(kprobe__sys_execveat, int fd, const char * filename, const char ** argv, const char ** env) {
    return trace__sys_execveat(ctx, argv, env);
}

int __attribute__((always_inline)) handle_exec_event(struct pt_regs *ctx, struct syscall_cache_t *syscall) {
    log_debug("handle_exec_event\n");

    if (syscall->exec.is_parsed) {
        return 0;
    }
    syscall->exec.is_parsed = 1;

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    proc_cache_t entry = {
        .container = {},
        .exec_timestamp = bpf_ktime_get_ns(),
    };

    // select the previous cookie entry in cache of the current process
    // (this entry was created by the fork of the current process)
    pid_cache_t *fork_entry = (pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
    if (fork_entry) {
        // Fetch the parent proc cache entry
        u32 parent_cookie = fork_entry->cookie;
        proc_cache_t *parent_entry = get_proc_from_cookie(parent_cookie);
        if (parent_entry) {
            // inherit the parent container context
            fill_container_context(parent_entry, &entry.container);
        }
    }

    // Insert new proc cache entry (Note: do not move the order of this block with the previous one, we need to inherit
    // the container ID before saving the entry in proc_cache. Modifying entry after insertion won't work.)
    u32 cookie = bpf_get_prandom_u32();
    bpf_map_update_elem(&proc_cache, &cookie, &entry, BPF_ANY);

    // update pid <-> cookie mapping
    if (fork_entry) {
        fork_entry->cookie = cookie;
    } else {
        pid_cache_t new_pid_entry = {
            .cookie = cookie,
        };
        bpf_map_update_elem(&pid_cache, &tgid, &new_pid_entry, BPF_ANY);
    }

    return 0;
}

int __attribute__((always_inline)) handle_sys_fork(struct pt_regs *ctx) {
    struct syscall_cache_t syscall = {
        .type = EVENT_FORK,
    };

    cache_syscall(&syscall);

    return 0;
}

SEC("kprobe/sys_fork")
int kprobe__sys_fork(struct pt_regs *ctx) {
    log_debug("kprobe__sys_fork\n");
    return handle_sys_fork(ctx);
}

SEC("kprobe/sys_clone")
int kprobe__sys_clone(struct pt_regs *ctx) {
    log_debug("kprobe__sys_clone\n");
    return handle_sys_fork(ctx);
}

SEC("kprobe/sys_clone3")
int kprobe__sys_clone3(struct pt_regs *ctx) {
    log_debug("kprobe__sys_clone3\n");
    return handle_sys_fork(ctx);
}

SEC("kprobe/sys_vfork")
int kprobe__sys_vfork(struct pt_regs *ctx) {
    log_debug("kprobe__sys_vfork\n");
    return handle_sys_fork(ctx);
}

#define DO_FORK_STRUCT_INPUT 1

int __attribute__((always_inline)) handle_do_fork(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_FORK);
    if (!syscall) {
        return 0;
    }
    syscall->fork.is_thread = 1;

    u64 input;
    LOAD_CONSTANT("do_fork_input", input);

    if (input == DO_FORK_STRUCT_INPUT) {
        void *args = (void *)PT_REGS_PARM1(ctx);
        int exit_signal;
        bpf_probe_read(&exit_signal, sizeof(int), (void *)args + 32);

        if (exit_signal == SIGCHLD) {
            syscall->fork.is_thread = 0;
        }
    } else {
        u64 flags = (u64)PT_REGS_PARM1(ctx);
        if ((flags & SIGCHLD) == SIGCHLD) {
            syscall->fork.is_thread = 0;
        }
    }

    return 0;
}

SEC("kprobe/kernel_clone")
int kprobe__kernel_clone(struct pt_regs *ctx) {
    log_debug("kprobe__kernel_clone\n");
    return handle_do_fork(ctx);
}

SEC("kprobe/_do_fork")
int kprobe___do_fork(struct pt_regs *ctx) {
    log_debug("kprobe___do_fork\n");
    return handle_do_fork(ctx);
}

SEC("tracepoint/sched/sched_process_fork")
int sched_process_fork(struct _tracepoint_sched_process_fork *args) {
    log_debug("sched_process_fork\n");

    // inherit netns
    u32 pid = 0;
    bpf_probe_read(&pid, sizeof(pid), &args->child_pid);

    // ignore the rest if kworker
    struct syscall_cache_t *syscall = peek_syscall(EVENT_FORK);
    if (!syscall) {
        u32 value = 1;
        // mark as ignored fork not from syscall, ex: kworkers
        bpf_map_update_elem(&pid_ignored, &pid, &value, BPF_ANY);
        return 0;
    }

    // if this is a thread, leave
    if (syscall->fork.is_thread) {
        return 0;
    }

    u64 ts = bpf_ktime_get_ns();
    exec_event_t event = {
        .pid_entry.fork_timestamp = ts,
    };
    fill_process_context(&event.process);

    // the `parent_pid` entry of `sched_process_fork` might point to the TID (and not PID) of the parent. Since we
    // only work with PID, we can't use the TID. This is why we use the PID generated by the eBPF context instead.
    u32 ppid = event.process.pid;
    event.pid_entry.ppid = ppid;
    // sched::sched_process_fork is triggered from the parent process, update the pid / tid to the child value
    event.process.pid = pid;
    event.process.tid = pid;

    // ignore kthreads
    if (IS_KTHREAD(ppid, pid)) {
        return 0;
    }

    pid_cache_t *parent_pid_entry = (pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &ppid);
    if (parent_pid_entry) {
        // ensure pid and ppid point to the same cookie
        event.pid_entry.cookie = parent_pid_entry->cookie;

        // fetch the parent proc cache entry
        proc_cache_t *parent_proc_entry = get_proc_from_cookie(event.pid_entry.cookie);
        if (parent_proc_entry) {
            copy_proc_cache_except_comm(parent_proc_entry, &event.proc_entry);
        }
    }

    // insert the pid cache entry for the new process
    bpf_map_update_elem(&pid_cache, &pid, &event.pid_entry, BPF_ANY);

    // send the entry to maintain userspace cache
    send_event(args, EVENT_FORK, event);

    return 0;
}

SEC("kprobe/do_exit")
int kprobe__do_exit(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;
    u32 pid = pid_tgid;

    void *ignored = bpf_map_lookup_elem(&pid_ignored, &pid);
    if (ignored) {
        bpf_map_delete_elem(&pid_ignored, &pid);
        return 0;
    }

    if (tgid == pid) {
        // update exit time
        pid_cache_t *pid_entry = (pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
        if (pid_entry) {
            pid_entry->exit_timestamp = bpf_ktime_get_ns();
        }

        // send the entry to maintain userspace cache
        exit_event_t event = {};
        proc_cache_t *cache_entry = fill_process_context(&event.process);
        fill_container_context(cache_entry, &event.container);
        send_event(ctx, EVENT_EXIT, event);
    }

    return 0;
}

SEC("kprobe/do_dentry_open")
int kprobe__do_dentry_open(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_EXEC);
    if (!syscall) {
        return 0;
    }

    return handle_exec_event(ctx, syscall);
}


#endif
