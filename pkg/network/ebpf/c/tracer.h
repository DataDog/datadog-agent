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

// From include/net/tcp.h
// tcp_flag_byte(th) (((u_int8_t *)th)[13])
#define TCP_FLAGS_OFFSET 13
#define TCPHDR_FIN 0x01

// skb_info_t embeds a conn_tuple_t extracted from the skb object as well as
// some ancillary data such as the data offset (the byte offset pointing to
// where the application payload begins) and the TCP flags if applicable.
// This struct is populated by calling `read_conn_tuple_skb` from a program type
// that manipulates a `__sk_buff` object.
typedef struct {
    conn_tuple_t tup;
    __u32 data_off;
    __u8 tcp_flags;
} skb_info_t;

// This determines the size of the payload fragment that is captured for each HTTP request
#define HTTP_BUFFER_SIZE 25
// This controls the number of HTTP transactions read from userspace at a time
#define HTTP_BATCH_SIZE 15
// The greater this number is the less likely are colisions/data-races between the flushes
#define HTTP_BATCH_PAGES 10

typedef enum {
    HTTP_PACKET_UNKNOWN,
    HTTP_REQUEST,
    HTTP_RESPONSE
} http_packet_t;

typedef enum {
    HTTP_METHOD_UNKNOWN,
    HTTP_GET,
    HTTP_POST,
    HTTP_PUT,
    HTTP_DELETE,
    HTTP_HEAD,
    HTTP_OPTIONS,
    HTTP_PATCH
} http_method_t;

typedef struct {
    // idx is a monotonic counter used for uniquely determinng a batch within a CPU core
    // this is useful for detecting race conditions that result in a batch being overrriden
    // before it gets consumed from userspace
    __u64 idx;
    // pos indicates the current batch slot that should be written to
    __u8 pos;
} http_batch_state_t;

// This struct is used in the map lookup that returns the active batch for a certain CPU core
typedef struct {
    __u32 cpu;
    // page_num can be obtained from (http_batch_state_t->idx % HTTP_BATCHES_PER_CPU)
    __u32 page_num;
} http_batch_key_t;

// HTTP transaction information associated to a certain socket (tuple_t)
typedef struct {
    conn_tuple_t tup;
    __u8 request_method;
    __u64 request_started;
    __u16 response_status_code;
    __u64 response_last_seen;
    char request_fragment[HTTP_BUFFER_SIZE];
} http_transaction_t;

typedef struct {
    http_batch_state_t state;
    http_transaction_t txs[HTTP_BATCH_SIZE];
} http_batch_t;

// http_batch_notification_t is flushed to userspace every time we complete a
// batch (that is, when we fill a page with HTTP_BATCH_SIZE entries). uppon
// receiving this notification the userpace program is then supposed to fetch
// the full batch by doing a map lookup using `cpu` and then retrieving the full
// page using batch_idx param. why just not flush the batch itself via the
// perf-ring? we do this because prior to Kernel 4.11 bpf_perf_event_output
// requires the data to be allocated in the eBPF stack. that makes batching
// virtually impossible given the stack limit of 512bytes.
typedef struct {
    __u32 cpu;
    __u64 batch_idx;
} http_batch_notification_t;

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

typedef struct {
    __u32 net_ns;
    __u16 port;
} port_binding_t;

#endif
