#ifndef _SELINUX_H_
#define _SELINUX_H_

#include "defs.h"
#include "filters.h"
#include "syscalls.h"
#include "process.h"


SEC("kprobe/sel_write_enforce")
int kprobe__sel_write_enforce(struct pt_regs *ctx) {
    bpf_printk("sel_write_enforce hit\n");
    return 0;
}

#endif
