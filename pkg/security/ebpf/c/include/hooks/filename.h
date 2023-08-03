#ifndef _HOOKS_FILENAME_H_
#define _HOOKS_FILENAME_H_

#include "helpers/syscalls.h"
#include "constants/fentry_macro.h"

HOOK_ENTRY("filename_create")
int hook_filename_create(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_ANY);
    if (!syscall) {
        return 0;
    }

    switch (syscall->type) {
        case EVENT_MKDIR:
            syscall->mkdir.path = (struct path *)CTX_PARM3(ctx);
            break;
       case EVENT_LINK:
            syscall->link.target_path = (struct path *)CTX_PARM3(ctx);
            break;
    }
    return 0;
}

#endif
