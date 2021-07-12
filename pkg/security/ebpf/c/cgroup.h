#ifndef _CGROUP_H_
#define _CGROUP_H_

#define CGROUP_DEFAULT  1
#define CGROUP_CENTOS_7 2

u32 __attribute__((always_inline)) get_cgroup_write_type(void) {
    u64 type;
    LOAD_CONSTANT("cgroup_write_type", type);
    return type;
}

static __attribute__((always_inline)) int trace__cgroup_write(struct pt_regs *ctx) {
    u32 cgroup_write_type = get_cgroup_write_type();
    u32 pid;

    switch (cgroup_write_type) {
        case CGROUP_DEFAULT: {
            char *pid_buff = (char *) PT_REGS_PARM2(ctx);
            pid = atoi(pid_buff);
            break;
        }
        case CGROUP_CENTOS_7: {
            pid = (u32) PT_REGS_PARM3(ctx);
            break;
        }
        default:
            // ignore
            return 0;
    }

    struct proc_cache_t new_entry = {};
    struct proc_cache_t *old_entry;
    u8 new_cookie = 0;
    u32 cookie = 0;

    // Retrieve the cookie of the process
    struct pid_cache_t *pid_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &pid);
    if (pid_entry) {
        cookie = pid_entry->cookie;
        // Select the old cache entry
        old_entry = bpf_map_lookup_elem(&proc_cache, &cookie);
        if (old_entry) {
            // copy cache data
            copy_proc_cache(old_entry, &new_entry);
        }
    } else {
        new_cookie = 1;
        cookie = bpf_get_prandom_u32();
    }

    struct dentry *container_d;
    struct qstr container_qstr;
    char *container_id;

    switch (cgroup_write_type) {
        case CGROUP_DEFAULT: {
            // Retrieve the container ID from the cgroup path.
            struct kernfs_open_file *kern_f = (struct kernfs_open_file *) PT_REGS_PARM1(ctx);
            struct file *f;
            bpf_probe_read(&f, sizeof(f), &kern_f->file);
            struct dentry *dentry = get_file_dentry(f);

            // The last dentry in the cgroup path should be `cgroup.procs`, thus the container ID should be its parent.
            bpf_probe_read(&container_d, sizeof(container_d), &dentry->d_parent);
            bpf_probe_read(&container_qstr, sizeof(container_qstr), &dentry->d_name);
            bpf_probe_read(&container_qstr, sizeof(container_qstr), &container_d->d_name);
            container_id = (void*) container_qstr.name;
            break;
        }
        case CGROUP_CENTOS_7: {
            void *cgroup = (void *) PT_REGS_PARM1(ctx);
            bpf_probe_read(&container_d, sizeof(container_d), cgroup + 72); // offsetof(struct cgroup, dentry)
            bpf_probe_read(&container_qstr, sizeof(container_qstr), &container_d->d_name);
            container_id = (void*) container_qstr.name;
            char prefix[4];
            bpf_probe_read(&prefix, sizeof(prefix), container_id);
            if (prefix[0] == 'd' && prefix[1] == 'o' && prefix[2] == 'c' && prefix[3] == 'k') {
                container_id += 7; // skip "docker-"
            }
            break;
        }
        default:
            // ignore
            return 0;
    }

    bpf_probe_read(&new_entry.container.container_id, sizeof(new_entry.container.container_id), container_id);
    bpf_map_update_elem(&proc_cache, &cookie, &new_entry, BPF_ANY);

    if (new_cookie) {
        struct pid_cache_t new_pid_entry = {
            .cookie = cookie,
        };
        bpf_map_update_elem(&pid_cache, &pid, &new_pid_entry, BPF_ANY);
    }
    return 0;
}

SEC("kprobe/cgroup_procs_write")
int kprobe__cgroup_procs_write(struct pt_regs *ctx) {
    return trace__cgroup_write(ctx);
}

SEC("kprobe/cgroup1_procs_write")
int kprobe__cgroup1_procs_write(struct pt_regs *ctx) {
    return trace__cgroup_write(ctx);
}

SEC("kprobe/cgroup_tasks_write")
int kprobe__cgroup_tasks_write(struct pt_regs *ctx) {
    return trace__cgroup_write(ctx);
}

SEC("kprobe/cgroup1_tasks_write")
int kprobe__cgroup1_tasks_write(struct pt_regs *ctx) {
    return trace__cgroup_write(ctx);
}

#endif
