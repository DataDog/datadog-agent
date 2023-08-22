#ifndef _HOOKS_FILENAME_H_
#define _HOOKS_FILENAME_H_

#include "helpers/syscalls.h"
#include "constants/fentry_macro.h"

int __attribute__((always_inline)) filename_create_common(struct path *p) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_ANY);
    if (!syscall) {
        return 0;
    }

    switch (syscall->type) {
        case EVENT_MKDIR:
            syscall->mkdir.path = p;
            break;
       case EVENT_LINK:
            syscall->link.target_path = p;
            break;
    }
    return 0;
}

HOOK_ENTRY("filename_create")
int hook_filename_create(ctx_t *ctx) {
    struct path* p = (struct path*)CTX_PARM3(ctx);
    return filename_create_common(p);
}

HOOK_ENTRY("kern_path_create")
int hook_kern_path_create(ctx_t *ctx) {
    struct path* p = (struct path*)CTX_PARM3(ctx);
    return filename_create_common(p);
}

HOOK_ENTRY("user_path_create")
int hook_user_path_create(ctx_t *ctx) {
    struct path* p = (struct path*)CTX_PARM3(ctx);
    return filename_create_common(p);
}

#endif
