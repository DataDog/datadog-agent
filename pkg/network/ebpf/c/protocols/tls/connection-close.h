#ifndef __CONNECTION_CLOSE_H
#define __CONNECTION_CLOSE_H

#include "conn_tuple.h"
#include "protocols/classification/defs.h"

// Connection close event with full protocol stack
typedef struct {
    conn_tuple_t tuple;          // Connection identification (IPs, ports, PID, netns)
    __u64 timestamp_ns;          // When it closed
    protocol_stack_t stack;      // Full protocol stack (API, Application, Encryption layers)
} __attribute__((packed)) tcp_close_event_t;

#endif // __CONNECTION_CLOSE_H
