#ifndef __SECCOMP_TRACER_KERN_USER_H
#define __SECCOMP_TRACER_KERN_USER_H

#include "ktypes.h"

#define CGROUP_NAME_MAX_LEN 128

// Event structure sent to userspace via ring buffer
typedef struct {
    char cgroup[CGROUP_NAME_MAX_LEN];
    __u32 syscall_nr;
    __u32 action; // SECCOMP_RET_KILL=0x80000000/0x00000000, ERRNO=0x00050000, TRAP=0x00030000
    __s32 stack_id; // Stack trace ID from BPF_MAP_TYPE_STACK_TRACE map, -1 if not captured
    __u32 pid;      // Process ID (TGID)
    __u32 tid;      // Thread ID
    char comm[16];  // Command name (TASK_COMM_LEN)
} seccomp_event_t;

#endif
