#ifndef DI_BASE_EVENT_H
#define DI_BASE_EVENT_H

#include "ktypes.h"

// standard fields which all events created in bpf will contain, regardless of the function that the
// probe is instrumenting
struct base_event {
    char probe_id[36]; // identifier for each user-configured instrumentation point, it's a standard 36 character UUID
    __u32 pid; // process ID
    __u32 uid; // user ID
    __u64 program_counters[10]; // program counters representing the stack trace of the instrumented function invocation
}__attribute__((aligned(8)));

#endif
