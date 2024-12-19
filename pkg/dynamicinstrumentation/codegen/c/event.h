#ifndef DI_EVENT_H
#define DI_EVENT_H

#include "ktypes.h"
#include "macros.h"

struct event {
    struct base_event base;
    char output[PARAM_BUFFER_SIZE];
};

// expression_context contains state that is meant to be shared across location expressions
// during execution of the full bpf program.
struct expression_context {
    __u64 *output_offset;
    __u8 *stack_counter;
    struct pt_regs *ctx;
    struct event *event;
    __u64 *temp_storage;
    char *zero_string;
};

#endif
