#ifndef _HOOKS_CGROUP_H_
#define _HOOKS_CGROUP_H_

#include "constants/custom.h"
#include "constants/offsets/filesystem.h"
#include "helpers/process.h"
#include "helpers/utils.h"
#include "hooks/dentry_resolver.h"
#include "structs/dentry_resolver.h"
#include "maps.h"

#define ROOT_CGROUP_PROCS_FILE_INO 2

static __attribute__((always_inline)) int trace__cgroup_write(ctx_t *ctx) {
    u32 cgroup_write_type = get_cgroup_write_type();
    u32 pid;

    switch (cgroup_write_type) {
    case CGROUP_DEFAULT: {
        char *pid_buff = (char *)CTX_PARM2(ctx);
        pid = atoi(pid_buff);
        break;
    }
    case CGROUP_CENTOS_7: {
        pid = (u32)CTX_PARM3(ctx);
        break;
    }
    default:
        // ignore
        return 0;
    }

    // from cgroups(7):
    // Writing the value 0 to a cgroup.procs file causes the writing process to be moved to the corresponding cgroup.
    // in this case we want to grab the tgid of the process that wrote to the file
    if (pid == 0) {
        pid = bpf_get_current_pid_tgid() >> 32;
    }

#ifdef DEBUG_CGROUP
    bpf_printk("trace__cgroup_write %d\n", pid);
#endif

    struct proc_cache_t new_entry = {};
    struct proc_cache_t *old_entry;
    u8 new_cookie = 0;
    u64 cookie = 0;

    // Retrieve the cgroup mount id to filter on
    u32 cgroup_mount_id_filter = get_cgroup_mount_id_filter();
    if (cgroup_mount_id_filter == CGROUP_MOUNT_ID_UNSET) {
        // ignore cgroups write event until the filter has been set
        return 0;
    }

    // Retrieve the cookie of the process
    struct pid_cache_t *pid_entry = (struct pid_cache_t *)bpf_map_lookup_elem(&pid_cache, &pid);
    if (pid_entry) {
        cookie = pid_entry->cookie;
        // Select the old cache entry
        old_entry = get_proc_from_cookie(cookie);
        if (old_entry) {
            // copy cache data
            copy_proc_cache(old_entry, &new_entry);
        }
    } else {
        new_cookie = 1;
        cookie = rand64();
    }

    struct dentry *container_d;
#ifdef DEBUG_CGROUP
    struct qstr container_qstr;
    char *container_id;
#endif

    struct dentry_resolver_input_t cgroup_dentry_resolver = {0};
    struct dentry_resolver_input_t *resolver = &cgroup_dentry_resolver;

    resolver->key.ino = 0;
    resolver->key.mount_id = 0;
    resolver->key.path_id = 0;
    resolver->dentry = NULL;

    switch (cgroup_write_type) {
    case CGROUP_DEFAULT: {
        // Retrieve the container ID from the cgroup path.
        struct kernfs_open_file *kern_f = (struct kernfs_open_file *)CTX_PARM1(ctx);
        struct file *f;
        bpf_probe_read(&f, sizeof(f), &kern_f->file);
        struct dentry *dentry = get_file_dentry(f);

        // The last dentry in the cgroup path should be `cgroup.procs`, thus the container ID should be its parent.
        bpf_probe_read(&container_d, sizeof(container_d), &dentry->d_parent);
#ifdef DEBUG_CGROUP
        bpf_probe_read(&container_qstr, sizeof(container_qstr), &container_d->d_name);
        container_id = (void *)container_qstr.name;
#endif

        resolver->key.ino = get_dentry_ino(container_d);
        resolver->key.mount_id = get_file_mount_id(f);
        resolver->dentry = container_d;
        break;
    }
    case CGROUP_CENTOS_7: {
        void *cgroup = (void *)CTX_PARM1(ctx);
        bpf_probe_read(&container_d, sizeof(container_d), cgroup + 72); // offsetof(struct cgroup, dentry)

#ifdef DEBUG_CGROUP
        bpf_probe_read(&container_qstr, sizeof(container_qstr), &container_d->d_name);
        container_id = (void *)container_qstr.name;
#endif

        u64 inode = get_dentry_ino(container_d);
        resolver->key.ino = inode;
        resolver->dentry = container_d;
        break;
    }
    default:
        // ignore
        return 0;
    }

    // if the process is being moved to the root cgroup then we don't want to track it
    if (resolver->key.ino == ROOT_CGROUP_PROCS_FILE_INO) {
        return 0;
    }

    if (!is_cgroup_mount_id_filter_valid(cgroup_mount_id_filter, &resolver->key)) {
        return 0;
    }

    new_entry.cgroup.cgroup_file = resolver->key;

#ifdef DEBUG_CGROUP
    bpf_printk("container id: %s\n", container_qstr.name);
#endif

    bpf_map_update_elem(&proc_cache, &cookie, &new_entry, BPF_ANY);

    if (new_cookie) {
        struct pid_cache_t new_pid_entry = {
            .cookie = cookie,
        };
        bpf_map_update_elem(&pid_cache, &pid, &new_pid_entry, BPF_ANY);
    }

    resolver->type = EVENT_CGROUP_WRITE;
    resolver->discarder_event_type = 0;
    resolver->callback = DR_CGROUP_WRITE_CALLBACK_KPROBE_KEY;
    resolver->iteration = 0;
    resolver->ret = 0;
    resolver->flags = 0;
    resolver->cgroup_write_ctx.cgroup_write_pid = pid;
    resolver->original_key = resolver->key;

    cache_dentry_resolver_input(resolver);

    resolve_dentry_no_syscall(ctx, KPROBE_OR_FENTRY_TYPE);

    return 0;
}

int __attribute__((always_inline)) dr_cgroup_write_callback(void *ctx) {
    struct dentry_resolver_input_t *inputs = peek_resolver_inputs(EVENT_ANY);
    if (!inputs)
        return 0;

    struct cgroup_write_event_t event = {
        .path_key = inputs->original_key,
        .pid = inputs->cgroup_write_ctx.cgroup_write_pid,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_cgroup_context(entry, &event.cgroup);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_CGROUP_WRITE, event);

    return 0;
}

TAIL_CALL_FNC(dr_cgroup_write_callback, ctx_t *ctx) {
    return dr_cgroup_write_callback(ctx);
}

HOOK_ENTRY("cgroup_procs_write")
int hook_cgroup_procs_write(ctx_t *ctx) {
    return trace__cgroup_write(ctx);
}

HOOK_ENTRY("cgroup1_procs_write")
int hook_cgroup1_procs_write(ctx_t *ctx) {
    return trace__cgroup_write(ctx);
}

HOOK_ENTRY("cgroup_tasks_write")
int hook_cgroup_tasks_write(ctx_t *ctx) {
    return trace__cgroup_write(ctx);
}

HOOK_ENTRY("cgroup1_tasks_write")
int hook_cgroup1_tasks_write(ctx_t *ctx) {
    return trace__cgroup_write(ctx);
}

static __attribute__((always_inline)) void cache_file(struct dentry *dentry, u32 mount_id);

static __attribute__((always_inline)) int trace__cgroup_open(ctx_t *ctx) {
    u32 cgroup_write_type = get_cgroup_write_type();
    struct file *file;

    switch (cgroup_write_type) {
    case CGROUP_CENTOS_7: {
        file = (struct file *)CTX_PARM2(ctx);
        break;
    }
    default:
        // ignore
        return 0;
    }

    struct dentry *dentry = get_file_dentry(file);
    u32 mount_id = get_file_mount_id(file);

    cache_file(dentry, mount_id);

    struct dentry *d_parent;
    bpf_probe_read(&d_parent, sizeof(d_parent), &dentry->d_parent);
    cache_file(d_parent, mount_id);

    return 0;
}

HOOK_ENTRY("cgroup_procs_open")
int hook_cgroup_procs_open(ctx_t *ctx) {
    return trace__cgroup_open(ctx);
}

HOOK_ENTRY("cgroup_tasks_open")
int hook_cgroup_tasks_open(ctx_t *ctx) {
    return trace__cgroup_open(ctx);
}

#endif
