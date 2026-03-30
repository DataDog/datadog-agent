#ifndef _SK_DEFS_H_
#define _SK_DEFS_H_

#include "tracer/tracer.h"

#ifndef NSEC_PER_USEC
#define NSEC_PER_USEC	1000L
#endif

#ifndef TCP_ECN_OK
#define TCP_ECN_OK	1
#endif

typedef struct {
    __u32 pid;
    __u16 state_transitions;
    __u16 failure_reason;
//    __u32 cookie;

    tcp_event_stats_t tcp_event_stats;

    __u64 start_ns;
    __u8 direction;
    conn_tuple_t tup;
} sk_tcp_stats_t;

typedef struct {
    __u64 sent_bytes;
    __u64 recv_bytes;
    __u32 sent_packets;
    __u32 recv_packets;
    __u32 pid;
//    __u32 cookie;
    __u64 start_ns;
    __u64 timestamp_ns;
    __u8 direction;
    conn_tuple_t tup;
} sk_udp_stats_t;

#endif
