#ifndef __JVM_MEMORY_KERN_USER_H
#define __JVM_MEMORY_KERN_USER_H

#include "ktypes.h"

// Maximum length of a memory pool name, including the terminating NUL byte.
// The longest HotSpot pool name is "CodeHeap 'non-profiled nmethods'" (32 bytes).
#define JVM_POOL_NAME_LEN 64

// Key of the jvm_pools map: one entry per memory pool of each JVM.
// No padding: 4 + 64 bytes, 4-byte aligned. The pool name buffer is zeroed
// before the name is copied, so the key is deterministic.
typedef struct {
    __u32 tgid;
    __u8 name[JVM_POOL_NAME_LEN];
} jvm_pool_key_t;

// Value of the jvm_pools map: the state of a memory pool, as reported by the
// last hotspot:mem__pool__gc__end probe. init and max are HotSpot's SIZE_MAX
// when undefined.
typedef struct {
    __u64 init;
    __u64 used;
    __u64 committed;
    __u64 max;
    __u64 timestamp; // bpf_ktime_get_ns() of the last update
} jvm_pool_value_t;

// Value of the jvm_gc map: the cumulative stop-the-world pause counters of a JVM.
// A hotspot:gc__begin / hotspot:gc__end pair brackets one pause, not one
// collection: concurrent collectors fire several pairs per cycle.
typedef struct {
    __u64 pause_start_ns;    // bpf_ktime_get_ns() of the in-flight pause start, 0 when none
    __u64 pauses;            // completed pauses
    __u64 full_pauses;       // completed pauses flagged as full collections by gc__begin
    __u64 pause_time_ns;     // cumulative duration of the completed pauses
    __u64 last_pause_end_ns; // bpf_ktime_get_ns() of the last completed pause end, 0 when none
    __u8 pause_full;         // full collection flag of the in-flight pause
    __u8 pad[7];
} jvm_gc_stats_t;

#endif
