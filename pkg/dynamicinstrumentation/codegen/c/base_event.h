#ifndef DI_BASE_EVENT_H
#define DI_BASE_EVENT_H

#include "ktypes.h"

struct base_event {
    char probe_id[304];
    __u32 pid;
    __u32 uid;
    __u64 program_counters[10];
}__attribute__((aligned(8)));

#endif
