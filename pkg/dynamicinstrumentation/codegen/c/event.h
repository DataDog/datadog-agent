#ifndef DI_EVENT_H
#define DI_EVENT_H

#include "ktypes.h"
#include "macros.h"

// event is the message which is passed back to user space from bpf containing
// all information about the invocation of the instrumented function
typedef struct event {
    struct base_event base;
    char output[PARAM_BUFFER_SIZE]; // values of parameters
} event_t;

// expression_context contains state that is meant to be shared across location expressions
// during execution of the full bpf program.
typedef struct expression_context {
    __u64 output_offset; // current offset within the output buffer to write to
    __u8 stack_counter;  // current size of the bpf parameter stack, used for emptying stack
    struct pt_regs *ctx;
    event_t *event;  // output event allocated on ringbuffer
    __u64 *temp_storage;  // temporary storage array on heap used by some location expressions
    char *zero_string;    // array of zero's used to zero out buffers
    struct bpf_map* param_stack;
} expression_context_t;

#endif
