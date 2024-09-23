#ifndef DI_TYPES_H
#define DI_TYPES_H

#include "ktypes.h"

// NOTE: Be careful when adding fields, alignment should always be to 8 bytes
struct base_event {
    char probe_id[304];
    __u32 pid;
    __u32 uid;
    __u64 program_counters[10];
}__attribute__((aligned(8)));

#endif
