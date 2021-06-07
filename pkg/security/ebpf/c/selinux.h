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

int __attribute__((always_inline)) trace__selinux(struct pt_regs *ctx, struct file *file) {
    struct selinux_event_t event = {
        .magic = 42,
    };

    struct dentry *dentry = get_file_dentry(file);

    fill_file_metadata(dentry, &event.file.metadata);
    set_file_inode(dentry, &event.file, 0);

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);

    send_event(ctx, EVENT_SELINUX, event);
    return 0;
}

#define PROBE_SEL_WRITE_FUNC(func_name)                        \
    SEC("kprobe/" #func_name)                                  \
    int kprobe__##func_name(struct pt_regs *ctx) {             \
        bpf_printk(#func_name " hit\n");                       \
        struct file *file = (struct file *)PT_REGS_PARM1(ctx); \
        return trace__selinux(ctx, file);                      \
    }

PROBE_SEL_WRITE_FUNC(sel_write_enforce)
PROBE_SEL_WRITE_FUNC(sel_write_bool)

#endif
