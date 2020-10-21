#ifndef __TRACER_BPF_H
#define __TRACER_BPF_H

#include <linux/types.h>

static const __u64 DISABLED = 0;
static const __u64 ENABLED = 1;

typedef struct {
    __u64 sent_bytes;
    __u64 recv_bytes;
    __u64 timestamp;
} conn_stats_ts_t;

// Metadata bit masks
// 0 << x is only for readability
typedef enum {
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
} tcp_conn_t;

typedef enum {
    HTTP_UNKNOWN = 0,
    HTTP_RESPONDING = 1 << 0,
    HTTP_REQUESTING_GET = 1 << 1,
    HTTP_REQUESTING_POST = 1 << 2,
    HTTP_REQUESTING_PUT = 1 << 3,
    HTTP_REQUESTING_DELETE = 1 << 4,
    HTTP_REQUESTING_HEAD = 1 << 5,
} http_state_t;

static const __u8 HTTP_REQUESTING = HTTP_REQUESTING_GET|HTTP_REQUESTING_POST|HTTP_REQUESTING_PUT|HTTP_REQUESTING_DELETE|HTTP_REQUESTING_HEAD;

// HTTP stats scoped to a certain response code (200x, 300x etc)
typedef struct {
    __u64 hits;
    __u64 total_duration;
} responses_by_code_t;

// HTTP stats summary associated to a certain socket (tuple_t)
typedef struct {
    // These fields track the current request/response
    __u8 state;
    __u64 request_started;
    __u16 response_code;
    __u64 response_last_seen;

    // These fields are responsible for storing an aggregated view of the previous transactions
    responses_by_code_t stats_200;
    responses_by_code_t stats_300;
    responses_by_code_t stats_400;
    responses_by_code_t stats_500;
} http_stats_t;

// Must match the number of tcp_conn_t objects embedded in the batch_t struct
#ifndef TCP_CLOSED_BATCH_SIZE
#define TCP_CLOSED_BATCH_SIZE 5
#endif

// This struct is meant to be used as a container for batching
// writes to the perf buffer. Ideally we should have an array of tcp_conn_t objects
// but apparently eBPF verifier doesn't allow arbitrary index access during runtime.
typedef struct {
    tcp_conn_t c0;
    tcp_conn_t c1;
    tcp_conn_t c2;
    tcp_conn_t c3;
    tcp_conn_t c4;
    __u16 pos;
    __u16 cpu;
} batch_t;

// Telemetry names
typedef struct {
    __u64 tcp_sent_miscounts;
    __u64 missed_tcp_close;
    __u64 udp_sends_processed;
    __u64 udp_sends_missed;
} telemetry_t;

#define PORT_LISTENING 1
#define PORT_CLOSED 0

typedef struct {
    __u16 port;
    __u64 fd;
} bind_syscall_args_t;

#endif
