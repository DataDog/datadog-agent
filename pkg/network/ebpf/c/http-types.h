#ifndef __HTTP_TYPES_H
#define __HTTP_TYPES_H

#include "tracer.h"

// This determines the size of the payload fragment that is captured for each HTTP request
#define HTTP_BUFFER_SIZE 25
// This controls the number of HTTP transactions read from userspace at a time
#define HTTP_BATCH_SIZE 15
// The greater this number is the less likely are colisions/data-races between the flushes
#define HTTP_BATCH_PAGES 10

typedef enum
{
    HTTP_PACKET_UNKNOWN,
    HTTP_REQUEST,
    HTTP_RESPONSE
} http_packet_t;

typedef enum
{
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
    // pos indicates the batch slot where the next http transaction should be written to
    __u8 pos;
    // idx_to_notify is used to track which batch completions were notified to userspace
    // * if idx_to_notify == idx, the current index is still being appended to;
    // * if idx_to_notify < idx, the batch at idx_to_notify needs to be sent to userspace;
    // (note that idx will never be less than idx_to_notify);
    __u64 idx_to_notify;
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

    // this field is used exclusively in the kernel side to prevent a TCP segment
    // to be processed twice in the context of localhost traffic. The field will
    // be populated with the "original" (pre-normalization) source port number of
    // the TCP segment containing the beginning of a given HTTP request
    __u16 owned_by_src_port;
} http_transaction_t;

typedef struct {
    __u64 idx;
    __u8 pos;
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

#endif
