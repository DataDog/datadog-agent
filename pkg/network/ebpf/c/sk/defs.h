#ifndef _SK_DEFS_H_
#define _SK_DEFS_H_

#include "tracer/tracer.h"

#ifndef NSEC_PER_USEC
#define NSEC_PER_USEC	1000L
#endif

#ifndef TCP_ECN_OK
#define TCP_ECN_OK	1
#endif

#ifndef NSEC_PER_SEC
#define NSEC_PER_SEC	1000000000L
#endif

typedef struct {
    __u64 sent_bytes;
    __u64 recv_bytes;
    __u32 sent_packets;
    __u32 recv_packets;
    __u32 retransmits;
    __u32 reord_seen;
    __u32 rcv_ooopack;
    __u32 delivered_ce;
} sk_initial_tcp_stats_t;

typedef struct {
    conn_tuple_t tup;

    sk_initial_tcp_stats_t initial;
    __u16 state_transitions;
    __u16 failure_reason;

    tcp_event_stats_t tcp_event_stats;
    __u64 start_ns;
    __u8 direction;
} sk_tcp_stats_t;

typedef struct {
    conn_tuple_t tup;

    __u64 timestamp_ns;
    __u64 start_ns;
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
