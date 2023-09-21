#ifndef _HOOKS_MODULE_H_
#define _HOOKS_MODULE_H_

#include "constants/syscall_macro.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"

int __attribute__((always_inline)) trace_init_module(u32 loaded_from_memory, const char *uargs) {
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

	int len = bpf_probe_read_user_str(&syscall.init_module.args, sizeof(syscall.init_module.args), uargs);
    if (len == sizeof(syscall.init_module.args)) {
        syscall.init_module.args_truncated = 1;
    }

    cache_syscall(&syscall);
    return 0;
}

HOOK_SYSCALL_ENTRY3(init_module, void *, umod, unsigned long, len, const char *, uargs) {
    return trace_init_module(1, uargs);
}

HOOK_SYSCALL_ENTRY3(finit_module, int, fd, const char *, uargs, int, flags) {
    return trace_init_module(0, uargs);
}

int __attribute__((always_inline)) trace_kernel_file(ctx_t *ctx, struct file *f, int dr_type) {
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
    syscall->resolver.callback = DR_NO_CALLBACK;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, dr_type);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_INIT_MODULE);

    return 0;
}

int __attribute__((always_inline)) fetch_mod_name_common(struct module *m) {
	struct syscall_cache_t *syscall = peek_syscall(EVENT_INIT_MODULE);
    if (!syscall) {
        return 0;
    }

    if (syscall->init_module.name[0] != 0) {
        return 0;
    }

    bpf_probe_read_str(&syscall->init_module.name, sizeof(syscall->init_module.name), &m->name);
    return 0;
}

HOOK_ENTRY("mod_sysfs_setup")
int hook_mod_sysfs_setup(ctx_t *ctx) {
    struct module *m = (struct module*)CTX_PARM1(ctx);
    return fetch_mod_name_common(m);
}

HOOK_ENTRY("module_param_sysfs_setup")
int hook_module_param_sysfs_setup(ctx_t *ctx) {
    struct module *m = (struct module*)CTX_PARM1(ctx);
    return fetch_mod_name_common(m);
}

HOOK_ENTRY("security_kernel_module_from_file")
int hook_security_kernel_module_from_file(ctx_t *ctx) {
    struct file *f = (struct file *)CTX_PARM1(ctx);
    return trace_kernel_file(ctx, f, DR_KPROBE_OR_FENTRY);
}

HOOK_ENTRY("security_kernel_read_file")
int hook_security_kernel_read_file(ctx_t *ctx) {
    struct file *f = (struct file *)CTX_PARM1(ctx);
    return trace_kernel_file(ctx, f, DR_KPROBE_OR_FENTRY);
}

int __attribute__((always_inline)) trace_init_module_ret(void *ctx, int retval, char *modname) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_INIT_MODULE);
    if (!syscall) {
        return 0;
    }

    struct init_module_event_t event = {
        .syscall.retval = retval,
        .file = syscall->init_module.file,
        .loaded_from_memory = syscall->init_module.loaded_from_memory,
    };

    bpf_probe_read_str(&event.args, sizeof(event.args), &syscall->init_module.args);
    event.args_truncated = syscall->init_module.args_truncated;

    if (!modname) {
        bpf_probe_read_str(&event.name, sizeof(event.name), &syscall->init_module.name[0]);
    } else {
        bpf_probe_read_str(&event.name, sizeof(event.name), modname);
    }

    if (syscall->init_module.dentry != NULL) {
        fill_file(syscall->init_module.dentry, &event.file);
    }

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_INIT_MODULE, event);
    return 0;
}

// only attached on rhel-7 based kernels
SEC("tracepoint/module/module_load")
int module_load(struct tracepoint_module_module_load_t *args) {
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

HOOK_SYSCALL_EXIT(init_module) {
    return trace_init_module_ret(ctx, (int)SYSCALL_PARMRET(ctx), NULL);
}

HOOK_SYSCALL_EXIT(finit_module) {
    return trace_init_module_ret(ctx, (int)SYSCALL_PARMRET(ctx), NULL);
}

HOOK_SYSCALL_ENTRY1(delete_module, const char *, name_user) {
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
    };
    bpf_probe_read_str(&event.name, sizeof(event.name), (void *)syscall->delete_module.name);

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_DELETE_MODULE, event);
    return 0;
}

HOOK_SYSCALL_EXIT(delete_module) {
    return trace_delete_module_ret(ctx, (int)SYSCALL_PARMRET(ctx));
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
