#ifndef __TRACER_TRACER_H
#define __TRACER_TRACER_H

#include "ktypes.h"

#include "conn_tuple.h"
#include "protocols/classification/defs.h"

#define bool _Bool
#define true 1
#define false 0

typedef enum
{
    CONN_DIRECTION_UNKNOWN = 0b00,
    CONN_DIRECTION_INCOMING = 0b01,
    CONN_DIRECTION_OUTGOING = 0b10,
} conn_direction_t;

typedef enum
{
    PACKET_COUNT_NONE = 0,
    PACKET_COUNT_ABSOLUTE = 1,
    PACKET_COUNT_INCREMENT = 2,
} packet_count_increment_t;

#define CONN_DIRECTION_MASK 0b11

typedef struct {
    __u64 sent_bytes;
    __u64 recv_bytes;
    __u64 timestamp;
    __u32 flags;
    // "cookie" that uniquely identifies
    // a conn_stas_ts_t. This is used
    // in user space to distinguish between
    // stats for two or more connections that
    // may share the same conn_tuple_t (this can
    // happen when we're aggregating connections).
    // This is not the same as a TCP cookie or
    // the cookie in struct sock in the kernel
    __u32 cookie;
    __u64 sent_packets;
    __u64 recv_packets;
    __u8 direction;
    protocol_stack_t protocol_stack;
} conn_stats_ts_t;

// Connection flags
typedef enum
{
    CONN_L_INIT = 1 << 0, // initial/first message sent
    CONN_R_INIT = 1 << 1, // reply received for initial message from remote
    CONN_ASSURED = 1 << 2 // "3-way handshake" complete, i.e. response to initial reply sent
} conn_flags_t;

typedef struct {
    __u32 retransmits;
    __u32 rtt;
    __u32 rtt_var;

    // Bit mask containing all TCP state transitions tracked by our tracer
    __u16 state_transitions;
} tcp_stats_t;

// Full data for a tcp connection
typedef struct {
    conn_tuple_t tup;
    conn_stats_ts_t conn_stats;
    tcp_stats_t tcp_stats;
} conn_t;

// Must match the number of conn_t objects embedded in the batch_t struct
#ifndef CONN_CLOSED_BATCH_SIZE
#define CONN_CLOSED_BATCH_SIZE 4
#endif

// This struct is meant to be used as a container for batching
// writes to the perf buffer. Ideally we should have an array of tcp_conn_t objects
// but apparently eBPF verifier doesn't allow arbitrary index access during runtime.
typedef struct {
    conn_t c0;
    conn_t c1;
    conn_t c2;
    conn_t c3;
    __u16 len;
    __u64 id;
} batch_t;

// Telemetry names
typedef struct {
    __u64 tcp_failed_connect;
    __u64 tcp_sent_miscounts;
    __u64 unbatched_tcp_close;
    __u64 unbatched_udp_close;
    __u64 udp_sends_processed;
    __u64 udp_sends_missed;
    __u64 udp_dropped_conns;
} telemetry_t;

typedef struct {
    struct sockaddr *addr;
    struct sock *sk;
} bind_syscall_args_t;

typedef struct {
    struct sock *sk;
    int segs;
    __u32 retrans_out_pre;
} tcp_retransmit_skb_args_t;

typedef struct {
    __u32 netns;
    __u16 port;
} port_binding_t;

typedef struct {
    struct sock *sk;
    struct msghdr *msg;
} udp_recv_sock_t;

typedef struct {
    struct sock *sk;
    size_t len;
    union {
        struct flowi4 *fl4;
        struct flowi6 *fl6;
    };
} ip_make_skb_args_t;

#endif
