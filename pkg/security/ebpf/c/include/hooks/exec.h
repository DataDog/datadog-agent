#ifndef _HOOKS_EXEC_H_
#define _HOOKS_EXEC_H_

#include "constants/syscall_macro.h"
#include "constants/offsets/filesystem.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"
#include "constants/fentry_macro.h"

int __attribute__((always_inline)) trace__sys_execveat(ctx_t *ctx, const char **argv, const char **env) {
    struct syscall_cache_t syscall = {
        .type = EVENT_EXEC,
        .exec = {
            .args = {
                .id = rand32(),
            },
            .envs = {
                .id = rand32(),
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

HOOK_SYSCALL_ENTRY3(execve, const char *, filename, const char **, argv, const char **, env) {
    return trace__sys_execveat(ctx, argv, env);
}

HOOK_SYSCALL_ENTRY4(execveat, int, fd, const char *, filename, const char **, argv, const char **, env) {
    return trace__sys_execveat(ctx, argv, env);
}

int __attribute__((always_inline)) handle_interpreted_exec_event(void *ctx, struct syscall_cache_t *syscall, struct file *file) {
    struct inode *interpreter_inode;
    bpf_probe_read(&interpreter_inode, sizeof(interpreter_inode), &file->f_inode);

    syscall->exec.linux_binprm.interpreter = get_inode_key_path(interpreter_inode, &file->f_path);
    syscall->exec.linux_binprm.interpreter.path_id = get_path_id(syscall->exec.linux_binprm.interpreter.mount_id, 0);

#ifdef DEBUG
    bpf_printk("interpreter file: %llx", file);
    bpf_printk("interpreter inode: %u", syscall->exec.linux_binprm.interpreter.ino);
    bpf_printk("interpreter mount id: %u %u %u", syscall->exec.linux_binprm.interpreter.mount_id, get_file_mount_id(file), get_path_mount_id(&file->f_path));
    bpf_printk("interpreter path id: %u", syscall->exec.linux_binprm.interpreter.path_id);
#endif

    // Add interpreter path to map/pathnames, which is used by the dentry resolver.
    // This overwrites the resolver fields on this syscall, but that's ok because the executed file has already been written to the map/pathnames ebpf map.
    syscall->resolver.key = syscall->exec.linux_binprm.interpreter;
    syscall->resolver.dentry = get_file_dentry(file);
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = DR_NO_CALLBACK;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE_OR_FENTRY);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_current_or_impersonated_exec_syscall();

    return 0;
}

#define DO_FORK_STRUCT_INPUT 1

int __attribute__((always_inline)) handle_do_fork(ctx_t *ctx) {
    struct syscall_cache_t syscall = {
        .type = EVENT_FORK,
        .fork.is_thread = 1,
    };

    u32 kthread_key = 0;
    u32 *is_kthread = bpf_map_lookup_elem(&is_new_kthread, &kthread_key);
    if (is_kthread) {
        syscall.fork.is_kthread = *is_kthread;
        *is_kthread = 0;
    }

    u64 input;
    LOAD_CONSTANT("do_fork_input", input);

    if (input == DO_FORK_STRUCT_INPUT) {
        void *args = (void *)CTX_PARM1(ctx);
        int exit_signal;
        bpf_probe_read(&exit_signal, sizeof(int), (void *)args + 32);

        if (exit_signal == SIGCHLD) {
            syscall.fork.is_thread = 0;
        }
    } else {
        u64 flags = (u64)CTX_PARM1(ctx);
        if ((flags & SIGCHLD) == SIGCHLD) {
            syscall.fork.is_thread = 0;
        }
    }

    cache_syscall(&syscall);

    return 0;
}

HOOK_ENTRY("kernel_thread")
int hook_kernel_thread(ctx_t *ctx) {
    u32 index = 0;
    u32 value = 1;
    bpf_map_update_elem(&is_new_kthread, &index, &value, BPF_ANY);
    return 0;
}

HOOK_ENTRY("user_mode_thread")
int hook_user_mode_thread(ctx_t *ctx) {
    u32 index = 0;
    u32 value = 1;
    bpf_map_update_elem(&is_new_kthread, &index, &value, BPF_ANY);
    return 0;
}

HOOK_ENTRY("kernel_clone")
int hook_kernel_clone(ctx_t *ctx) {
    return handle_do_fork(ctx);
}

HOOK_ENTRY("do_fork")
int hook_do_fork(ctx_t *ctx) {
    return handle_do_fork(ctx);
}

HOOK_ENTRY("_do_fork")
int hook__do_fork(ctx_t *ctx) {
    return handle_do_fork(ctx);
}

SEC("tracepoint/sched/sched_process_fork")
int sched_process_fork(struct _tracepoint_sched_process_fork *args) {
    // inherit netns
    u32 pid = 0;
    bpf_probe_read(&pid, sizeof(pid), &args->child_pid);

    // ignore the rest if kworker
    struct syscall_cache_t *syscall = peek_syscall(EVENT_FORK);
    if (!syscall || syscall->fork.is_kthread) {
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

    // if this is a thread, leave
    if (syscall->fork.is_thread) {
        pop_syscall(EVENT_FORK);
        return 0;
    }

    u64 ts = bpf_ktime_get_ns();
    struct process_event_t *event = new_process_event(1);
    if (event == NULL) {
        pop_syscall(EVENT_FORK);
        return 0;
    }

    event->pid_entry.fork_timestamp = ts;

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
        pop_syscall(EVENT_FORK);
        return 0;
    }

    struct pid_cache_t *parent_pid_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &ppid);
    if (parent_pid_entry) {
        // ensure pid and ppid point to the same cookie
        event->pid_entry.cookie = parent_pid_entry->cookie;

        // ensure pid and ppid have the same credentials
        event->pid_entry.credentials = parent_pid_entry->credentials;

        // fetch the parent proc cache entry
        u64 on_stack_cookie = event->pid_entry.cookie;
        struct proc_cache_t *parent_pc = get_proc_from_cookie(on_stack_cookie);
        if (parent_pc) {
            fill_container_context(parent_pc, &event->container);
            copy_proc_entry(&parent_pc->entry, &event->proc_entry);
        }
    }

    struct pid_cache_t on_stack_pid_entry = event->pid_entry;
    // insert the pid cache entry for the new process
    bpf_map_update_elem(&pid_cache, &pid, &on_stack_pid_entry, BPF_ANY);

    // [activity_dump] inherit tracing state
    inherit_traced_state(args, ppid, pid, event->container.container_id, event->proc_entry.comm);

    // send the entry to maintain userspace cache
    send_event_ptr(args, EVENT_FORK, event);

    pop_syscall(EVENT_FORK);

    return 0;
}

HOOK_ENTRY("do_coredump")
int hook_do_coredump(ctx_t *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    u8 in_coredump = 1;

    bpf_map_update_elem(&tasks_in_coredump, &key, &in_coredump, BPF_ANY);

    return 0;
}

HOOK_ENTRY("do_exit")
int hook_do_exit(ctx_t *ctx) {
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
        if (pc) {
            dec_mount_ref(ctx, pc->entry.executable.path_key.mount_id);
        }
        fill_container_context(pc, &event.container);
        fill_span_context(&event.span);
        event.exit_code = (u32)(u64)CTX_PARM1(ctx);
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

    // cleanup any remaining syscall cache entry for this pid_tgid
    pop_syscall(EVENT_ANY);

    return 0;
}

HOOK_ENTRY("exit_itimers")
int hook_exit_itimers(ctx_t *ctx) {
    void *signal = (void *)CTX_PARM1(ctx);

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

HOOK_ENTRY("prepare_binprm")
int hook_prepare_binprm(ctx_t *ctx) {
    return fill_exec_context();
}

HOOK_ENTRY("bprm_execve")
int hook_bprm_execve(ctx_t *ctx) {
    return fill_exec_context();
}

HOOK_ENTRY("security_bprm_check")
int hook_security_bprm_check(ctx_t *ctx) {
    return fill_exec_context();
}

TAIL_CALL_TARGET("get_envs_offset")
int tail_call_target_get_envs_offset(void *ctx) {
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

void __attribute__((always_inline)) parse_args_envs(void *ctx, struct args_envs_parsing_context_t *args_envs_ctx, struct args_envs_t *args_envs) {
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

TAIL_CALL_TARGET("parse_args_envs_split")
int tail_call_target_parse_args_envs_split(void *ctx) {
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

TAIL_CALL_TARGET("parse_args_envs")
int tail_call_target_parse_args_envs(void *ctx) {
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

int __attribute__((always_inline)) fetch_interpreter(void *ctx, struct linux_binprm *bprm) {
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
    bpf_printk("binprm_file_offset: %d", binprm_file_offset);

    bpf_printk("interpreter file: %llx", interpreter);

    const char *s;
    bpf_probe_read(&s, sizeof(s), &bprm->filename);
    bpf_printk("*filename from binprm: %s", s);

    bpf_probe_read(&s, sizeof(s), &bprm->interp);
    bpf_printk("*interp from binprm: %s", s);
#endif

    return handle_interpreted_exec_event(ctx, syscall, interpreter);
}

HOOK_ENTRY("setup_new_exec")
int hook_setup_new_exec_interp(ctx_t *ctx) {
    struct linux_binprm *bprm = (struct linux_binprm *) CTX_PARM1(ctx);
    return fetch_interpreter(ctx, bprm);
}

HOOK_ENTRY("setup_new_exec")
int hook_setup_new_exec_args_envs(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_current_or_impersonated_exec_syscall();
    if (!syscall) {
        return 0;
    }

    void *bprm = (void *)CTX_PARM1(ctx);

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

HOOK_ENTRY("setup_arg_pages")
int hook_setup_arg_pages(ctx_t *ctx) {
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

int __attribute__((always_inline)) send_exec_event(ctx_t *ctx) {
    struct syscall_cache_t *syscall = pop_current_or_impersonated_exec_syscall();
    if (!syscall) {
        return 0;
    }

    // check if this is a thread first
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 now = bpf_ktime_get_ns();
    u32 tgid = pid_tgid >> 32;

    bpf_map_delete_elem(&exec_pid_transfer, &tgid);

    struct proc_cache_t pc = {
        .entry = {
            .executable = {
                .path_key = {
                    .ino = syscall->exec.file.path_key.ino,
                    .mount_id = syscall->exec.file.path_key.mount_id,
                    .path_id = syscall->exec.file.path_key.path_id,
                },
                .flags = syscall->exec.file.flags
            },
            .exec_timestamp = bpf_ktime_get_ns(),
        },
        .container = {},
    };
    fill_file(syscall->exec.dentry, &pc.entry.executable);
    bpf_get_current_comm(&pc.entry.comm, sizeof(pc.entry.comm));

    u64 parent_inode = 0;

    // select the previous cookie entry in cache of the current process
    // (this entry was created by the fork of the current process)
    struct pid_cache_t *fork_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
    if (fork_entry) {
        // Fetch the parent proc cache entry
        u64 parent_cookie = fork_entry->cookie;
        struct proc_cache_t *parent_pc = get_proc_from_cookie(parent_cookie);
        if (parent_pc) {
            parent_inode = parent_pc->entry.executable.path_key.ino;

            // inherit the parent container context
            fill_container_context(parent_pc, &pc.container);
            dec_mount_ref(ctx, parent_pc->entry.executable.path_key.mount_id);
        }
    }

    // Insert new proc cache entry (Note: do not move the order of this block with the previous one, we need to inherit
    // the container ID before saving the entry in proc_cache. Modifying entry after insertion won't work.)
    u64 cookie = rand64();
    bpf_map_update_elem(&proc_cache, &cookie, &pc, BPF_ANY);

    // update pid <-> cookie mapping
    if (fork_entry) {
        fork_entry->cookie = cookie;
    } else {
        struct pid_cache_t new_pid_entry = {
            .cookie = cookie,
        };
        bpf_map_update_elem(&pid_cache, &tgid, &new_pid_entry, BPF_ANY);
        fork_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
        if (fork_entry == NULL) {
            // should never happen, ignore
            return 0;
        }
    }

    struct process_event_t *event = new_process_event(0);
    if (event == NULL) {
        return 0;
    }

    // copy proc_cache data
    fill_container_context(&pc, &event->container);
    copy_proc_entry(&pc.entry, &event->proc_entry);

    // copy pid_cache entry data
    copy_pid_cache_except_exit_ts(fork_entry, &event->pid_entry);

    // add pid / tid context
    struct process_context_t *on_stack_process = &event->process;
    fill_process_context(on_stack_process);

    // override the pid context inode with the parent inode so that we can compare
    on_stack_process->inode = parent_inode;

    copy_span_context(&syscall->exec.span_context, &event->span);
    fill_args_envs(event, syscall);

    // [activity_dump] check if this process should be traced
    should_trace_new_process(ctx, now, tgid, event->container.container_id, event->proc_entry.comm);

    // add interpreter path info
    event->linux_binprm.interpreter = syscall->exec.linux_binprm.interpreter;

    // send the entry to maintain userspace cache
    send_event_ptr(ctx, EVENT_EXEC, event);

    return 0;
}

HOOK_ENTRY("mprotect_fixup")
int hook_mprotect_fixup(ctx_t *ctx) {
    return send_exec_event(ctx);
}

#endif
