#ifndef _EXEC_H_
#define _EXEC_H_

#include <linux/tty.h>

#include "filters.h"
#include "syscalls.h"
#include "container.h"
#include "span.h"

#define MAX_PERF_STR_BUFF_LEN 256
#define MAX_STR_BUFF_LEN (1 << 15)
#define MAX_ARRAY_ELEMENT_PER_TAIL 28
#define MAX_ARRAY_ELEMENT_SIZE 4096
#define MAX_ARGS_ELEMENTS 140

struct args_envs_event_t {
    struct kevent_t event;
    u32 id;
    u32 size;
    char value[MAX_PERF_STR_BUFF_LEN];
};

struct str_array_buffer_t {
    char value[MAX_STR_BUFF_LEN];
};

struct bpf_map_def SEC("maps/args_envs_progs") args_envs_progs = {
    .type = BPF_MAP_TYPE_PROG_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 10,
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
    struct span_context_t span;
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
    struct span_context_t span;
    struct container_context_t container;
    u32 exit_code;
};

struct _tracepoint_sched_process_fork {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    char parent_comm[16];
    pid_t parent_pid;
    char child_comm[16];
    pid_t child_pid;
};

struct _tracepoint_sched_process_exec {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    int data_loc_filename;
    pid_t pid;
    pid_t old_pid;
};

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

struct bpf_map_def SEC("maps/pid_ignored") pid_ignored = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 16738,
    .pinning = 0,
    .namespace = "",
};

struct proc_cache_t __attribute__((always_inline)) *get_proc_from_cookie(u32 cookie) {
    if (!cookie) {
        return NULL;
    }

    return bpf_map_lookup_elem(&proc_cache, &cookie);
}

void __attribute__((always_inline)) parse_str_array(struct pt_regs *ctx, struct str_array_ref_t *array_ref, u64 event_type) {
    const char **array = array_ref->array;
    int index = array_ref->index;
    if (index == 255) {
        return;
    }

    array_ref->truncated = 0;

    u32 key = 0;
    struct str_array_buffer_t *buff = bpf_map_lookup_elem(&str_array_buffers, &key);
    if (!buff) {
        return;
    }

    const char *str;
    bpf_probe_read(&str, sizeof(str), (void *)&array[index]);

    struct args_envs_event_t event = {
        .id = array_ref->id,
    };

    int i = 0;
    int n = 0;

    void *perf_ptr = &buff->value[0];

#pragma unroll
    for (i = 0; i < MAX_ARRAY_ELEMENT_PER_TAIL; i++) {
        void *ptr = &(buff->value[(event.size + sizeof(n)) & (MAX_STR_BUFF_LEN - MAX_ARRAY_ELEMENT_SIZE - 1)]);

        n = bpf_probe_read_str(ptr, MAX_ARRAY_ELEMENT_SIZE, (void *)str);
        if (n > 0) {
            n--; // remove trailing 0

            // insert size before the string
            bpf_probe_read(&(buff->value[event.size&(MAX_STR_BUFF_LEN - MAX_ARRAY_ELEMENT_SIZE - 1)]), sizeof(n), &n);

            int len = n + sizeof(n);
            if (event.size + len >= MAX_PERF_STR_BUFF_LEN) {
                // copy value to the event
                bpf_probe_read(&event.value, MAX_PERF_STR_BUFF_LEN, perf_ptr);

                // an only one argument overflow the limit
                if (event.size == 0) {
                    event.size = MAX_PERF_STR_BUFF_LEN;
                    index++;
                }

                send_event(ctx, event_type, event);
                event.size = 0;
            } else {
                event.size += len;
                index++;
            }

            bpf_probe_read(&str, sizeof(str), (void *)&array[index]);
        } else {
            index = 255; // stop here
            break;
        }
    }
    array_ref->index = index;
    array_ref->truncated = i == MAX_ARRAY_ELEMENT_PER_TAIL;

    // flush remaining values
    if (event.size > 0) {
        bpf_probe_read(&event.value, MAX_PERF_STR_BUFF_LEN, perf_ptr);

        send_event(ctx, event_type, event);
    }
}

SEC("kprobe/parse_args_envs")
int kprobe_parse_args_envs(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_EXEC);
    if (!syscall) {
        return 0;
    }

    struct str_array_ref_t *array = &syscall->exec.args;
    if (syscall->exec.next_tail > MAX_ARGS_ELEMENTS / MAX_ARRAY_ELEMENT_PER_TAIL) {
        array = &syscall->exec.envs;
    }
    parse_str_array(ctx, array, EVENT_ARGS_ENVS);

    syscall->exec.next_tail++;

    bpf_tail_call_compat(ctx, &args_envs_progs, syscall->exec.next_tail);

    return 0;
}

int __attribute__((always_inline)) trace__sys_execveat(struct pt_regs *ctx, const char **argv, const char **env) {
    struct syscall_cache_t syscall = {
        .type = EVENT_EXEC,
        .exec = {
            .args = {
                .id = bpf_get_prandom_u32(),
                .array = argv,
                .index = 0,
            },
            .envs = {
                .id = bpf_get_prandom_u32(),
                .array = env,
            }
        }
    };
    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE3(execve, const char *, filename, const char **, argv, const char **, env) {
    return trace__sys_execveat(ctx, argv, env);
}

SYSCALL_KPROBE4(execveat, int, fd, const char *, filename, const char **, argv, const char **, env) {
    return trace__sys_execveat(ctx, argv, env);
}

int __attribute__((always_inline)) handle_exec_event(struct pt_regs *ctx, struct syscall_cache_t *syscall, struct file *file, struct path *path, struct inode *inode) {
    if (syscall->exec.is_parsed) {
        return 0;
    }
    syscall->exec.is_parsed = 1;

    syscall->exec.dentry = get_file_dentry(file);
    syscall->exec.file.path_key = get_inode_key_path(inode, path);
    syscall->exec.file.path_key.path_id = get_path_id(0);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct dentry *exec_dentry = get_path_dentry(path);
    struct proc_cache_t entry = {
        .executable = {
            .path_key = {
                .ino = syscall->exec.file.path_key.ino,
                .mount_id = get_path_mount_id(path),
                .path_id = syscall->exec.file.path_key.path_id,
            },
            .flags = syscall->exec.file.flags,
        },
        .container = {},
        .exec_timestamp = bpf_ktime_get_ns(),
    };
    fill_file_metadata(exec_dentry, &entry.executable.metadata);
    set_file_inode(exec_dentry, &entry.executable, 0);
    bpf_get_current_comm(&entry.comm, sizeof(entry.comm));

    // select the previous cookie entry in cache of the current process
    // (this entry was created by the fork of the current process)
    struct pid_cache_t *fork_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
    if (fork_entry) {
        // Fetch the parent proc cache entry
        u32 parent_cookie = fork_entry->cookie;
        struct proc_cache_t *parent_entry = get_proc_from_cookie(parent_cookie);
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

    // resolve dentry
    syscall->resolver.key = syscall->exec.file.path_key;
    syscall->resolver.dentry = syscall->exec.dentry;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = DR_NO_CALLBACK;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE);
    return 0;
}

int __attribute__((always_inline)) handle_sys_fork(struct pt_regs *ctx) {
    struct syscall_cache_t syscall = {
        .type = EVENT_FORK,
    };

    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE0(fork) {
    return handle_sys_fork(ctx);
}

SYSCALL_KPROBE0(clone) {
    return handle_sys_fork(ctx);
}

SYSCALL_KPROBE0(clone3) {
    return handle_sys_fork(ctx);
}

SYSCALL_KPROBE0(vfork) {
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
int kprobe_kernel_clone(struct pt_regs *ctx) {
    return handle_do_fork(ctx);
}

SEC("kprobe/do_fork")
int kprobe_do_fork(struct pt_regs *ctx) {
    return handle_do_fork(ctx);
}

SEC("kprobe/_do_fork")
int kprobe__do_fork(struct pt_regs *ctx) {
    return handle_do_fork(ctx);
}

SEC("kretprobe/alloc_pid")
int kretprobe_alloc_pid(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_FORK);
    if (!syscall) {
        return 0;
    }

    // cache the struct pid in the syscall cache, it will be populated by `alloc_pid`
    struct pid *pid = (struct pid *) PT_REGS_RC(ctx);
    bpf_probe_read(&syscall->fork.pid, sizeof(syscall->fork.pid), &pid);
    return 0;
}

// There is only one use case for this hook point: fetch the nr translation for a long running process in a container,
// for which we missed the fork, and that finally decides to exec without cloning first.
// Note that in most cases (except the exec one), bpf_get_current_pid_tgid won't match the input task.
// TODO(will): replace this hook point by a snapshot
SEC("kretprobe/__task_pid_nr_ns")
int kretprobe__task_pid_nr_ns(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_EXEC);
    if (!syscall) {
        return 0;
    }

    u32 root_nr = bpf_get_current_pid_tgid();
    u32 namespace_nr = (pid_t) PT_REGS_RC(ctx);

    // no namespace
    if (!namespace_nr || root_nr == namespace_nr) {
      return 0;
    }

    register_nr(root_nr, namespace_nr);
    return 0;
}

SEC("tracepoint/sched/sched_process_exec")
int sched_process_exec(struct _tracepoint_sched_process_exec *args) {
    // prepare filename pointer
    unsigned short __offset = args->data_loc_filename & 0xFFFF;
    char *filename = (char *)args + __offset;

    struct exec_path key = {};
    bpf_probe_read_str(&key.filename, MAX_PATH_LEN, filename);

    struct bpf_map_def *exec_count = select_buffer(&exec_count_fb, &exec_count_bb, SYSCALL_MONITOR_KEY);
    if (exec_count == NULL) {
        return 0;
    }

    u64 zero = 0;
    u64 *count = bpf_map_lookup_or_try_init(exec_count, &key, &zero);
    if (count == NULL) {
        return 0;
    }

    __sync_fetch_and_add(count, 1);

    return 0;
}

SEC("tracepoint/sched/sched_process_fork")
int sched_process_fork(struct _tracepoint_sched_process_fork *args) {
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

    u32 parent_pid = args->parent_pid;
    bpf_probe_read(&parent_pid, sizeof(parent_pid), &args->child_pid);
    u32 *netns = bpf_map_lookup_elem(&netns_cache, &parent_pid);
    if (netns != NULL) {
        u32 child_netns_entry = *netns;
        bpf_map_update_elem(&netns_cache, &pid, &child_netns_entry, BPF_ANY);
    }

    // cache namespace nr translations
    cache_nr_translations(syscall->fork.pid);

    // if this is a thread, leave
    if (syscall->fork.is_thread) {
        return 0;
    }

    u64 ts = bpf_ktime_get_ns();
    struct exec_event_t event = {
        .pid_entry.fork_timestamp = ts,
    };
    bpf_get_current_comm(&event.proc_entry.comm, sizeof(event.proc_entry.comm));
    fill_process_context(&event.process);
    fill_span_context(&event.span);

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

    struct pid_cache_t *parent_pid_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &ppid);
    if (parent_pid_entry) {
        // ensure pid and ppid point to the same cookie
        event.pid_entry.cookie = parent_pid_entry->cookie;

        // ensure pid and ppid have the same credentials
        event.pid_entry.credentials = parent_pid_entry->credentials;

        // fetch the parent proc cache entry
        struct proc_cache_t *parent_proc_entry = get_proc_from_cookie(event.pid_entry.cookie);
        if (parent_proc_entry) {
            copy_proc_cache_except_comm(parent_proc_entry, &event.proc_entry);
        }
    }

    // insert the pid cache entry for the new process
    bpf_map_update_elem(&pid_cache, &pid, &event.pid_entry, BPF_ANY);

    // [activity_dump] inherit tracing state
    inherit_traced_state(args, ppid, pid, event.proc_entry.container.container_id, event.proc_entry.comm);

    // send the entry to maintain userspace cache
    send_event(args, EVENT_FORK, event);

    return 0;
}

SEC("kprobe/do_exit")
int kprobe_do_exit(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;
    u32 pid = pid_tgid;

    void *ignored = bpf_map_lookup_elem(&pid_ignored, &pid);
    if (ignored) {
        bpf_map_delete_elem(&pid_ignored, &pid);
        return 0;
    }

    // delete netns entry
    bpf_map_delete_elem(&netns_cache, &pid);

    if (tgid == pid) {
        if (!is_flushing_discarders()) {
            remove_pid_discarder(tgid);
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
        fill_span_context(&event.span);
        event.exit_code = (u32)PT_REGS_PARM1(ctx);
        send_event(ctx, EVENT_EXIT, event);

        unregister_span_memory();

        // [activity_dump] cleanup tracing state for this pid
        cleanup_traced_state(tgid);
    }

    // remove nr translations
    remove_nr(pid);

    return 0;
}

SEC("kprobe/exit_itimers")
int kprobe_exit_itimers(struct pt_regs *ctx) {
    void *signal = (void *)PT_REGS_PARM1(ctx);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct proc_cache_t *entry = get_proc_cache(tgid);
    if (entry) {
        u64 tty_offset;
        LOAD_CONSTANT("tty_offset", tty_offset);

        u64 tty_name_offset;
        LOAD_CONSTANT("tty_name_offset", tty_name_offset);

        struct tty_struct *tty;
        bpf_probe_read(&tty, sizeof(tty), (char *)signal + tty_offset);
        if (tty) {
            bpf_probe_read_str(entry->tty_name, TTY_NAME_LEN, (char *)tty + tty_name_offset);
        }
    }

    return 0;
}

int __attribute__((always_inline)) parse_args_and_env(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_EXEC);
    if (!syscall) {
        return 0;
    }

    // call it here before the memory get replaced
    fill_span_context(&syscall->exec.span_context);

    bpf_tail_call_compat(ctx, &args_envs_progs, syscall->exec.next_tail);
    return 0;
}

SEC("kprobe/prepare_binprm")
int kprobe_prepare_binprm(struct pt_regs *ctx) {
    return parse_args_and_env(ctx);
}

SEC("kprobe/bprm_execve")
int kprobe_bprm_execve(struct pt_regs *ctx) {
    return parse_args_and_env(ctx);
}

SEC("kprobe/security_bprm_check")
int kprobe_security_bprm_check(struct pt_regs *ctx) {
    return parse_args_and_env(ctx);
}

void __attribute__((always_inline)) fill_args_envs(struct exec_event_t *event, struct syscall_cache_t *syscall) {
    event->args_id = syscall->exec.args.id;
    event->args_truncated = syscall->exec.args.truncated;
    event->envs_id = syscall->exec.envs.id;
    event->envs_truncated = syscall->exec.envs.truncated;
}

SEC("kprobe/security_bprm_committed_creds")
int kprobe_security_bprm_committed_creds(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_EXEC);
    if (!syscall) {
        return 0;
    }

    // check if this is a thread first
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 now = bpf_ktime_get_ns();
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

            copy_span_context(&syscall->exec.span_context, &event.span);
            fill_args_envs(&event, syscall);

            // [activity_dump] check if this process should be traced
            should_trace_new_process(ctx, now, tgid, event.proc_entry.container.container_id, event.proc_entry.comm);

            // send the entry to maintain userspace cache
            send_event(ctx, EVENT_EXEC, event);
        }
    }

    return 0;
}

#endif
