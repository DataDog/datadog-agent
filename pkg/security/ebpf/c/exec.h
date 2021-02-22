#ifndef _EXEC_H_
#define _EXEC_H_

#include <linux/tty.h>

#include "filters.h"
#include "syscalls.h"
#include "container.h"

#define MAX_ARGS_PERF_LEN 128
#define MAX_ARGS_LEN (1 << 15)
#define MAX_ARGS 32
#define MAX_ARG_SIZE 4096

struct first_args_value_t {
    u32 id;
    u32 truncated;
    char args[MAX_ARGS_PERF_LEN];
};

struct args_value_t {
    char args[MAX_ARGS_LEN];
};

struct bpf_map_def SEC("maps/args_cache") args_cache = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct args_value_t),
    .max_entries = 255,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/args_value") args_value = {
    .type = BPF_MAP_TYPE_PERCPU_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct args_value_t),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

struct exec_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct proc_cache_t proc_entry;
    struct pid_cache_t pid_entry;
    struct first_args_value_t args;
};

struct exit_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
};

struct _tracepoint_sched_process_fork
{
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    char parent_comm[16];
    pid_t parent_pid;
    char child_comm[16];
    pid_t child_pid;
};

void __attribute__((always_inline)) extract_args(struct syscall_cache_t *syscall, const char **argv) {
    syscall->exec.args_id = bpf_get_prandom_u32();

    u32 key = 0;
    struct args_value_t *args = bpf_map_lookup_elem(&args_value, &key);
    if (!args) {
        return;
    }

    u32 offset = 0;
    u32 a = 1;
    u32 len = 0;

    const char *str;
    bpf_probe_read(&str, sizeof(str), (void *)&argv[a]);

#pragma unroll
    for (int i = 0; i < MAX_ARGS; i++) {
        int n = bpf_probe_read_str(&(args->args[(offset + sizeof(len)) & (MAX_ARGS_LEN - MAX_ARG_SIZE - 1)]), MAX_ARG_SIZE, (void *)str);
        if (n > 0) {
            n--; // ignore trailing space

            len = n;

            bpf_probe_read(&(args->args[offset&(MAX_ARGS_LEN - MAX_ARG_SIZE - 1)]), sizeof(len), &len);

            bpf_probe_read(&str, sizeof(str), (void *)&argv[++a]);

            offset += n + sizeof(len);
        } else {
            bpf_map_update_elem(&args_cache, &syscall->exec.args_id, args, BPF_ANY);
            return;
        }
    }

    syscall->exec.args_truncated = 1;
    bpf_map_update_elem(&args_cache, &syscall->exec.args_id, args, BPF_ANY);
}

int __attribute__((always_inline)) trace__sys_execveat(const char **argv, const char **env) {
    struct syscall_cache_t syscall = {
        .type = SYSCALL_EXEC,
    };
    extract_args(&syscall, argv);

    cache_syscall(&syscall);
    return 0;
}

SYSCALL_KPROBE3(execve, const char *, filename, const char **, argv, const char **, env) {
    return trace__sys_execveat(argv, env);
}

SYSCALL_KPROBE4(execveat, int, fd, const char *, filename, const char **, argv, const char **, env) {
    return trace__sys_execveat(argv, env);
}

int __attribute__((always_inline)) handle_exec_event(struct pt_regs *ctx, struct syscall_cache_t *syscall) {
    if (syscall->exec.is_parsed) {
        return 0;
    }
    syscall->exec.is_parsed = 1;

    struct file *file = (struct file *)PT_REGS_PARM1(ctx);
    struct inode *inode = (struct inode *)PT_REGS_PARM2(ctx);
    struct path *path = &file->f_path;

    syscall->open.dentry = get_file_dentry(file);
    syscall->open.path_key = get_inode_key_path(inode, &file->f_path);
    syscall->open.path_key.path_id = get_path_id(0);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct dentry *exec_dentry = get_path_dentry(path);
    struct proc_cache_t entry = {
        .executable = {
            .inode = syscall->open.path_key.ino,
            .overlay_numlower = get_overlay_numlower(exec_dentry),
            .mount_id = get_path_mount_id(path),
            .path_id = syscall->open.path_key.path_id,
        },
        .container = {},
        .exec_timestamp = bpf_ktime_get_ns(),
    };
    fill_file_metadata(exec_dentry, &entry.executable.metadata);
    bpf_get_current_comm(&entry.comm, sizeof(entry.comm));

    // cache dentry
    resolve_dentry(syscall->open.dentry, syscall->open.path_key, 0);

    // select the previous cookie entry in cache of the current process
    // (this entry was created by the fork of the current process)
    struct pid_cache_t *fork_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
    if (fork_entry) {
        // Fetch the parent proc cache entry
        u32 parent_cookie = fork_entry->cookie;
        struct proc_cache_t *parent_entry = bpf_map_lookup_elem(&proc_cache, &parent_cookie);
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
        struct pid_cache_t new_pid_entry = {
            .cookie = cookie,
        };
        bpf_map_update_elem(&pid_cache, &tgid, &new_pid_entry, BPF_ANY);
    }

    return 0;
}

#define DO_FORK_STRUCT_INPUT 1

int __attribute__((always_inline)) handle_do_fork(struct pt_regs *ctx) {
    u64 input;
    LOAD_CONSTANT("do_fork_input", input);

    if (input == DO_FORK_STRUCT_INPUT) {
        void *args = (void *)PT_REGS_PARM1(ctx);
        int exit_signal;
        bpf_probe_read(&exit_signal, sizeof(int), (void *)args + 32);

        // Only insert an entry if this is a thread
        if (exit_signal == SIGCHLD) {
            return 0;
        }
    } else {
        u64 flags = (u64)PT_REGS_PARM1(ctx);

        if ((flags & SIGCHLD) == SIGCHLD) {
            return 0;
        }
    }

    struct syscall_cache_t syscall = {
        .type = SYSCALL_FORK,
        .clone = {
            .is_thread = 1,
        }
    };
    cache_syscall(&syscall);

    return 0;
}

SEC("kprobe/kernel_clone")
int kprobe_kernel_clone(struct pt_regs *ctx) {
    return handle_do_fork(ctx);
}

SEC("kprobe/do_fork")
int krpobe_do_fork(struct pt_regs *ctx) {
    return handle_do_fork(ctx);
}

SEC("kprobe/_do_fork")
int kprobe__do_fork(struct pt_regs *ctx) {
    return handle_do_fork(ctx);
}

SEC("tracepoint/sched/sched_process_fork")
int sched_process_fork(struct _tracepoint_sched_process_fork *args) {
    // check if this is a thread first
    struct syscall_cache_t *syscall = pop_syscall(SYSCALL_FORK);
    if (syscall) {
        return 0;
    }

    u32 pid = 0;
    u32 ppid = 0;
    bpf_probe_read(&pid, sizeof(pid), &args->child_pid);
    u64 ts = bpf_ktime_get_ns();

    struct exec_event_t event = {
        .pid_entry.fork_timestamp = ts,
    };
    bpf_get_current_comm(&event.proc_entry.comm, sizeof(event.proc_entry.comm));
    fill_process_context(&event.process);

    // the `parent_pid` entry of `sched_process_fork` might point to the TID (and not PID) of the parent. Since we
    // only work with PID, we can't use the TID. This is why we use the PID generated by the eBPF context instead.
    ppid = event.process.pid;
    event.pid_entry.ppid = ppid;
    // sched::sched_process_fork is triggered from the parent process, update the pid / tid to the child value
    event.process.pid = pid;
    event.process.tid = pid;

    struct pid_cache_t *parent_pid_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &ppid);
    if (parent_pid_entry) {
        // ensure pid and ppid point to the same cookie
        event.pid_entry.cookie = parent_pid_entry->cookie;

        // ensure pid and ppid have the same credentials
        event.pid_entry.credentials = parent_pid_entry->credentials;

        // fetch the parent proc cache entry
        struct proc_cache_t *parent_proc_entry = bpf_map_lookup_elem(&proc_cache, &event.pid_entry.cookie);
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
int kprobe_do_exit(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;
    u32 pid = pid_tgid;

    if (tgid == pid) {
        if (!is_flushing_discarders()) {
            bpf_map_delete_elem(&pid_discarders, &tgid);
        }

        // update exit time
        struct pid_cache_t *pid_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
        if (pid_entry) {
            pid_entry->exit_timestamp = bpf_ktime_get_ns();
        }

        // send the entry to maintain userspace cache
        struct exit_event_t event = {};
        struct proc_cache_t *cache_entry = fill_process_context(&event.process);
        fill_container_context(cache_entry, &event.container);

        send_event(ctx, EVENT_EXIT, event);
    }

    return 0;
}

SEC("kprobe/exit_itimers")
int kprobe_exit_itimers(struct pt_regs *ctx) {
    struct signal_struct *signal = (struct signal_struct *)PT_REGS_PARM1(ctx);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct proc_cache_t *entry = get_proc_cache(tgid);
    if (entry) {
        struct tty_struct *tty;
        bpf_probe_read(&tty, sizeof(tty), &signal->tty);
        bpf_probe_read_str(entry->tty_name, TTY_NAME_LEN, tty->name);
    }

    return 0;
}

void __attribute__((always_inline)) fill_args(struct exec_event_t *event, struct syscall_cache_t *syscall) {
    struct args_value_t *args = bpf_map_lookup_elem(&args_cache, &syscall->exec.args_id);
    if (args) {
        bpf_probe_read(&event->args.args, MAX_ARGS_PERF_LEN, args->args);
        event->args.id = syscall->exec.args_id;
        event->args.truncated = syscall->exec.args_truncated;
    }
}

SEC("kprobe/security_bprm_committed_creds")
int kprobe_security_bprm_committed_creds(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(SYSCALL_EXEC);
    if (!syscall) {
        return 0;
    }

    // check if this is a thread first
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct pid_cache_t *pid_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
    if (pid_entry) {
        u32 cookie = pid_entry->cookie;
        struct proc_cache_t *proc_entry = bpf_map_lookup_elem(&proc_cache, &cookie);
        if (proc_entry) {
            struct exec_event_t event = {};
            // copy proc_cache entry data
            copy_proc_cache_except_comm(proc_entry, &event.proc_entry);
            bpf_get_current_comm(&event.proc_entry.comm, sizeof(event.proc_entry.comm));

            // copy pid_cache entry data
            copy_pid_cache_except_exit_ts(pid_entry, &event.pid_entry);

            // add pid / tid context
            fill_process_context(&event.process);
            fill_args(&event, syscall);

            // send the entry to maintain userspace cache
            send_event(ctx, EVENT_EXEC, event);
        }
    }

    return 0;
}
#endif
