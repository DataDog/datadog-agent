#ifndef DI_BASE_EVENT_H
#define DI_BASE_EVENT_H

#include "ktypes.h"

// standard fields which all events created in bpf will contain, regardless of the function that the
// probe is instrumenting
struct base_event {
    __u32 pid; // process ID
    __u32 uid; // user ID
    __u64 program_counters[10]; // program counters representing the stack trace of the instrumented function invocation
    __u64 param_indicies[20]; // indicies of where each parameter starts in argument buffer
    char probe_id[36]; // identifier for each user-configured instrumentation point, it's a standard 36 character UUID
};

#endif
