#ifndef _WRITE_H_
#define _WRITE_H_

#include "defs.h"
#include "filters.h"
#include "syscalls.h"
#include "process.h"

#include <linux/fs.h>
#include <linux/module.h>

struct module_start {
	enum module_state state;
	struct list_head list;
	char name[MODULE_NAME_LEN];
};

struct file_operations_start {
    struct module *owner;
};

size_t __attribute__((always_inline)) strlen_mod_name(const char *s) {
    for (size_t i = 0; i < MODULE_NAME_LEN; ++i) {
        if (s[i] == '\0') {
            return i;
        }
    }

    return -1;
}

SEC("kprobe/vfs_write")
int kprobe__vfs_write(struct pt_regs *ctx) {
    struct file file;
    bpf_probe_read(&file, sizeof(struct file), (void*)PT_REGS_PARM1(ctx));

    if (file.f_op == NULL) {
        return 0;
    }

    struct file_operations_start file_op;
    bpf_probe_read(&file_op, sizeof(struct file_operations_start), (void*)file.f_op);

    if (file_op.owner == NULL) {
        return 0;
    }

    struct module_start owner;
    bpf_probe_read(&owner, sizeof(struct module_start), (void*)file_op.owner);

    // if (strlen_mod_name(owner.name) > 0) {
        bpf_printk("module owner name: %s\n", owner.name);
    // }

    return 0;
}

/*
SEC("kprobe/vfs_write")
int kprobe__vfs_write(struct pt_regs *ctx) {
    struct file *file = (struct file*)PT_REGS_PARM1(ctx);
    char* name = file->f_op->owner->name;

    bpf_printk("module owner name: %s", name);

    return 0;
}
*/

#endif
