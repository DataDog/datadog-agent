#ifndef _CGROUP_H_
#define _CGROUP_H_

static __attribute__((always_inline)) int trace__cgroup_write(struct pt_regs *ctx) {
    char *pid_buff = (char *) PT_REGS_PARM2(ctx);
    u32 pid = atoi(pid_buff);
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

    // Retrieve the container ID from the cgroup path.
    struct kernfs_open_file *kern_f = (struct kernfs_open_file *) PT_REGS_PARM1(ctx);
    struct file *f;
    bpf_probe_read(&f, sizeof(f), &kern_f->file);
    struct dentry *dentry = get_file_dentry(f);

    // The last dentry in the cgroup path should be `cgroup.procs`, thus the container ID should be its parent.
    struct dentry *container_d;
    struct qstr container_qstr;
    bpf_probe_read(&container_d, sizeof(container_d), &dentry->d_parent);
    bpf_probe_read(&container_qstr, sizeof(container_qstr), &container_d->d_name);
    bpf_probe_read(&new_entry.container.container_id, sizeof(new_entry.container.container_id), (void*) container_qstr.name);
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
