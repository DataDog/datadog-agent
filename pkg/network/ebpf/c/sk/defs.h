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
    conn_tuple_t tup;

    __u64 initial_sent_bytes;
    __u64 initial_recv_bytes;
    __u32 initial_sent_packets;
    __u32 initial_recv_packets;
    __u32 initial_retransmits;
    __u16 state_transitions;
    __u16 failure_reason;

    tcp_event_stats_t tcp_event_stats;
    __u64 start_ns;
    __u8 direction;
} sk_tcp_stats_t;

typedef struct {
    conn_tuple_t tup;

    __u64 sent_bytes;
    __u64 recv_bytes;
    __u32 sent_packets;
    __u32 recv_packets;
    __u64 start_ns;
    __u64 timestamp_ns;
    __u8 flags;
    __u8 direction;
} sk_udp_stats_t;

#endif
