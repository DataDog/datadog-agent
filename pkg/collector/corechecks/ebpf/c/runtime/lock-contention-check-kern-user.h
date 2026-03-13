#ifndef __LOCK_CONTENTION_CHECK_KERN_USER_H
#define __LOCK_CONTENTION_CHECK_KERN_USER_H

#include "ktypes.h"

// lock_type_key_t classifies kernel lock types derived from LCB_F_* flags
// in the contention_begin tracepoint.
typedef enum {
    LOCK_TYPE_SPINLOCK = 0,
    LOCK_TYPE_MUTEX = 1,
    LOCK_TYPE_RWSEM_READ = 2,
    LOCK_TYPE_RWSEM_WRITE = 3,
    LOCK_TYPE_RWLOCK_READ = 4,
    LOCK_TYPE_RWLOCK_WRITE = 5,
    LOCK_TYPE_RT_MUTEX = 6,
    LOCK_TYPE_PCPU_SPINLOCK = 7,
    LOCK_TYPE_OTHER = 8,
    LOCK_TYPE_MAX = 9,
} lock_type_key_t;

// lock_contention_stats_t holds aggregated contention statistics per lock type.
// Stored in a per-CPU array map indexed by lock_type_key_t.
typedef struct {
    __u64 total_time_ns;  // cumulative nanoseconds spent waiting
    __u64 count;          // number of contention events
    __u64 max_time_ns;    // max single-event wait time (reset per flush interval)
} lock_contention_stats_t;

#endif
