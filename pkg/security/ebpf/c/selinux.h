#ifndef _SELINUX_H_
#define _SELINUX_H_

#include "defs.h"
#include "filters.h"
#include "syscalls.h"
#include "process.h"

struct selinux_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct file_t file;
    u32 magic;
};

SEC("kprobe/sel_write_enforce")
int kprobe__sel_write_enforce(struct pt_regs *ctx) {
    bpf_printk("sel_write_enforce hit\n");

    struct selinux_event_t event = {
        .magic = 42,
    };

    struct file *file = (struct file *)PT_REGS_PARM1(ctx);
    struct dentry *dentry = get_file_dentry(file);

    fill_file_metadata(dentry, &event.file.metadata);
    set_file_inode(dentry, &event.file, 0);

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);

    send_event(ctx, EVENT_SELINUX, event);
    return 0;
}

#endif
