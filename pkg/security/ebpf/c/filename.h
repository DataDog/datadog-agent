#ifndef _FILENAME_H_
#define _FILENAME_H_

#include "syscalls.h"

SEC("kprobe/filename_create")
int kprobe__filename_create(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    switch (syscall->type) {
        case EVENT_MKDIR:
            syscall->mkdir.path = (struct path *)PT_REGS_PARM3(ctx);
            break;
       case EVENT_LINK:
            syscall->link.target_path = (struct path *)PT_REGS_PARM3(ctx);
            break;
    }
    return 0;
}

#endif
