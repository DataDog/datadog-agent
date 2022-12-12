#ifndef __HTTP_TYPES_H
#define __HTTP_TYPES_H

#include "tracer.h"

// This determines the size of the payload fragment that is captured for each HTTP request
#define HTTP_BUFFER_SIZE (8 * 20)
// This controls the number of HTTP transactions read from userspace at a time
#define HTTP_BATCH_SIZE 15
// HTTP_BATCH_PAGES controls how many `http_batch_t` instances exist for each CPU core
// It's desirable to set this >= 1 to allow batch insertion and flushing to happen idependently
// without risk of overriding data.
#define HTTP_BATCH_PAGES 3

// HTTP/1.1 XXX
// _________^
#define HTTP_STATUS_OFFSET 9

// This is needed to reduce code size on multiple copy opitmizations that were made in
// the http eBPF program.
_Static_assert((HTTP_BUFFER_SIZE % 8) == 0, "HTTP_BUFFER_SIZE must be a multiple of 8.");

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

// This struct is used in the map lookup that returns the active batch for a certain CPU core
typedef struct {
    __u32 cpu;
    // page_num can be obtained from (http_batch_state_t->idx % HTTP_BATCHES_PER_CPU)
    __u32 page_num;
} http_batch_key_t;

// HTTP transaction information associated to a certain socket (tuple_t)
typedef struct {
    conn_tuple_t tup;
    __u64 request_started;
    __u8  request_method;
    __u16 response_status_code;
    __u64 response_last_seen;
    char request_fragment[HTTP_BUFFER_SIZE] __attribute__ ((aligned (8)));

    // this field is used exclusively in the kernel side to prevent a TCP segment
    // to be processed twice in the context of localhost traffic. The field will
    // be populated with the "original" (pre-normalization) source port number of
    // the TCP segment containing the beginning of a given HTTP request
    __u16 owned_by_src_port;

    // this field is used to disambiguate segments in the context of keep-alives
    // we populate it with the TCP seq number of the request and then the response segments
    __u32 tcp_seq;

    __u64 tags;
} http_transaction_t;

typedef struct {
    // idx is a monotonic counter used for uniquely determinng a batch within a CPU core
    // this is useful for detecting race conditions that result in a batch being overrriden
    // before it gets consumed from userspace
    __u64 idx;
    // idx_to_flush is used to track which batches were flushed to userspace
    // * if idx_to_flush == idx, the current index is still being appended to;
    // * if idx_to_flush < idx, the batch at idx_to_notify needs to be sent to userspace;
    // (note that idx will never be less than idx_to_flush);
    __u64 idx_to_flush;
} http_batch_state_t;

typedef struct {
    __u64 idx;
    __u8 pos;
    http_transaction_t txs[HTTP_BATCH_SIZE];
} http_batch_t;

// OpenSSL types
typedef struct {
    void *ctx;
    void *buf;
} ssl_read_args_t;

typedef struct {
    void *ctx;
    void *buf;
} ssl_write_args_t;

typedef struct {
    void *ctx;
    void *buf;
    size_t *size_out_param;
} ssl_read_ex_args_t;

typedef struct {
    void *ctx;
    void *buf;
    size_t *size_out_param;
} ssl_write_ex_args_t;

typedef struct {
    conn_tuple_t tup;
    __u32 fd;
} ssl_sock_t;

#define LIB_PATH_MAX_SIZE 120

typedef struct {
    __u32 pid;
    __u32 len;
    char buf[LIB_PATH_MAX_SIZE];
} lib_path_t;

#endif
