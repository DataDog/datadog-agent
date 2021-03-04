#ifndef _EXEC_H_
#define _EXEC_H_

#include <linux/tty.h>

#include "filters.h"
#include "syscalls.h"
#include "container.h"

#define MAX_PERF_STR_BUFF_LEN 128
#define MAX_STR_BUFF_LEN (1 << 15)
#define MAX_ARRAY_ELEMENT 40
#define MAX_ARRAY_ELEMENT_SIZE 4096

struct args_envs_event_t {
    struct kevent_t event;
    u32 id;
    u32 size;
    char value[MAX_PERF_STR_BUFF_LEN];
};

struct str_array_buffer_t {
    char value[MAX_STR_BUFF_LEN];
};

struct bpf_map_def SEC("maps/str_array_buffers") str_array_buffers = {
    .type = BPF_MAP_TYPE_PERCPU_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct str_array_buffer_t),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

struct exec_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct proc_cache_t proc_entry;
    struct pid_cache_t pid_entry;
    u32 args_id;
    u32 args_truncated;
    u32 envs_id;
    u32 envs_truncated;
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

void __attribute__((always_inline)) parse_str_array(struct pt_regs *ctx, struct str_array_ref_t *array_ref, const char **data) {
    u32 id = bpf_get_prandom_u32();
    array_ref->id = id;

    u32 key = 0;
    struct str_array_buffer_t *buff = bpf_map_lookup_elem(&str_array_buffers, &key);
    if (!buff) {
        return;
    }

    int a = 1;

    const char *str;
    bpf_probe_read(&str, sizeof(str), (void *)&data[a]);

    struct args_envs_event_t event = {
        .id = id,
    };

    int i = 0, n = 0, offset = 0;
    int perf_offset = 0;

    #pragma unroll
    for (i = 0; i < MAX_ARRAY_ELEMENT; i++) {
        void *ptr = &(buff->value[(offset + sizeof(n)) & (MAX_STR_BUFF_LEN - MAX_ARRAY_ELEMENT_SIZE - 1)]);

        n = bpf_probe_read_str(ptr, MAX_ARRAY_ELEMENT_SIZE, (void *)str);
        if (n > 0) {
            n--; // remove trailing 0
            bpf_probe_read(&(buff->value[offset&(MAX_STR_BUFF_LEN - MAX_ARRAY_ELEMENT_SIZE - 1)]), sizeof(n), &n);
            bpf_probe_read(&str, sizeof(str), (void *)&data[++a]);

            int len = n + sizeof(n);
            offset += len;

            if (event.size + len > MAX_PERF_STR_BUFF_LEN) {
                void *perf_ptr = &(buff->value[perf_offset&(MAX_STR_BUFF_LEN - MAX_ARRAY_ELEMENT_SIZE - 1)]);
                bpf_probe_read(&event.value, MAX_PERF_STR_BUFF_LEN, perf_ptr);

                // current argument overflow the perf buff size, thus send it truncated
                if (len > MAX_PERF_STR_BUFF_LEN) {
                    event.size = MAX_PERF_STR_BUFF_LEN;

                    perf_offset = offset;
                    len = 0;
                } else {
                    perf_offset += event.size;
                }

                send_event(ctx, EVENT_ARGS_ENVS, event);
                event.size = 0;
            }
            event.size += len;
        } else {
            break;
        }
    }
    array_ref->truncated = i == MAX_ARRAY_ELEMENT;

    // flush remaining values
    if (event.size > 0) {
        if (event.size > MAX_PERF_STR_BUFF_LEN) {
            event.size = MAX_PERF_STR_BUFF_LEN;
        }
        void *perf_ptr = &(buff->value[perf_offset&(MAX_STR_BUFF_LEN - MAX_ARRAY_ELEMENT_SIZE - 1)]);
        bpf_probe_read(&event.value, MAX_PERF_STR_BUFF_LEN, perf_ptr);

        send_event(ctx, EVENT_ARGS_ENVS, event);
    }
}

int __attribute__((always_inline)) trace__sys_execveat(struct pt_regs *ctx, const char **argv, const char **env) {
    struct syscall_cache_t syscall = {
        .type = SYSCALL_EXEC,
    };
    parse_str_array(ctx, &syscall.exec.args, argv);
    //parse_str_array(ctx, &syscall.exec.envs, env);

    cache_syscall(&syscall);
    return 0;
}

SYSCALL_KPROBE3(execve, const char *, filename, const char **, argv, const char **, env) {
    return trace__sys_execveat(ctx, argv, env);
}

SYSCALL_KPROBE4(execveat, int, fd, const char *, filename, const char **, argv, const char **, env) {
    return trace__sys_execveat(ctx, argv, env);
}

int __attribute__((always_inline)) handle_exec_event(struct pt_regs *ctx, struct syscall_cache_t *syscall) {
    if (syscall->exec.is_parsed) {
        return 0;
    }
    syscall->exec.is_parsed = 1;

    struct file *file = (struct file *)PT_REGS_PARM1(ctx);
    struct inode *inode = (struct inode *)PT_REGS_PARM2(ctx);
    struct path *path = &file->f_path;

    syscall->exec.dentry = get_file_dentry(file);
    syscall->exec.path_key = get_inode_key_path(inode, &file->f_path);
    syscall->exec.path_key.path_id = get_path_id(0);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct dentry *exec_dentry = get_path_dentry(path);
    struct proc_cache_t entry = {
        .executable = {
            .inode = syscall->exec.path_key.ino,
            .overlay_numlower = get_overlay_numlower(exec_dentry),
            .mount_id = get_path_mount_id(path),
            .path_id = syscall->exec.path_key.path_id,
        },
        .container = {},
        .exec_timestamp = bpf_ktime_get_ns(),
    };
    fill_file_metadata(exec_dentry, &entry.executable.metadata);
    bpf_get_current_comm(&entry.comm, sizeof(entry.comm));

    // cache dentry
    resolve_dentry(syscall->exec.dentry, syscall->exec.path_key, 0);

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

void __attribute__((always_inline)) fill_args_envs(struct exec_event_t *event, struct syscall_cache_t *syscall) {
    event->args_id = syscall->exec.args.id;
    event->args_truncated = syscall->exec.args.truncated;
    event->envs_id = syscall->exec.envs.id;
    event->envs_truncated = syscall->exec.envs.truncated;
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

            // fill args and envs
            fill_args_envs(&event, syscall);

            // send the entry to maintain userspace cache
            send_event(ctx, EVENT_EXEC, event);
        }
    }

    return 0;
}
#endif
