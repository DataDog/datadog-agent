#ifndef __CONNECTION_CLOSE_H
#define __CONNECTION_CLOSE_H

#include "conn_tuple.h"
#include "protocols/classification/defs.h"

typedef struct {
    conn_tuple_t tuple;
    __u64 timestamp_ns;
    protocol_stack_t stack;
} tcp_close_event_t;

#endif // __CONNECTION_CLOSE_H
