#ifndef _EXEC_H_
#define _EXEC_H_

#include <linux/tty.h>
#include <linux/binfmts.h>

#include "filters.h"
#include "syscalls.h"
#include "container.h"
#include "span.h"

#define MAX_PERF_STR_BUFF_LEN 256
#define MAX_STR_BUFF_LEN (1 << 15)
#define MAX_ARRAY_ELEMENT_SIZE 4096
#define MAX_ARRAY_ELEMENT_PER_TAIL 28
#define MAX_ARGS_ELEMENTS (MAX_ARRAY_ELEMENT_PER_TAIL * (32 / 2)) // split tailcall limit
#define MAX_ARGS_READ_PER_TAIL 208

struct args_envs_event_t {
    struct kevent_t event;
    u32 id;
    u32 size;
    char value[MAX_PERF_STR_BUFF_LEN];
};

struct str_array_buffer_t {
    char value[MAX_STR_BUFF_LEN];
};

#define EXEC_GET_ENVS_OFFSET 0
#define EXEC_PARSE_ARGS_ENVS_SPLIT 1
#define EXEC_PARSE_ARGS_ENVS 2

struct bpf_map_def SEC("maps/args_envs_progs") args_envs_progs = {
    .type = BPF_MAP_TYPE_PROG_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 3,
};

struct bpf_map_def SEC("maps/str_array_buffers") str_array_buffers = {
    .type = BPF_MAP_TYPE_PERCPU_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct str_array_buffer_t),
    .max_entries = 1,
};

struct process_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct process_entry_t proc_entry;
    struct pid_cache_t pid_entry;
    struct linux_binprm_t linux_binprm;
    u32 args_id;
    u32 args_truncated;
    u32 envs_id;
    u32 envs_truncated;
};

// _gen is a suffix for maps storing large structs to work around ebpf object size limits
struct bpf_map_def SEC("maps/process_event_gen") process_event_gen = {
    .type = BPF_MAP_TYPE_PERCPU_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct process_event_t),
    .max_entries = 1,
};

__attribute__((always_inline)) struct process_event_t *new_process_event(u8 is_fork) {
    u32 key = 0;
    struct process_event_t *evt = bpf_map_lookup_elem(&process_event_gen, &key);

    if (evt) {
        __builtin_memset(evt, 0, sizeof(*evt));
        if (!is_fork) {
            evt->event.flags |= EVENT_FLAGS_ACTIVITY_DUMP_SAMPLE;
        }
    }

    return evt;
}

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
};

struct bpf_map_def SEC("maps/exec_count_bb") exec_count_bb = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct exec_path),
    .value_size = sizeof(u64),
    .max_entries = 2048,
};

struct bpf_map_def SEC("maps/tasks_in_coredump") tasks_in_coredump = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(u8),
    .max_entries = 64,
    .map_flags = BPF_F_NO_COMMON_LRU,
};

struct bpf_map_def SEC("maps/exec_pid_transfer") exec_pid_transfer = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(u64),
    .max_entries = 512,
};

struct proc_cache_t __attribute__((always_inline)) *get_proc_from_cookie(u32 cookie) {
    if (!cookie) {
        return NULL;
    }

    return bpf_map_lookup_elem(&proc_cache, &cookie);
}

int __attribute__((always_inline)) trace__sys_execveat(struct pt_regs *ctx, const char **argv, const char **env) {
    struct syscall_cache_t syscall = {
        .type = EVENT_EXEC,
        .exec = {
            .args = {
                .id = bpf_get_prandom_u32(),
            },
            .envs = {
                .id = bpf_get_prandom_u32(),
            }
        }
    };
    cache_syscall(&syscall);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;
    u32 pid = pid_tgid;
    // exec is called from a non leader thread:
    //   - we need to remember that this thread will change its pid to the thread group leader's in the flush_old_exec kernel function,
    //     before sending the event to userspace
    //   - because the "real" thread leader will be terminated during this exec syscall, we also need to make sure to not send
    //     the corresponding exit event
    if (tgid != pid) {
        bpf_map_update_elem(&exec_pid_transfer, &tgid, &pid_tgid, BPF_ANY);
    }

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

    // set mount_id to 0 is this is a fileless exec, meaning that the vfs type is tmpfs and that is an internal mount
    u32 mount_id = is_tmpfs(syscall->exec.dentry) && get_path_mount_flags(path) & MNT_INTERNAL ? 0 : get_path_mount_id(path);

    syscall->exec.file.path_key.ino = get_inode_ino(inode);
    syscall->exec.file.path_key.mount_id = mount_id;
    syscall->exec.file.path_key.path_id = get_path_id(0);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct dentry *exec_dentry = get_path_dentry(path);
    struct proc_cache_t pc = {
        .entry = {
            .executable = {
                .path_key = {
                    .ino = syscall->exec.file.path_key.ino,
                    .mount_id = mount_id,
                    .path_id = syscall->exec.file.path_key.path_id,
                },
                .flags = syscall->exec.file.flags
            },
            .exec_timestamp = bpf_ktime_get_ns(),
        },
        .container = {},
    };
    fill_file_metadata(exec_dentry, &pc.entry.executable.metadata);
    set_file_inode(exec_dentry, &pc.entry.executable, 0);
    bpf_get_current_comm(&pc.entry.comm, sizeof(pc.entry.comm));

    // select the previous cookie entry in cache of the current process
    // (this entry was created by the fork of the current process)
    struct pid_cache_t *fork_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
    if (fork_entry) {
        // Fetch the parent proc cache entry
        u32 parent_cookie = fork_entry->cookie;
        struct proc_cache_t *parent_pc = get_proc_from_cookie(parent_cookie);
        if (parent_pc) {
            // inherit the parent container context
            fill_container_context(parent_pc, &pc.container);
        }
    }

    // Insert new proc cache entry (Note: do not move the order of this block with the previous one, we need to inherit
    // the container ID before saving the entry in proc_cache. Modifying entry after insertion won't work.)
    u32 cookie = bpf_get_prandom_u32();
    bpf_map_update_elem(&proc_cache, &cookie, &pc, BPF_ANY);

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

int __attribute__((always_inline)) handle_interpreted_exec_event(struct pt_regs *ctx, struct syscall_cache_t *syscall, struct file *file) {
    struct inode *interpreter_inode;
    bpf_probe_read(&interpreter_inode, sizeof(interpreter_inode), &file->f_inode);

    syscall->exec.linux_binprm.interpreter = get_inode_key_path(interpreter_inode, &file->f_path);
    syscall->exec.linux_binprm.interpreter.path_id = get_path_id(0);

#ifdef DEBUG
    bpf_printk("interpreter file: %llx\n", file);
    bpf_printk("interpreter inode: %u\n", syscall->exec.linux_binprm.interpreter.ino);
    bpf_printk("interpreter mount id: %u %u %u\n", syscall->exec.linux_binprm.interpreter.mount_id, get_file_mount_id(file), get_path_mount_id(&file->f_path));
    bpf_printk("interpreter path id: %u\n", syscall->exec.linux_binprm.interpreter.path_id);
#endif

    // Add interpreter path to map/pathnames, which is used by the dentry resolver.
    // This overwrites the resolver fields on this syscall, but that's ok because the executed file has already been written to the map/pathnames ebpf map.
    syscall->resolver.key = syscall->exec.linux_binprm.interpreter;
    syscall->resolver.dentry = get_file_dentry(file);
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

    u32 parent_pid = 0;
    bpf_probe_read(&parent_pid, sizeof(parent_pid), &args->parent_pid);
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
    struct process_event_t *event = new_process_event(1);
    if (event == NULL) {
        return 0;
    }

    event->pid_entry.fork_timestamp = ts;

    bpf_get_current_comm(event->proc_entry.comm, sizeof(event->proc_entry.comm));
    struct process_context_t *on_stack_process = &event->process;
    fill_process_context(on_stack_process);
    fill_span_context(&event->span);

    // the `parent_pid` entry of `sched_process_fork` might point to the TID (and not PID) of the parent. Since we
    // only work with PID, we can't use the TID. This is why we use the PID generated by the eBPF context instead.
    u32 ppid = event->process.pid;
    event->pid_entry.ppid = ppid;
    // sched::sched_process_fork is triggered from the parent process, update the pid / tid to the child value
    event->process.pid = pid;
    event->process.tid = pid;

    // ignore kthreads
    if (IS_KTHREAD(ppid, pid)) {
        return 0;
    }

    struct pid_cache_t *parent_pid_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &ppid);
    if (parent_pid_entry) {
        // ensure pid and ppid point to the same cookie
        event->pid_entry.cookie = parent_pid_entry->cookie;

        // ensure pid and ppid have the same credentials
        event->pid_entry.credentials = parent_pid_entry->credentials;

        // fetch the parent proc cache entry
        u32 on_stack_cookie = event->pid_entry.cookie;
        struct proc_cache_t *parent_pc = get_proc_from_cookie(on_stack_cookie);
        if (parent_pc) {
            fill_container_context(parent_pc, &event->container);
            copy_proc_entry_except_comm(&parent_pc->entry, &event->proc_entry);
        }
    }

    struct pid_cache_t on_stack_pid_entry = event->pid_entry;
    // insert the pid cache entry for the new process
    bpf_map_update_elem(&pid_cache, &pid, &on_stack_pid_entry, BPF_ANY);

    // [activity_dump] inherit tracing state
    inherit_traced_state(args, ppid, pid, event->container.container_id, event->proc_entry.comm);

    // send the entry to maintain userspace cache
    send_event_ptr(args, EVENT_FORK, event);

    return 0;
}

SEC("kprobe/do_coredump")
int kprobe_do_coredump(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    u8 in_coredump = 1;

    bpf_map_update_elem(&tasks_in_coredump, &key, &in_coredump, BPF_ANY);

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

    u64 *pid_tgid_execing = (u64 *)bpf_map_lookup_elem(&exec_pid_transfer, &tgid);

    // only send the exit event if this is the thread group leader that isn't being killed by an execing thread
    if (tgid == pid && pid_tgid_execing == NULL) {
        expire_pid_discarder(tgid);

        // update exit time
        struct pid_cache_t *pid_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
        if (pid_entry) {
            pid_entry->exit_timestamp = bpf_ktime_get_ns();
        }

        // send the entry to maintain userspace cache
        struct exit_event_t event = {};
        struct proc_cache_t *pc = fill_process_context(&event.process);
        fill_container_context(pc, &event.container);
        fill_span_context(&event.span);
        event.exit_code = (u32)PT_REGS_PARM1(ctx);
        u8 *in_coredump = (u8 *)bpf_map_lookup_elem(&tasks_in_coredump, &pid_tgid);
        if (in_coredump) {
            event.exit_code |= 0x80;
            bpf_map_delete_elem(&tasks_in_coredump, &pid_tgid);
        }
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

    struct proc_cache_t *pc = get_proc_cache(tgid);
    if (pc) {
        u64 tty_offset;
        LOAD_CONSTANT("tty_offset", tty_offset);

        u64 tty_name_offset;
        LOAD_CONSTANT("tty_name_offset", tty_name_offset);

        struct tty_struct *tty;
        bpf_probe_read(&tty, sizeof(tty), (char *)signal + tty_offset);
        if (tty) {
            bpf_probe_read_str(pc->entry.tty_name, TTY_NAME_LEN, (char *)tty + tty_name_offset);
        }
    }

    return 0;
}

// the following functions must use the {peek,pop}_current_or_impersonated_exec_syscall to retrieve the syscall context
// because the task performing the exec syscall may change its pid in the flush_old_exec() kernel function

struct syscall_cache_t *__attribute__((always_inline)) peek_current_or_impersonated_exec_syscall() {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_EXEC);
    if (!syscall) {
        u64 pid_tgid = bpf_get_current_pid_tgid();
        u32 tgid = pid_tgid >> 32;
        u32 pid = pid_tgid;
        u64 *pid_tgid_execing_ptr = (u64 *)bpf_map_lookup_elem(&exec_pid_transfer, &tgid);
        if (!pid_tgid_execing_ptr) {
            return NULL;
        }
        u64 pid_tgid_execing = *pid_tgid_execing_ptr;
        u32 tgid_execing = pid_tgid_execing >> 32;
        u32 pid_execing = pid_tgid_execing;
        if (tgid != tgid_execing || pid == pid_execing) {
            return NULL;
        }
        // the current task is impersonating its thread group leader
        syscall = peek_task_syscall(pid_tgid_execing, EVENT_EXEC);
    }
    return syscall;
}

struct syscall_cache_t *__attribute__((always_inline)) pop_current_or_impersonated_exec_syscall() {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_EXEC);
    if (!syscall) {
        u64 pid_tgid = bpf_get_current_pid_tgid();
        u32 tgid = pid_tgid >> 32;
        u32 pid = pid_tgid;
        u64 *pid_tgid_execing_ptr = (u64 *)bpf_map_lookup_elem(&exec_pid_transfer, &tgid);
        if (!pid_tgid_execing_ptr) {
            return NULL;
        }
        u64 pid_tgid_execing = *pid_tgid_execing_ptr;
        u32 tgid_execing = pid_tgid_execing >> 32;
        u32 pid_execing = pid_tgid_execing;
        if (tgid != tgid_execing || pid == pid_execing) {
            return NULL;
        }
        // the current task is impersonating its thread group leader
        syscall = pop_task_syscall(pid_tgid_execing, EVENT_EXEC);
    }
    return syscall;
}

int __attribute__((always_inline)) fill_exec_context(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_current_or_impersonated_exec_syscall();
    if (!syscall) {
        return 0;
    }

    // call it here before the memory get replaced
    fill_span_context(&syscall->exec.span_context);

    return 0;
}

SEC("kprobe/prepare_binprm")
int kprobe_prepare_binprm(struct pt_regs *ctx) {
    return fill_exec_context(ctx);
}

SEC("kprobe/bprm_execve")
int kprobe_bprm_execve(struct pt_regs *ctx) {
    return fill_exec_context(ctx);
}

SEC("kprobe/security_bprm_check")
int kprobe_security_bprm_check(struct pt_regs *ctx) {
    return fill_exec_context(ctx);
}

SEC("kprobe/get_envs_offset")
int kprobe_get_envs_offset(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_current_or_impersonated_exec_syscall();
    if (!syscall) {
        return 0;
    }

    u32 key = 0;
    struct str_array_buffer_t *buff = bpf_map_lookup_elem(&str_array_buffers, &key);
    if (!buff) {
        return 0;
    }

    int i;
    long bytes_read;
    const char *args_start = syscall->exec.args_envs_ctx.args_start;
    u64 offset = syscall->exec.args_envs_ctx.envs_offset;
    u32 args_count = syscall->exec.args_envs_ctx.args_count;

#pragma unroll
    for (i = 0; i < MAX_ARGS_READ_PER_TAIL && args_count < syscall->exec.args.count; i++) {
        bytes_read = bpf_probe_read_str(&buff->value[0], MAX_ARRAY_ELEMENT_SIZE, (void *)(args_start + offset));
        if (bytes_read <= 0 || bytes_read == MAX_ARRAY_ELEMENT_SIZE) {
            syscall->exec.args_envs_ctx.envs_offset = 0;
            return 0;
        }
        offset += bytes_read;
        args_count++;
    }

    syscall->exec.args_envs_ctx.envs_offset = offset;
    syscall->exec.args_envs_ctx.args_count = args_count;

    if (args_count == syscall->exec.args.count) {
        return 0;
    }

    bpf_tail_call_compat(ctx, &args_envs_progs, EXEC_GET_ENVS_OFFSET);

    // make sure to reset envs_offset if the tailcall limit is reached and all args couldn't be read
    if (args_count != syscall->exec.args.count) {
        syscall->exec.args_envs_ctx.envs_offset = 0;
    }

    return 0;
}

void __attribute__((always_inline)) parse_args_envs(struct pt_regs *ctx, struct args_envs_parsing_context_t *args_envs_ctx, struct args_envs_t *args_envs) {
    const char *args_start = args_envs_ctx->args_start;
    int offset = args_envs_ctx->parsing_offset;

    args_envs->truncated = 0;

    u32 key = 0;
    struct str_array_buffer_t *buff = bpf_map_lookup_elem(&str_array_buffers, &key);
    if (!buff) {
        return;
    }

    struct args_envs_event_t event = {
        .id = args_envs->id,
    };

    int i = 0;
    int bytes_read = 0;

    void *buff_ptr = &buff->value[0];

#pragma unroll
    for (i = 0; i < MAX_ARRAY_ELEMENT_PER_TAIL; i++) {
        void *string_array_ptr = &(buff->value[(event.size + sizeof(bytes_read)) & (MAX_STR_BUFF_LEN - MAX_ARRAY_ELEMENT_SIZE - 1)]);

        bytes_read = bpf_probe_read_str(string_array_ptr, MAX_ARRAY_ELEMENT_SIZE, (void *)(args_start + offset));
        if (bytes_read > 0) {
            bytes_read--; // remove trailing 0

            // insert size before the string
            bpf_probe_read(&(buff->value[event.size&(MAX_STR_BUFF_LEN - MAX_ARRAY_ELEMENT_SIZE - 1)]), sizeof(bytes_read), &bytes_read);

            int data_length = bytes_read + sizeof(bytes_read);
            if (event.size + data_length >= MAX_PERF_STR_BUFF_LEN) {
                // copy value to the event
                bpf_probe_read(&event.value, MAX_PERF_STR_BUFF_LEN, buff_ptr);

                // only one argument overflows the limit
                if (event.size == 0) {
                    event.size = MAX_PERF_STR_BUFF_LEN;
                    args_envs->counter++;
                    offset += bytes_read + 1; // count trailing 0
                }

                send_event(ctx, EVENT_ARGS_ENVS, event);
                event.size = 0;
            } else {
                event.size += data_length;
                args_envs->counter++;
                offset += bytes_read + 1;
            }

            if (args_envs->counter == args_envs->count) {
                break;
            }
        } else {
            break;
        }
    }
    args_envs_ctx->parsing_offset = offset;
    args_envs->truncated = i == MAX_ARRAY_ELEMENT_PER_TAIL;

    // flush remaining values
    if (event.size > 0) {
        bpf_probe_read(&event.value, MAX_PERF_STR_BUFF_LEN, buff_ptr);

        send_event(ctx, EVENT_ARGS_ENVS, event);
    }
}

SEC("kprobe/parse_args_envs_split")
int kprobe_parse_args_envs_split(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_current_or_impersonated_exec_syscall();
    if (!syscall) {
        return 0;
    }

    struct args_envs_t *args_envs;

    if (syscall->exec.args.counter < syscall->exec.args.count && syscall->exec.args.counter <= MAX_ARGS_ELEMENTS) {
        args_envs = &syscall->exec.args;
    } else if (syscall->exec.envs.counter < syscall->exec.envs.count) {
        if (syscall->exec.envs.counter == 0) {
            syscall->exec.args_envs_ctx.parsing_offset = syscall->exec.args_envs_ctx.envs_offset;
        }
        args_envs = &syscall->exec.envs;
    } else {
        return 0;
    }

    parse_args_envs(ctx, &syscall->exec.args_envs_ctx, args_envs);

    bpf_tail_call_compat(ctx, &args_envs_progs, EXEC_PARSE_ARGS_ENVS_SPLIT);

    args_envs->truncated = 1;

    return 0;
}

SEC("kprobe/parse_args_envs")
int kprobe_parse_args_envs(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_current_or_impersonated_exec_syscall();
    if (!syscall) {
        return 0;
    }

    struct args_envs_t *args_envs;

    if (syscall->exec.args.counter < syscall->exec.args.count) {
        args_envs = &syscall->exec.args;
    } else if (syscall->exec.envs.counter < syscall->exec.envs.count) {
        args_envs = &syscall->exec.envs;
    } else {
        return 0;
    }

    parse_args_envs(ctx, &syscall->exec.args_envs_ctx, args_envs);

    bpf_tail_call_compat(ctx, &args_envs_progs, EXEC_PARSE_ARGS_ENVS);

    args_envs->truncated = 1;

    return 0;
}

int __attribute__((always_inline)) fetch_interpreter(struct pt_regs *ctx, struct linux_binprm *bprm) {
    struct syscall_cache_t *syscall = peek_current_or_impersonated_exec_syscall();
    if (!syscall) {
        return 0;
    }

    u64 binprm_file_offset;
    LOAD_CONSTANT("binprm_file_offset", binprm_file_offset);

    // The executable contains information about the interpreter
    struct file *interpreter;
    bpf_probe_read(&interpreter, sizeof(interpreter), (char *)bprm + binprm_file_offset);

#ifdef DEBUG
    bpf_printk("binprm_file_offset: %d\n", binprm_file_offset);

    bpf_printk("interpreter file: %llx\n", interpreter);

    const char *s;
    bpf_probe_read(&s, sizeof(s), &bprm->filename);
    bpf_printk("*filename from binprm: %s\n", s);

    bpf_probe_read(&s, sizeof(s), &bprm->interp);
    bpf_printk("*interp from binprm: %s\n", s);
#endif

    return handle_interpreted_exec_event(ctx, syscall, interpreter);
}

SEC("kprobe/setup_new_exec")
int kprobe_setup_new_exec_interp(struct pt_regs *ctx) {
    struct linux_binprm *bprm = (struct linux_binprm *) PT_REGS_PARM1(ctx);
    return fetch_interpreter(ctx, bprm);
}

SEC("kprobe/setup_new_exec")
int kprobe_setup_new_exec_args_envs(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_current_or_impersonated_exec_syscall();
    if (!syscall) {
        return 0;
    }

    void *bprm = (void *)PT_REGS_PARM1(ctx);

    int argc = 0;
    u64 argc_offset;
    LOAD_CONSTANT("linux_binprm_argc_offset", argc_offset);
    bpf_probe_read(&argc, sizeof(argc), (char *)bprm + argc_offset);

    int envc = 0;
    u64 envc_offset;
    LOAD_CONSTANT("linux_binprm_envc_offset", envc_offset);
    bpf_probe_read(&envc, sizeof(envc), (char *)bprm + envc_offset);

    unsigned long p = 0;
    u64 p_offset;
    LOAD_CONSTANT("linux_binprm_p_offset", p_offset);
    bpf_probe_read(&p, sizeof(p), (char *)bprm + p_offset);
    // if we fail to retrieve the pointer to the args then don't bother parsing them
    if (p == 0) {
        return 0;
    }

    syscall->exec.args_envs_ctx.args_start = (char *)p;
    syscall->exec.args_envs_ctx.args_count = 0;
    syscall->exec.args_envs_ctx.parsing_offset = 0;
    syscall->exec.args_envs_ctx.envs_offset = 0;
    syscall->exec.args.count = argc;
    syscall->exec.envs.count = envc;

    bpf_tail_call_compat(ctx, &args_envs_progs, EXEC_GET_ENVS_OFFSET);

    return 0;
}

SEC("kprobe/setup_arg_pages")
int kprobe_setup_arg_pages(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_current_or_impersonated_exec_syscall();
    if (!syscall) {
        return 0;
    }

    if (syscall->exec.args_envs_ctx.envs_offset != 0) {
        bpf_tail_call_compat(ctx, &args_envs_progs, EXEC_PARSE_ARGS_ENVS_SPLIT);
    } else {
        bpf_tail_call_compat(ctx, &args_envs_progs, EXEC_PARSE_ARGS_ENVS);
    }

    return 0;
}

void __attribute__((always_inline)) fill_args_envs(struct process_event_t *event, struct syscall_cache_t *syscall) {
    event->args_id = syscall->exec.args.id;
    event->args_truncated = syscall->exec.args.truncated;
    event->envs_id = syscall->exec.envs.id;
    event->envs_truncated = syscall->exec.envs.truncated;
}

int __attribute__((always_inline)) send_exec_event(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_current_or_impersonated_exec_syscall();
    if (!syscall) {
        return 0;
    }

    // check if this is a thread first
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 now = bpf_ktime_get_ns();
    u32 tgid = pid_tgid >> 32;

    bpf_map_delete_elem(&exec_pid_transfer, &tgid);

    struct pid_cache_t *pid_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
    if (pid_entry) {
        u32 cookie = pid_entry->cookie;
        struct proc_cache_t *pc = bpf_map_lookup_elem(&proc_cache, &cookie);
        if (pc) {
            struct process_event_t *event = new_process_event(0);
            if (event == NULL) {
                return 0;
            }

            // copy proc_cache data
            fill_container_context(pc, &event->container);
            copy_proc_entry_except_comm(&pc->entry, &event->proc_entry);
            bpf_get_current_comm(&event->proc_entry.comm, sizeof(event->proc_entry.comm));

            // copy pid_cache entry data
            copy_pid_cache_except_exit_ts(pid_entry, &event->pid_entry);

            // add pid / tid context
            struct process_context_t *on_stack_process = &event->process;
            fill_process_context(on_stack_process);

            copy_span_context(&syscall->exec.span_context, &event->span);
            fill_args_envs(event, syscall);

            // [activity_dump] check if this process should be traced
            should_trace_new_process(ctx, now, tgid, event->container.container_id, event->proc_entry.comm);

            // add interpreter path info
            event->linux_binprm.interpreter = syscall->exec.linux_binprm.interpreter;

            // send the entry to maintain userspace cache
            send_event_ptr(ctx, EVENT_EXEC, event);
        }
    }

    return 0;
}

SEC("kprobe/mprotect_fixup")
int kprobe_mprotect_fixup(struct pt_regs *ctx) {
    return send_exec_event(ctx);
}

#endif
