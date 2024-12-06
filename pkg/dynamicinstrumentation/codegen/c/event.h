#ifndef DI_EVENT_H
#define DI_EVENT_H

#include "ktypes.h"

struct event {
    struct base_event base;
    char output[PARAM_BUFFER_SIZE];
};

struct expression_context {
    __u64 *output_offset;
    struct pt_regs *ctx;
    struct event *event;
    __u64 *temp_storage;
    char *zero_string;
};

#endif
