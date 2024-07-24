#ifndef _HOOKS_CGROUP_H_
#define _HOOKS_CGROUP_H_

#include "constants/custom.h"
#include "constants/offsets/filesystem.h"
#include "helpers/process.h"
#include "helpers/utils.h"
#include "maps.h"

#define CGROUP_MANAGER_DOCKER 1
#define CGROUP_MANAGER_CRIO 2
#define CGROUP_MANAGER_PODMAN 3
#define CGROUP_MANAGER_CRI 4
#define CGROUP_MANAGER_SYSTEMD 5

static __attribute__((always_inline)) int is_docker_cgroup(ctx_t *ctx, struct dentry *container_d) {
    struct dentry *parent_d;
    struct qstr parent_qstr;
    char prefix[15];

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

#ifdef DEBUG
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

    int check_validity = 0;
    u32 container_flags = 0;
    char prefix[15];

    switch (cgroup_write_type) {
    case CGROUP_DEFAULT: {
        // Retrieve the container ID from the cgroup path.
        struct kernfs_open_file *kern_f = (struct kernfs_open_file *)CTX_PARM1(ctx);
        struct file *f;
        bpf_probe_read(&f, sizeof(f), &kern_f->file);
        struct dentry *dentry = get_file_dentry(f);

        // The last dentry in the cgroup path should be `cgroup.procs`, thus the container ID should be its parent.
        bpf_probe_read(&container_d, sizeof(container_d), &dentry->d_parent);
        bpf_probe_read(&container_qstr, sizeof(container_qstr), &container_d->d_name);
        container_id = (void *)container_qstr.name;

        if (is_docker_cgroup(ctx, container_d)) {
            container_flags |= CGROUP_MANAGER_DOCKER;
            check_validity = 1;
        }

        break;
    }
    case CGROUP_CENTOS_7: {
        void *cgroup = (void *)CTX_PARM1(ctx);
        bpf_probe_read(&container_d, sizeof(container_d), cgroup + 72); // offsetof(struct cgroup, dentry)
        bpf_probe_read(&container_qstr, sizeof(container_qstr), &container_d->d_name);
        container_id = (void *)container_qstr.name;

        if (is_docker_cgroup(ctx, container_d)) {
            container_flags |= CGROUP_MANAGER_DOCKER;
            check_validity = 1;
        }

        break;
    }
    default:
        // ignore
        return 0;
    }

    bpf_probe_read(&prefix, sizeof(prefix), container_id);

    if (prefix[0] == 'd' && prefix[1] == 'o' && prefix[2] == 'c' && prefix[3] == 'k' && prefix[4] == 'e'
        && prefix[5] == 'r' && prefix[6] == '-') {
        container_id += 7; // skip "docker-"
        container_flags |= CGROUP_MANAGER_DOCKER;
        check_validity = 1;
    }
    else if (prefix[0] == 'c' && prefix[1] == 'r' && prefix[2] == 'i' && prefix[3] == 'o' && prefix[4] == '-') {
        container_id += 5; // skip "crio-"
        container_flags |= CGROUP_MANAGER_CRIO;
        check_validity = 1;
    }
    else if (prefix[0] == 'l' && prefix[1] == 'i' && prefix[2] == 'b' && prefix[3] == 'p' && prefix[4] == 'o'
        && prefix[5] == 'd' && prefix[6] == '-') {
        container_id += 7; // skip "libpod-"
        container_flags |= CGROUP_MANAGER_PODMAN;
        check_validity = 1;
    }
    else if (prefix[0] == 'c' && prefix[1] == 'r' && prefix[2] == 'i' && prefix[3] == '-' && prefix[4] == 'c'
        && prefix[5] == 'o' && prefix[6] == 'n' && prefix[7] == 't' && prefix[8] == 'a' && prefix[9] == 'i'
        && prefix[10] == 'n' && prefix[11] == 'e' && prefix[12] == 'r' && prefix[13] == 'd' && prefix[14] == '-') {
        container_id += 15; // skip "cri-containerd-"
        container_flags |= CGROUP_MANAGER_CRI;
        check_validity = 1;
    }

#ifdef DEBUG
    bpf_printk("container id: %s\n", container_qstr.name);
#endif

    bpf_probe_read(&new_entry.container.container_id, sizeof(new_entry.container.container_id), container_id);
    new_entry.container.cgroup_context.cgroup_flags = container_flags;

    if (check_validity && !is_container_id_valid(new_entry.container.container_id)) {
        return 0;
    }

#ifdef DEBUG
    bpf_printk("container flags: %d: %s\n", container_flags, prefix);
#endif

    bpf_map_update_elem(&proc_cache, &cookie, &new_entry, BPF_ANY);

    if (new_cookie) {
        struct pid_cache_t new_pid_entry = {
            .cookie = cookie,
        };
        bpf_map_update_elem(&pid_cache, &pid, &new_pid_entry, BPF_ANY);
    }

    return 0;
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

#endif
