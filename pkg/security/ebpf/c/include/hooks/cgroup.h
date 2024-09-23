#ifndef _HOOKS_CGROUP_H_
#define _HOOKS_CGROUP_H_

#include "constants/custom.h"
#include "constants/offsets/filesystem.h"
#include "helpers/process.h"
#include "helpers/utils.h"
#include "hooks/dentry_resolver.h"
#include "structs/dentry_resolver.h"
#include "maps.h"

static __attribute__((always_inline)) int is_docker_cgroup(ctx_t *ctx, struct dentry *container_d) {
    struct dentry *parent_d;
    struct qstr parent_qstr;
    char prefix[6];

    // We may not have a prefix for the cgroup so we look at the parent folder
    // (for instance Amazon Linux 2 + Docker)
    bpf_probe_read(&parent_d, sizeof(parent_d), &container_d->d_parent);
    if (parent_d != NULL) {
        bpf_probe_read(&parent_qstr, sizeof(parent_qstr), &parent_d->d_name);
        bpf_probe_read(&prefix, sizeof(prefix), parent_qstr.name);
        if (prefix[0] == 'd' && prefix[1] == 'o' && prefix[2] == 'c' && prefix[3] == 'k' && prefix[4] == 'e' && prefix[5] == 'r') {
            return 1;
        }
    }

    return 0;
}

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

#ifdef DEBUG_CGROUP
    bpf_printk("trace__cgroup_write %d\n", pid);
#endif

    struct proc_cache_t new_entry = {};
    struct proc_cache_t *old_entry;
    u8 new_cookie = 0;
    u64 cookie = 0;

    // Retrieve the cookie of the process
    struct pid_cache_t *pid_entry = (struct pid_cache_t *)bpf_map_lookup_elem(&pid_cache, &pid);
    if (pid_entry) {
        cookie = pid_entry->cookie;
        // Select the old cache entry
        old_entry = get_proc_from_cookie(cookie);
        if (old_entry) {
            if (old_entry->container.container_id[0] != '\0') {
                return 0;
            }

            // copy cache data
            copy_proc_cache(old_entry, &new_entry);
        }
    } else {
        new_cookie = 1;
        cookie = rand64();
    }

    struct dentry *container_d;
    struct qstr container_qstr;
    char *container_id;
    u32 container_flags = 0;

    struct dentry_resolver_input_t cgroup_dentry_resolver;
    struct dentry_resolver_input_t *resolver = &cgroup_dentry_resolver;

    u32 key = 0;
    cgroup_prefix_t *prefix = bpf_map_lookup_elem(&cgroup_prefix, &key);
    if (prefix == NULL)
        return 0;

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

        resolver->key.ino = get_dentry_ino(dentry);
        resolver->key.mount_id = get_file_mount_id(f);
        resolver->dentry = dentry;

        // The last dentry in the cgroup path should be `cgroup.procs`, thus the container ID should be its parent.
        bpf_probe_read(&container_d, sizeof(container_d), &dentry->d_parent);
        bpf_probe_read(&container_qstr, sizeof(container_qstr), &container_d->d_name);
        container_id = (void *)container_qstr.name;

        if (is_docker_cgroup(ctx, container_d)) {
            container_flags = CGROUP_MANAGER_DOCKER;
        }

        break;
    }
    case CGROUP_CENTOS_7: {
        void *cgroup = (void *)CTX_PARM1(ctx);
        bpf_probe_read(&container_d, sizeof(container_d), cgroup + 72); // offsetof(struct cgroup, dentry)
        bpf_probe_read(&container_qstr, sizeof(container_qstr), &container_d->d_name);
        container_id = (void *)container_qstr.name;

        u64 inode = get_dentry_ino(container_d);
        resolver->key.ino = inode;
        struct file_t *entry = bpf_map_lookup_elem(&exec_file_cache, &inode);
        if (entry == NULL) {
            return 0;
        }
        else {
            resolver->key.mount_id = entry->path_key.mount_id;
        }

        resolver->dentry = container_d;

        if (is_docker_cgroup(ctx, container_d)) {
            container_flags = CGROUP_MANAGER_DOCKER;
        }

        break;
    }
    default:
        // ignore
        return 0;
    }


    if (bpf_probe_read(prefix, 15, container_id))
        return 0;

    if ((*prefix)[0] == 'd' && (*prefix)[1] == 'o' && (*prefix)[2] == 'c' && (*prefix)[3] == 'k' && (*prefix)[4] == 'e'
        && (*prefix)[5] == 'r' && (*prefix)[6] == '-') {
        container_id += 7; // skip "docker-"
        container_flags = CGROUP_MANAGER_DOCKER;
    }
    else if ((*prefix)[0] == 'c' && (*prefix)[1] == 'r' && (*prefix)[2] == 'i' && (*prefix)[3] == 'o' && (*prefix)[4] == '-') {
        container_id += 5; // skip "crio-"
        container_flags = CGROUP_MANAGER_CRIO;
    }
    else if ((*prefix)[0] == 'l' && (*prefix)[1] == 'i' && (*prefix)[2] == 'b' && (*prefix)[3] == 'p' && (*prefix)[4] == 'o'
        && (*prefix)[5] == 'd' && (*prefix)[6] == '-') {
        container_id += 7; // skip "libpod-"
        container_flags = CGROUP_MANAGER_PODMAN;
    }
    else if ((*prefix)[0] == 'c' && (*prefix)[1] == 'r' && (*prefix)[2] == 'i' && (*prefix)[3] == '-' && (*prefix)[4] == 'c'
        && (*prefix)[5] == 'o' && (*prefix)[6] == 'n' && (*prefix)[7] == 't' && (*prefix)[8] == 'a' && (*prefix)[9] == 'i'
        && (*prefix)[10] == 'n' && (*prefix)[11] == 'e' && (*prefix)[12] == 'r' && (*prefix)[13] == 'd' && (*prefix)[14] == '-') {
        container_id += 15; // skip "cri-containerd-"
        container_flags = CGROUP_MANAGER_CRI;
    }

#ifdef DEBUG_CGROUP
    bpf_printk("container id: %s\n", container_qstr.name);
#endif

    int length = bpf_probe_read_str(prefix, sizeof(cgroup_prefix_t), container_id) & 0xff;
    if (container_flags == 0 && (
        (length >= 9 && (*prefix)[length-9] == '.'  && (*prefix)[length-8] == 's' && (*prefix)[length-7] == 'e' && (*prefix)[length-6] == 'r' && (*prefix)[length-5] == 'v' && (*prefix)[length-4] == 'i' && (*prefix)[length-3] == 'c' && (*prefix)[length-2] == 'e')
        ||
        (length >= 7 && (*prefix)[length-7] == '.'  && (*prefix)[length-6] == 's' && (*prefix)[length-5] == 'c' && (*prefix)[length-4] == 'o' && (*prefix)[length-3] == 'p' && (*prefix)[length-2] == 'e')
    )) {
        container_flags = CGROUP_MANAGER_SYSTEMD;
    }
    bpf_probe_read(&new_entry.container.container_id, sizeof(new_entry.container.container_id), container_id);

    new_entry.container.cgroup_context.cgroup_flags = container_flags;
    new_entry.container.cgroup_context.cgroup_file = resolver->key;

#ifdef DEBUG_CGROUP
    bpf_printk("container flags=%d, inode=%d: prefix=%s\n", container_flags, new_entry.container.cgroup_context.cgroup_file.ino, prefix);
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
    resolver->sysretval = 0;
    resolver->original_key = resolver->key;

    cache_dentry_resolver_input(resolver);

    resolve_dentry_no_syscall(ctx, DR_KPROBE_OR_FENTRY);

    return 0;
}

int __attribute__((always_inline)) dr_cgroup_write_callback(void *ctx) {
    struct dentry_resolver_input_t *inputs = peek_resolver_inputs(EVENT_ANY);
    if (!inputs)
        return 0;

    struct cgroup_write_event_t event = {
        .file.path_key = inputs->original_key,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_CGROUP_WRITE, event);

    return 0;
}

TAIL_CALL_TARGET("dr_cgroup_write_callback")
int tail_call_target_dr_cgroup_write_callback(ctx_t *ctx) {
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
