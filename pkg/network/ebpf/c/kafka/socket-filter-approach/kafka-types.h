#ifndef __KAFKA_TYPES_H
#define __KAFKA_TYPES_H

#include "../../tracer.h"

// Every kafka message encodes starts with:
//  * 4 bytes for the size of the payload
//  * 2 bytes for api key
//  * 2 bytes for api version
//  * 4 bytes for correlation id
// Reference: https://kafka.apache.org/protocol.html#protocol_messages
#define KAFKA_MIN_SIZE 12

// Max today is 13 for fetch (https://kafka.apache.org/protocol.html#protocol_messages)
#define KAFKA_MAX_VERSION 13

#define KAFKA_MAX_API 67

//// This determines the size of the payload fragment that is captured for each HTTP request
//#define HTTP_BUFFER_SIZE (8 * 20)
#define KAFKA_BUFFER_SIZE (8 * 20)

#define CLIENT_ID_MAX_STRING_SIZE (8 * 8)
#define TOPIC_NAME_MAX_STRING_SIZE (8 * 8)

//// This controls the number of HTTP transactions read from userspace at a time
//#define HTTP_BATCH_SIZE 15
//// HTTP_BATCH_PAGES controls how many `http_batch_t` instances exist for each CPU core
//// It's desirable to set this >= 1 to allow batch insertion and flushing to happen idependently
//// without risk of overriding data.
//#define HTTP_BATCH_PAGES 3
//
#define KAFKA_PROG 0
//
//// HTTP/1.1 XXX
//// _________^
//#define HTTP_STATUS_OFFSET 9
//
//// This is needed to reduce code size on multiple copy opitmizations that were made in
//// the http eBPF program.
//_Static_assert((HTTP_BUFFER_SIZE % 8) == 0, "HTTP_BUFFER_SIZE must be a multiple of 8.");
//
//typedef enum
//{
//    HTTP_PACKET_UNKNOWN,
//    HTTP_REQUEST,
//    HTTP_RESPONSE
//} http_packet_t;
//
//typedef enum
//{
//    HTTP_METHOD_UNKNOWN,
//    HTTP_GET,
//    HTTP_POST,
//    HTTP_PUT,
//    HTTP_DELETE,
//    HTTP_HEAD,
//    HTTP_OPTIONS,
//    HTTP_PATCH
//} http_method_t;
//
//// This struct is used in the map lookup that returns the active batch for a certain CPU core
//typedef struct {
//    __u32 cpu;
//    // page_num can be obtained from (http_batch_state_t->idx % HTTP_BATCHES_PER_CPU)
//    __u32 page_num;
//} http_batch_key_t;
//
// HTTP transaction information associated to a certain socket (tuple_t)
// Kafka transaction information associated to a certain socket (tuple_t)
typedef struct {
    // this field is used exclusively in the kernel side to prevent a TCP segment
    // to be processed twice in the context of localhost traffic. The field will
    // be populated with the "original" (pre-normalization) source port number of
    // the TCP segment containing the beginning of a given HTTP request
    __u16 owned_by_src_port;
    conn_tuple_t tup;
    uint16_t request_api_key;
    uint16_t request_api_version;
    uint32_t correlation_id;

//    __u64 request_started;
//    __u8  request_method;
//    __u16 response_status_code;
//    __u64 response_last_seen;

    uint32_t current_offset_in_request_fragment;
    char request_fragment[KAFKA_BUFFER_SIZE] __attribute__ ((aligned (8)));
    char client_id[CLIENT_ID_MAX_STRING_SIZE] __attribute__ ((aligned (8)));
    // TODO: Support UTF8
    char topic_name[TOPIC_NAME_MAX_STRING_SIZE] __attribute__ ((aligned (8)));

    // this field is used to disambiguate segments in the context of keep-alives
    // we populate it with the TCP seq number of the request and then the response segments
//    __u32 tcp_seq;
//
//    __u64 tags;
//} http_transaction_t;
} kafka_transaction_t;
//
//typedef struct {
//    // idx is a monotonic counter used for uniquely determinng a batch within a CPU core
//    // this is useful for detecting race conditions that result in a batch being overrriden
//    // before it gets consumed from userspace
//    __u64 idx;
//    // idx_to_flush is used to track which batches were flushed to userspace
//    // * if idx_to_flush == idx, the current index is still being appended to;
//    // * if idx_to_flush < idx, the batch at idx_to_notify needs to be sent to userspace;
//    // (note that idx will never be less than idx_to_flush);
//    __u64 idx_to_flush;
//} http_batch_state_t;
//
//typedef struct {
//    __u64 idx;
//    __u8 pos;
//    http_transaction_t txs[HTTP_BATCH_SIZE];
//} http_batch_t;
//
//// OpenSSL types
//typedef struct {
//    void *ctx;
//    void *buf;
//} ssl_read_args_t;
//
//typedef struct {
//    void *ctx;
//    void *buf;
//} ssl_write_args_t;
//
//typedef struct {
//    void *ctx;
//    void *buf;
//    size_t *size_out_param;
//} ssl_read_ex_args_t;
//
//typedef struct {
//    void *ctx;
//    void *buf;
//    size_t *size_out_param;
//} ssl_write_ex_args_t;
//
//typedef struct {
//    conn_tuple_t tup;
//    __u32 fd;
//} ssl_sock_t;
//
 #define LIB_PATH_MAX_SIZE 120
//
//typedef struct {
//    __u32 pid;
//    __u32 len;
//    char buf[LIB_PATH_MAX_SIZE];
//} lib_path_t;

#endif
