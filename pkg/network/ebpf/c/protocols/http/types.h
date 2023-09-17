#ifndef __HTTP_TYPES_H
#define __HTTP_TYPES_H

#include "conn_tuple.h"

// This determines the size of the payload fragment that is captured for each HTTP request
#define HTTP_BUFFER_SIZE (8 * 20)
// This controls the number of HTTP transactions read from userspace at a time
#define HTTP_BATCH_SIZE 15

// HTTP/1.1 XXX
// _________^
#define HTTP_STATUS_OFFSET 9

// Pseudo TCP sequence number representing a segment with a FIN or RST flags set
// For more information see `http_seen_before`
#define HTTP_TERMINATING 0xFFFFFFFF

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

// HTTP transaction information associated to a certain socket (tuple_t)
typedef struct {
    conn_tuple_t tup;
    __u64 request_started;
    __u64 response_last_seen;
    __u64 tags;
    // this field is used to disambiguate segments in the context of keep-alives
    // we populate it with the TCP seq number of the request and then the response segments
    __u32 tcp_seq;
    __u16 response_status_code;
    __u8  request_method;
    char request_fragment[HTTP_BUFFER_SIZE] __attribute__ ((aligned (8)));
} http_transaction_t;

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

#endif
