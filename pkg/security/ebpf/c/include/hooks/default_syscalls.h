#ifndef _HOOKS_DEFAULT_SYSCALLS_H
#define _HOOKS_DEFAULT_SYSCALLS_H

// is_default_syscall is provided by an architecture-specific header. Syscall
// IDs differ between amd64 and arm64, and arm64 does not expose the legacy
// "non-at" variants (open/stat/lstat/fork/vfork/readlink/getdents/...).
#if defined(__x86_64__)
#include "default_syscalls_amd64.h"
#elif defined(__aarch64__)
#include "default_syscalls_arm64.h"
#else
static __attribute__((always_inline)) int is_default_syscall(unsigned long id) {
    (void)id;
    return 0;
}
#endif

#endif
