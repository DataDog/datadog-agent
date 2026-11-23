#ifndef _HOOKS_MODULE_H_
#define _HOOKS_MODULE_H_

#include "constants/syscall_macro.h"
#include "helpers/filesystem.h"
#include "helpers/span_fill.h"
#include "helpers/syscalls.h"

static __always_inline struct file *get_task_file_from_fd(int fd) {
    if (fd < 0) {
        return NULL;
    }

    u64 files_offset;
    LOAD_CONSTANT("task_struct_files_offset", files_offset);
    u64 fdt_offset;
    LOAD_CONSTANT("files_struct_fdt_offset", fdt_offset);
    u64 fd_offset;
    LOAD_CONSTANT("fdtable_fd_offset", fd_offset);
    if (!files_offset || !fdt_offset || !fd_offset) {
        return NULL;
    }

    void *ptr = (void *)bpf_get_current_task();
    if (!ptr) {
        return NULL;
    }

    bpf_probe_read(&ptr, sizeof(ptr), ptr + files_offset);
    if (!ptr) {
        return NULL;
    }

    bpf_probe_read(&ptr, sizeof(ptr), ptr + fdt_offset);
    if (!ptr) {
        return NULL;
    }

    u32 max_fds = 0;
    bpf_probe_read(&max_fds, sizeof(max_fds), ptr);
    u32 ufd = (u32)fd;
    if (ufd >= max_fds) {
        return NULL;
    }

    bpf_probe_read(&ptr, sizeof(ptr), ptr + fd_offset);
    if (!ptr) {
        return NULL;
    }

    bpf_probe_read(&ptr, sizeof(ptr), ptr + (u64)ufd * sizeof(ptr));
    return (struct file *)ptr;
}

static __always_inline int trace_init_module(void *ctx, u32 loaded_from_memory, const char *uargs, struct file *f) {
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

    if (f) {
        struct dentry *dentry = get_file_dentry(f);
        syscall.init_module.file.path_key.mount_id = get_file_mount_id(f);
        set_file_inode(dentry, &syscall.init_module.file, PATH_ID_INVALIDATE_TYPE_NONE);
        fill_file_metadata(dentry, &syscall.init_module.file);

        if (syscall.init_module.file.path_key.ino != 0 &&
            syscall.init_module.file.path_key.mount_id != 0 &&
            syscall.init_module.file.metadata.mode != 0) {
            syscall.init_module.dentry = dentry;
            syscall.resolver.key = syscall.init_module.file.path_key;
            syscall.resolver.dentry = dentry;
            syscall.resolver.iteration = 0;
            syscall.resolver.event_type = syscall.type;
            syscall.resolver.flags = get_resolver_flags(&syscall, 1);
            syscall.resolver.callback = DR_NO_CALLBACK;
            syscall.resolver.ret = 0;

            cache_syscall(ctx, &syscall);
            update_proc_cache_cgroup();

            resolve_dentry(ctx, KPROBE_OR_FENTRY_TYPE);
            return 0;
        }

        syscall.init_module.file.path_key.ino = 0;
        syscall.init_module.file.path_key.mount_id = 0;
        syscall.init_module.file.path_key.path_id = 0;
    }

    cache_syscall_update_cgroup(ctx, &syscall);
    return 0;
}

HOOK_SYSCALL_ENTRY3(init_module, void *, umod, unsigned long, len, const char *, uargs) {
    return trace_init_module(ctx, 1, uargs, NULL);
}

HOOK_SYSCALL_ENTRY2(finit_module, int, fd, const char *, uargs) {
    return trace_init_module(ctx, 0, uargs, get_task_file_from_fd(fd));
}

static __always_inline int trace_kernel_file(ctx_t *ctx, struct file *f, enum TAIL_CALL_PROG_TYPE prog_type) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_INIT_MODULE);
    if (!syscall || !f) {
        return 0;
    }

    struct dentry *dentry = get_file_dentry(f);
    int mount_id = get_file_mount_id(f);
    u64 ino = get_dentry_ino(dentry);
    if (!dentry || mount_id <= 0 || ino == 0) {
        return 0;
    }

    if (syscall->init_module.dentry == dentry &&
        syscall->init_module.file.path_key.mount_id == (u32)mount_id &&
        syscall->init_module.file.path_key.ino == ino &&
        syscall->resolver.ret > 0 &&
        syscall->resolver.key.ino == 0) {
        fill_file_metadata(dentry, &syscall->init_module.file);
        return 0;
    }

    syscall->init_module.dentry = dentry;
    syscall->init_module.file.path_key.mount_id = mount_id;
    syscall->init_module.file.path_key.ino = ino;
    syscall->init_module.file.flags = 0;
    syscall->init_module.file.metadata.nlink = 0;
    set_file_inode(dentry, &syscall->init_module.file, PATH_ID_INVALIDATE_TYPE_NONE);
    fill_file_metadata(dentry, &syscall->init_module.file);

    syscall->resolver.key = syscall->init_module.file.path_key;
    syscall->resolver.dentry = dentry;
    syscall->resolver.iteration = 0;
    syscall->resolver.event_type = syscall->type;
    syscall->resolver.flags = get_resolver_flags(syscall, 1);
    syscall->resolver.callback = DR_NO_CALLBACK;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, prog_type);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_INIT_MODULE);

    return 0;
}

static __always_inline int fetch_mod_name_common(struct module *m) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_INIT_MODULE);
    if (!syscall) {
        return 0;
    }

    if (syscall->init_module.name[0] != 0) {
        return 0;
    }

    u64 module_name_offset;
    LOAD_CONSTANT("module_name_offset", module_name_offset);
    bpf_probe_read_str(&syscall->init_module.name, sizeof(syscall->init_module.name), (void *)m + module_name_offset);
    return 0;
}

HOOK_ENTRY("mod_sysfs_setup")
int hook_mod_sysfs_setup(ctx_t *ctx) {
    struct module *m = (struct module *)CTX_PARM1(ctx);
    return fetch_mod_name_common(m);
}

HOOK_ENTRY("module_param_sysfs_setup")
int hook_module_param_sysfs_setup(ctx_t *ctx) {
    struct module *m = (struct module *)CTX_PARM1(ctx);
    return fetch_mod_name_common(m);
}

HOOK_ENTRY("security_kernel_module_from_file")
int hook_security_kernel_module_from_file(ctx_t *ctx) {
    struct file *f = (struct file *)CTX_PARM1(ctx);
    return trace_kernel_file(ctx, f, KPROBE_OR_FENTRY_TYPE);
}

HOOK_ENTRY("security_kernel_read_file")
int hook_security_kernel_read_file(ctx_t *ctx) {
    struct file *f = (struct file *)CTX_PARM1(ctx);
    return trace_kernel_file(ctx, f, KPROBE_OR_FENTRY_TYPE);
}

static __always_inline int trace_init_module_ret_impl(void *ctx, int retval, char *modname, enum TAIL_CALL_PROG_TYPE prog_type) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_INIT_MODULE);
    if (!syscall) {
        return 0;
    }

    struct init_module_event_t *event = SPAN_FILL_EVENT(struct init_module_event_t, EVENT_INIT_MODULE);
    if (!event) {
        goto pop_and_exit;
    }

    event->syscall.retval = retval;
    event->file = syscall->init_module.file;
    event->loaded_from_memory = syscall->init_module.loaded_from_memory;

    bpf_probe_read_str(&event->args, sizeof(event->args), &syscall->init_module.args);
    event->args_truncated = syscall->init_module.args_truncated;

    if (!modname) {
        bpf_probe_read_str(&event->name, sizeof(event->name), &syscall->init_module.name[0]);
    } else {
        bpf_probe_read_str(&event->name, sizeof(event->name), modname);
    }

    pop_syscall(EVENT_INIT_MODULE);

    struct proc_cache_t *entry = fill_process_context(&event->process);
    fill_cgroup_context(entry, &event->cgroup);

    span_fill_tail_call(ctx, prog_type);

pop_and_exit:
    pop_syscall(EVENT_INIT_MODULE);
    return 0;
}

static __always_inline int trace_init_module_ret(void *ctx, int retval, char *modname) {
    return trace_init_module_ret_impl(ctx, retval, modname, KPROBE_OR_FENTRY_TYPE);
}

// only attached on rhel-7 based kernels
SEC("tracepoint/module/module_load")
int module_load(struct tracepoint_module_module_load_t *args) {
    // check if the tracepoint is hit by a kworker
    u32 pid = bpf_get_current_pid_tgid();
    if (!IS_KERNEL_THREAD(pid)) {
        return 0;
    }

    struct syscall_cache_t *syscall = peek_syscall(EVENT_INIT_MODULE);
    if (!syscall) {
        return 0;
    }

    unsigned short modname_offset = args->data_loc_modname & 0xFFFF;
    char *modname = (char *)args + modname_offset;

    // module_load runs in tracepoint context.
    return trace_init_module_ret_impl(args, 0, modname, TRACEPOINT_TYPE);
}

HOOK_SYSCALL_EXIT(init_module) {
    return trace_init_module_ret(ctx, (int)SYSCALL_PARMRET(ctx), NULL);
}

HOOK_SYSCALL_EXIT(finit_module) {
    return trace_init_module_ret(ctx, (int)SYSCALL_PARMRET(ctx), NULL);
}

HOOK_SYSCALL_ENTRY1(delete_module, const char *, name_user) {
    struct syscall_cache_t syscall = {
        .type = EVENT_DELETE_MODULE,
        .delete_module = {
            .name = name_user,
        },
    };

    cache_syscall_update_cgroup(ctx, &syscall);
    return 0;
}

static __always_inline int trace_delete_module_ret_impl(void *ctx, int retval, enum TAIL_CALL_PROG_TYPE prog_type) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_DELETE_MODULE);
    if (!syscall) {
        return 0;
    }

    struct delete_module_event_t *event = SPAN_FILL_EVENT(struct delete_module_event_t, EVENT_DELETE_MODULE);
    if (!event) {
        goto pop_and_exit;
    }
    event->syscall.retval = retval;
    bpf_probe_read_str(&event->name, sizeof(event->name), (void *)syscall->delete_module.name);

    pop_syscall(EVENT_DELETE_MODULE);

    struct proc_cache_t *entry = fill_process_context(&event->process);
    fill_cgroup_context(entry, &event->cgroup);

    span_fill_tail_call(ctx, prog_type);

pop_and_exit:
    pop_syscall(EVENT_DELETE_MODULE);
    return 0;
}

static __always_inline int trace_delete_module_ret(void *ctx, int retval) {
    return trace_delete_module_ret_impl(ctx, retval, KPROBE_OR_FENTRY_TYPE);
}

HOOK_SYSCALL_EXIT(delete_module) {
    return trace_delete_module_ret(ctx, (int)SYSCALL_PARMRET(ctx));
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_init_module_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return trace_init_module_ret_impl(args, args->ret, NULL, TRACEPOINT_TYPE);
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_delete_module_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return trace_delete_module_ret_impl(args, args->ret, TRACEPOINT_TYPE);
}

#endif
