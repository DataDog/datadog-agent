#ifndef _MODULE_H_
#define _MODULE_H_

struct init_module_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    struct file_t file;
    char name[MODULE_NAME_LEN];
    u32 loaded_from_memory;
    u32 padding;
};

int __attribute__((always_inline)) trace_init_module(u32 loaded_from_memory) {
    struct policy_t policy = fetch_policy(EVENT_INIT_MODULE);
    if (is_discarded_by_process(policy.mode, EVENT_INIT_MODULE)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_INIT_MODULE,
        .init_module = {
            .loaded_from_memory = loaded_from_memory,
        },
    };

    cache_syscall(&syscall);
    return 0;
}

SYSCALL_KPROBE0(init_module) {
    return trace_init_module(1);
}

SYSCALL_KPROBE0(finit_module) {
    return trace_init_module(0);
}

int __attribute__((always_inline)) trace_kernel_file(struct pt_regs *ctx, struct file *f) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_INIT_MODULE);
    if (!syscall) {
        return 0;
    }

    syscall->init_module.dentry = get_file_dentry(f);
    set_file_inode(syscall->init_module.dentry, &syscall->init_module.file, 0);
    syscall->init_module.file.path_key.mount_id = get_file_mount_id(f);

    syscall->resolver.key = syscall->init_module.file.path_key;
    syscall->resolver.dentry = syscall->init_module.dentry;
    syscall->resolver.discarder_type = syscall->policy.mode != NO_FILTER ? EVENT_INIT_MODULE : 0;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE);
    return 0;
}

SEC("kprobe/security_kernel_module_from_file")
int kprobe_security_kernel_module_from_file(struct pt_regs *ctx) {
    struct file *f = (struct file *)PT_REGS_PARM1(ctx);
    return trace_kernel_file(ctx, f);
}

SEC("kprobe/security_kernel_read_file")
int kprobe_security_kernel_read_file(struct pt_regs *ctx) {
    struct file *f = (struct file *)PT_REGS_PARM1(ctx);
    return trace_kernel_file(ctx, f);
}

int __attribute__((always_inline)) trace_module(struct module *mod) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_INIT_MODULE);
    if (!syscall) {
        return 0;
    }

    bpf_probe_read_str(&syscall->init_module.name, sizeof(syscall->init_module.name), &mod->name[0]);
    return 0;
}

SEC("kprobe/do_init_module")
int kprobe_do_init_module(struct pt_regs *ctx) {
    struct module *mod = (struct module *)PT_REGS_PARM1(ctx);
    return trace_module(mod);
}

SEC("kprobe/module_put")
int kprobe_module_put(struct pt_regs *ctx) {
    struct module *mod = (struct module *)PT_REGS_PARM1(ctx);
    return trace_module(mod);
}

int __attribute__((always_inline)) trace_init_module_ret(void *ctx, int retval, char *modname) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_INIT_MODULE);
    if (!syscall) {
        return 0;
    }

    struct init_module_event_t event = {
        .syscall.retval = retval,
        .event.async = 0,
        .file = syscall->init_module.file,
        .loaded_from_memory = syscall->init_module.loaded_from_memory,
    };

    if (!modname) {
        bpf_probe_read_str(&event.name, sizeof(event.name), &syscall->init_module.name[0]);
    } else {
        bpf_probe_read_str(&event.name, sizeof(event.name), modname);
    }

    if (syscall->init_module.dentry != NULL) {
        fill_file_metadata(syscall->init_module.dentry, &event.file.metadata);
    }

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_INIT_MODULE, event);
    return 0;
}

struct tracepoint_module_module_load_t {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    unsigned int taints;
    int data_loc_modname;
};

SEC("tracepoint/module/module_load")
int module_load(struct tracepoint_module_module_load_t *args) {
    u64 tracepoint_module_load_sends_event;
    LOAD_CONSTANT("tracepoint_module_load_sends_event", tracepoint_module_load_sends_event);
    if (tracepoint_module_load_sends_event) {
        // check if the tracepoint is hit by a kworker
        u32 pid = bpf_get_current_pid_tgid();
        u32 *is_kworker = bpf_map_lookup_elem(&pid_ignored, &pid);
        if (!is_kworker) {
            return 0;
        }

        struct syscall_cache_t *syscall = peek_syscall(EVENT_INIT_MODULE);
        if (!syscall) {
            return 0;
        }

        unsigned short modname_offset = args->data_loc_modname & 0xFFFF;
        char *modname = (char *)args + modname_offset;

        return trace_init_module_ret(args, 0, modname);
    }

    return 0;
}

SYSCALL_KRETPROBE(init_module) {
    return trace_init_module_ret(ctx, (int)PT_REGS_RC(ctx), NULL);
}

SYSCALL_KRETPROBE(finit_module) {
    return trace_init_module_ret(ctx, (int)PT_REGS_RC(ctx), NULL);
}

struct delete_module_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    char name[MODULE_NAME_LEN];
};

SYSCALL_KPROBE1(delete_module, const char *, name_user) {
    struct policy_t policy = fetch_policy(EVENT_DELETE_MODULE);
    if (is_discarded_by_process(policy.mode, EVENT_DELETE_MODULE)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_DELETE_MODULE,
        .delete_module = {
            .name = name_user,
        },
    };

    cache_syscall(&syscall);
    return 0;
}

int __attribute__((always_inline)) trace_delete_module_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_DELETE_MODULE);
    if (!syscall) {
        return 0;
    }

    struct delete_module_event_t event = {
        .syscall.retval = retval,
        .event.async = 0,
    };
    bpf_probe_read_str(&event.name, sizeof(event.name), (void *)syscall->delete_module.name);

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_DELETE_MODULE, event);
    return 0;
}

SYSCALL_KRETPROBE(delete_module) {
    return trace_delete_module_ret(ctx, (int)PT_REGS_RC(ctx));
}

SEC("tracepoint/handle_sys_init_module_exit")
int tracepoint_handle_sys_init_module_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return trace_init_module_ret(args, args->ret, NULL);
}

SEC("tracepoint/handle_sys_delete_module_exit")
int tracepoint_handle_sys_delete_module_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return trace_delete_module_ret(args, args->ret);
}

#endif
