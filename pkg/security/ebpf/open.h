#ifndef _OPEN_H_
#define _OPEN_H_

#include "syscalls.h"
#include "process.h"

struct open_event_t {
    struct   event_t event;
    struct   process_data_t process;
    int      mode;
    int      flags;
    unsigned long inode;
    int      mount_id;
    u32      padding;
};

SEC("kprobe/__x64_sys_openat")
int kprobe__sys_openat(struct pt_regs *ctx) {
    if (filter_process())
        return 0;

    struct syscall_cache_t syscall = {
        .open = {
            .flags = (int) PT_REGS_PARM3(ctx),
            .mode = (umode_t) PT_REGS_PARM4(ctx)
        }
    };

    cache_syscall(&syscall);

    return 0;
}

SEC("kprobe/vfs_open")
int kprobe__vfs_open(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

#ifdef DEBUG
    struct dentry *dentry;
    bpf_probe_read(&dentry, sizeof(dentry), &path->dentry);

    struct qstr qstr;
    bpf_probe_read(&qstr, sizeof(qstr), &dentry->d_name);

    printk("kprobe/vfs_open %s\n", qstr.name);
#endif

    syscall->open.path = (struct path *)PT_REGS_PARM1(ctx);
    syscall->open.file = (struct file *)PT_REGS_PARM2(ctx);

    return 0;
}

SEC("kretprobe/__x64_sys_openat")
int kretprobe__sys_openat(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    struct inode *f_inode = get_file_inode(syscall->open.file);
    struct dentry *f_dentry = get_file_dentry(syscall->open.file);

    struct open_event_t event = {
        .event.retval = PT_REGS_RC(ctx),
        .event.type = EVENT_MAY_OPEN,
        .event.timestamp = bpf_ktime_get_ns(),
        .mount_id = get_inode_mount_id(f_inode),
        .inode = get_inode_ino(f_inode),
        .flags = syscall->open.flags,
        .mode = syscall->open.mode,
    };

    fill_process_data(&event.process);
    resolve_dentry(f_dentry, event.inode);

    send_event(ctx, event);

#ifdef DEBUG
    if (event.process.comm[0] == 'c' && event.process.comm[1] == 'a') {
        printk("trace__sys_openat_ret %p %p %d\n", file, f_dentry, event.mount_id);
    }
#endif

    return 0;
}

#endif