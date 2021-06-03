#ifndef _WRITE_H_
#define _WRITE_H_

#include "defs.h"
#include "filters.h"
#include "syscalls.h"
#include "process.h"

#include <linux/fs.h>
#include <linux/module.h>

/*
SEC("kprobe/vfs_write")
int kprobe__vfs_write(struct pt_regs *ctx) {
    struct file file;
    bpf_probe_read(&file, sizeof(struct file), (void*)PT_REGS_PARM1(ctx));

    struct file_operations file_op;
    bpf_probe_read(&file_op, sizeof(struct file_operations), (void*)file.f_op);

    struct module owner;
    bpf_probe_read(&owner, sizeof(struct module), (void*)file_op.owner);

    bpf_printk("module owner name: %s", owner.name);

    return 0;
}
*/

SEC("kprobe/vfs_write")
int kprobe__vfs_write(struct pt_regs *ctx) {
    struct file *file = (struct file*)PT_REGS_PARM1(ctx);
    char* name = file->f_op->owner->name;

    bpf_printk("module owner name: %s", name);

    return 0;
}

#endif
