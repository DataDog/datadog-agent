#ifndef __TRACER_BPF_H
#define __TRACER_BPF_H

#include "ktypes.h"

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
    protocol_t protocol;
} conn_stats_ts_t;

// Connection flags
typedef enum
{
    CONN_L_INIT = 1 << 0, // initial/first message sent
    CONN_R_INIT = 1 << 1, // reply received for initial message from remote
    CONN_ASSURED = 1 << 2 // "3-way handshake" complete, i.e. response to initial reply sent
} conn_flags_t;

// Metadata bit masks
// 0 << x is only for readability
typedef enum
{
    // Connection type
    CONN_TYPE_UDP = 0,
    CONN_TYPE_TCP = 1,

    // Connection family
    CONN_V4 = 0 << 1,
    CONN_V6 = 1 << 1,
} metadata_mask_t;

typedef struct {
    /* Using the type unsigned __int128 generates an error in the ebpf verifier */
    __u64 saddr_h;
    __u64 saddr_l;
    __u64 daddr_h;
    __u64 daddr_l;
    __u16 sport;
    __u16 dport;
    __u32 netns;
    __u32 pid;
    // Metadata description:
    // First bit indicates if the connection is TCP (1) or UDP (0)
    // Second bit indicates if the connection is V6 (1) or V4 (0)
    __u32 metadata; // This is that big because it seems that we atleast need a 32-bit aligned struct
} conn_tuple_t;

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

// From include/net/tcp.h
// tcp_flag_byte(th) (((u_int8_t *)th)[13])
#define TCP_FLAGS_OFFSET 13
#define TCPHDR_FIN 0x01
#define TCPHDR_RST 0x04
#define TCPHDR_ACK 0x10

// skb_info_t embeds a conn_tuple_t extracted from the skb object as well as
// some ancillary data such as the data offset (the byte offset pointing to
// where the application payload begins) and the TCP flags if applicable.
// This struct is populated by calling `read_conn_tuple_skb` from a program type
// that manipulates a `__sk_buff` object.
typedef struct {
    __u32 data_off;
    __u32 tcp_seq;
    __u8 tcp_flags;
} skb_info_t;

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
    __u64 missed_tcp_close;
    __u64 missed_udp_close;
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
    __u32 pid;
    __u32 fd;
} pid_fd_t;

typedef struct {
    struct sock *sk;
    size_t len;
    union {
        struct flowi4 *fl4;
        struct flowi6 *fl6;
    };
} ip_make_skb_args_t;

#endif
