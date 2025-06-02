#ifndef DI_BASE_EVENT_H
#define DI_BASE_EVENT_H

#include "ktypes.h"
#include "macros.h"

// standard fields which all events created in bpf will contain, regardless of the function that the
// probe is instrumenting
struct base_event {
    __u32 pid; // process ID
    __u32 uid; // user ID
    __u64 program_counters[STACK_DEPTH_LIMIT]; // program counters representing the stack trace of the instrumented function invocation
    __u64 param_indicies[MAX_FIELD_AND_PARAM_COUNT]; // indicies of where each parameter starts in argument buffer
    char probe_id[36]; // identifier for each user-configured instrumentation point, it's a standard 36 character UUID
};

#endif
