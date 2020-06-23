#ifndef __TRACER_BPF_H
#define __TRACER_BPF_H

#include <linux/types.h>

static const __u8 GUESS_SADDR = 0;
static const __u8 GUESS_DADDR = 1;
static const __u8 GUESS_FAMILY = 2;
static const __u8 GUESS_SPORT = 3;
static const __u8 GUESS_DPORT = 4;
static const __u8 GUESS_NETNS = 5;
static const __u8 GUESS_RTT = 6;
static const __u8 GUESS_DADDR_IPV6 = 7;

#ifndef TASK_COMM_LEN
#define TASK_COMM_LEN 16
#endif

typedef struct {
    char comm[TASK_COMM_LEN];
} proc_t;

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
} tcp_stats_t;

// Full data for a tcp connection
typedef struct {
    conn_tuple_t tup;
    conn_stats_ts_t conn_stats;
    tcp_stats_t tcp_stats;
} tcp_conn_t;


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

static const __u8 TRACER_STATE_UNINITIALIZED = 0;
static const __u8 TRACER_STATE_CHECKING = 1;
static const __u8 TRACER_STATE_CHECKED = 2;
static const __u8 TRACER_STATE_READY = 3;

static const __u8 TRACER_IPV6_DISABLED = 0;
static const __u8 TRACER_IPV6_ENABLED = 1;

// Telemetry names
typedef struct {
    __u64 tcp_sent_miscounts;
    __u64 missed_tcp_close;
} telemetry_t;

typedef struct {
    __u64 state;

    /* checking */
    proc_t proc;
    __u64 what;
    __u64 offset_saddr;
    __u64 offset_daddr;
    __u64 offset_sport;
    __u64 offset_dport;
    __u64 offset_netns;
    __u64 offset_ino;
    __u64 offset_family;
    __u64 offset_rtt;
    __u64 offset_rtt_var;
    __u64 offset_daddr_ipv6;

    __u64 err;

    __u32 daddr_ipv6[4];
    __u32 netns;
    __u32 rtt;
    __u32 rtt_var;
    __u32 saddr;
    __u32 daddr;
    __u16 sport;
    __u16 dport;
    __u16 family;

    __u8 ipv6_enabled;
    __u8 padding;
} tracer_status_t;

#define PORT_LISTENING 1
#define PORT_CLOSED 0


typedef struct {
  __u16 port;
  __u64 fd;
} bind_syscall_args_t;

#endif
