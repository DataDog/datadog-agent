#ifndef __SYSCALL_LATENCY_KERN_USER_H
#define __SYSCALL_LATENCY_KERN_USER_H

#include "ktypes.h"

/*
 * Tracked syscall slot assignments.
 *
 * Syscalls are mapped to dense slots so the stats array stays small
 * (SYSCALL_SLOT_MAX entries per CPU rather than 512).  The mapping is
 * a compile-time switch in classify_syscall(), compiled to a jump table.
 */
typedef enum {
    SYSCALL_SLOT_READ         = 0,
    SYSCALL_SLOT_WRITE        = 1,
    SYSCALL_SLOT_PREAD64      = 2,
    SYSCALL_SLOT_PWRITE64     = 3,
    SYSCALL_SLOT_POLL         = 4,
    SYSCALL_SLOT_SELECT       = 5,
    SYSCALL_SLOT_MMAP         = 6,
    SYSCALL_SLOT_MUNMAP       = 7,
    SYSCALL_SLOT_CONNECT      = 8,
    SYSCALL_SLOT_ACCEPT       = 9,
    SYSCALL_SLOT_ACCEPT4      = 10,
    SYSCALL_SLOT_FUTEX        = 11,
    SYSCALL_SLOT_EPOLL_WAIT   = 12,
    SYSCALL_SLOT_EPOLL_PWAIT  = 13,
    SYSCALL_SLOT_CLONE        = 14,
    SYSCALL_SLOT_EXECVE       = 15,
    SYSCALL_SLOT_IO_URING     = 16,
    SYSCALL_SLOT_MAX          = 17,
    SYSCALL_NOT_TRACKED       = 0xFF,
} syscall_slot_t;

/*
 * Per-slot statistics stored in a per-CPU array map.
 * total_time_ns and count are monotonically increasing;
 * max_time_ns is reset to 0 after each flush interval.
 * slow_count counts calls whose duration exceeded SLOW_THRESHOLD_NS.
 */
typedef struct {
    __u64 total_time_ns;
    __u64 count;
    __u64 max_time_ns;
    __u64 slow_count;
} syscall_stats_t;

/* 1 ms — calls slower than this increment slow_count */
#define SLOW_THRESHOLD_NS 1000000ULL

/* Length of the cgroup leaf name buffer, matching other eBPF checks in this tree. */
#define CGROUP_NAME_LEN 128

/* Map key for per-container per-syscall stats.
 * Combines the cgroup leaf name with the syscall slot so a single hash map
 * replaces the per-CPU array, giving per-container granularity. */
typedef struct {
    char cgroup_name[CGROUP_NAME_LEN]; /* leaf cgroup name from get_cgroup_name() */
    __u8  slot;                         /* syscall_slot_t */
    __u8  pad[7];                       /* explicit padding for alignment */
} cgroup_stats_key_t;

#endif /* __SYSCALL_LATENCY_KERN_USER_H */
