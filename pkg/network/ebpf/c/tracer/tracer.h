#ifndef __TRACER_TRACER_H
#define __TRACER_TRACER_H

#include "ktypes.h"

#include "conn_tuple.h"
#include "protocols/classification/defs.h"

#define bool _Bool
#define true 1
#define false 0

// TCP Failures
#define TCP_CONN_FAILED_RESET 104
#define TCP_CONN_FAILED_TIMEOUT 110
#define TCP_CONN_FAILED_REFUSED 111
#define TCP_CONN_FAILED_EHOSTUNREACH 113
#define TCP_CONN_FAILED_ENETUNREACH 101
// this isn't really a failure from the kernel, this happens when userspace closes the socket during SYN_SENT
#define TCP_CONN_FAILED_CANCELED 125

typedef enum {
    CONN_DIRECTION_UNKNOWN = 0b00,
    CONN_DIRECTION_INCOMING = 0b01,
    CONN_DIRECTION_OUTGOING = 0b10,
} conn_direction_t;

typedef enum {
    PACKET_COUNT_NONE = 0,
    PACKET_COUNT_ABSOLUTE = 1,
    PACKET_COUNT_INCREMENT = 2,
} packet_count_increment_t;

#define CONN_DIRECTION_MASK 0b11

typedef struct {
    __u16 chosen_version;
    __u16 cipher_suite;
    __u8  offered_versions;
} tls_info_t;

typedef struct {
    __u64 updated;
    tls_info_t info;
} tls_info_wrapper_t;

// 48-bit milliseconds timestamp
typedef struct {
    __u16 timestamp[3];
} time_ms_t;

typedef struct {
    __u64 sent_bytes;
    __u64 recv_bytes;
    __u32 sent_packets;
    __u32 recv_packets;
    time_ms_t timestamp_ms;
    // duration of the connection.
    // this is initialized to the current unix
    // timestamp when a conn_stats_ts_t is created.
    // the field remains unchanged until this object
    // is removed from the conn_stats map when it
    // is updated with (CURRENT_TIME - duration)
    time_ms_t duration_ms;
    // "cookie" that uniquely identifies
    // a conn_stas_ts_t. This is used
    // in user space to distinguish between
    // stats for two or more connections that
    // may share the same conn_tuple_t (this can
    // happen when we're aggregating connections).
    // This is not the same as a TCP cookie or
    // the cookie in struct sock in the kernel
    __u32 cookie;
    protocol_stack_t protocol_stack;
    __u8 flags;
    __u8 direction;
    tls_info_t tls_tags;
    __u32 cert_id;
} conn_stats_ts_t;

// Connection flags
typedef enum {
    CONN_L_INIT = 1 << 0, // initial/first message sent
    CONN_R_INIT = 1 << 1, // reply received for initial message from remote
    CONN_ASSURED = 1 << 2 // "3-way handshake" complete, i.e. response to initial reply sent
} conn_flags_t;

// Per-connection TCP event counters. Keyed by zero-PID conn_tuple_t (like tcp_retransmits)
// because tcp_enter_loss, tcp_enter_recovery, and tcp_send_probe0 all fire from kernel
// timer/softirq context without a reliable userspace PID.
// Stored in a separate BPF map for active-connection polling; also embedded
// in tcp_stats_t so closed connections carry the final values.
typedef struct {
    __u32 rto_count;
    __u32 recovery_count;
    __u32 probe0_count;
} tcp_event_stats_t;

typedef struct {
    __u32 rtt;
    __u32 rtt_var;
    __u32 retransmits;

    // Bit mask containing all TCP state transitions tracked by our tracer
    __u16 state_transitions;
    __u16 failure_reason;

    // Discrete TCP event counters (RTO, fast recovery, zero-window probes)
    tcp_event_stats_t tcp_event_stats;

    // TCP congestion stats (from tcp_sock reads in sendmsg/recvmsg, CO-RE/runtime only)
    __u32 reord_seen;
    __u32 rcv_ooopack;
    __u32 delivered_ce;
    __u8 ecn_negotiated;
    __u8 _pad[3]; // explicit padding to maintain 4-byte alignment

    // Explicit padding to make tcp_stats_t a multiple of 8 bytes (48 total).
    // Without this, conn_stats_ts_t (which starts with uint64 sent_bytes)
    // would be at offset 92 in conn_t — not 8-byte aligned — causing
    // different struct layouts on x86_64 vs ARM64.
    __u32 _pad2;
} tcp_stats_t;

// Full data for a tcp connection
typedef struct {
    conn_tuple_t tup;
    // move tcp_stats here to align conn_stats on a cacheline boundary
    tcp_stats_t tcp_stats;
    conn_stats_ts_t conn_stats;
} conn_t;

// Telemetry names
typedef struct {
    __u64 tcp_sent_miscounts;
    __u64 udp_sends_processed;
    __u64 udp_sends_missed;
    __u64 udp_dropped_conns;
    __u64 tcp_done_missing_pid;
    __u64 tcp_connect_failed_tuple;
    __u64 tcp_done_failed_tuple;
    __u64 tcp_finish_connect_failed_tuple;
    __u64 tcp_close_target_failures;
    __u64 tcp_done_connection_flush;
    __u64 tcp_close_connection_flush;
    __u64 tcp_syn_retransmit;
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

typedef struct {
    struct sock *sk;
    conn_tuple_t tup;
} skp_conn_tuple_t;

typedef struct {
    __u64 pid_tgid;
    __u64 timestamp;
} pid_ts_t;

#endif
