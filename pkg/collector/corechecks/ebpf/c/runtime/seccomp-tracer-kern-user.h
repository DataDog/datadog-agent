#ifndef __SECCOMP_TRACER_KERN_USER_H
#define __SECCOMP_TRACER_KERN_USER_H

#include "ktypes.h"

#define CGROUP_NAME_MAX_LEN 128

// Event structure sent to userspace via ring buffer
typedef struct {
    char cgroup[CGROUP_NAME_MAX_LEN];
    __u32 syscall_nr;
    __u32 action; // SECCOMP_RET_KILL=0x80000000/0x00000000, ERRNO=0x00050000, TRAP=0x00030000
} seccomp_event_t;

#endif
