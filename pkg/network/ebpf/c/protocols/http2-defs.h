#ifndef __HTTP2_DEFS_H
#define __HTTP2_DEFS_H

#include <linux/types.h>

// A limit of max frames we will upload from a single connection to the user mode.
// NOTE: we may need to revisit this const if we need to capture more connections.
#define HTTP2_MAX_FRAMES 3

// A limit of max headers frames which we except to see in the request/response.
// NOTE: we may need to change the max size.
#define HTTP2_MAX_HEADERS_COUNT 15

// A limit of max frame size in order to be able to load a max size and pass the varifier.
// NOTE: we may need to change the max size.
#define HTTP2_MAX_PATH_LEN 32

#define MAX_STATIC_TABLE_INDEX 64

#define HTTP2_FRAME_HEADER_SIZE 9

#define HTTP2_SETTINGS_SIZE 6

// This determines the size of the payload fragment that is captured for each HTTP request
#define HTTP2_BUFFER_SIZE (8 * 20)

#define HTTP2_BATCH_SIZE 15

// HTTP_BATCH_PAGES controls how many `http_batch_t` instances exist for each CPU core
// It's desirable to set this >= 1 to allow batch insertion and flushing to happen idependently
// without risk of overriding data.
#define HTTP2_BATCH_PAGES 3


typedef enum {
    kMethod = 2,
    kPath = 4,
    kStatus = 9,
} __attribute__ ((packed)) header_key;

typedef enum {
    kGET = 2,
    kPOST = 3,
    kEmptyPath = 4,
    kIndexPath = 5,
    k200 = 8,
    k204 = 9,
    k206 = 10,
    k304 = 11,
    k400 = 12,
    k404 = 13,
    k500 = 14,
} __attribute__ ((packed)) header_value;

typedef struct {
    header_key name;
    header_value value;
} static_table_value;

typedef struct {
    char buffer[32] __attribute__ ((aligned (8)));
    __u64 string_len;
} __attribute__ ((packed)) dynamic_string_value;

typedef struct {
    __u64 index;
    dynamic_string_value value;
} dynamic_table_value;

typedef struct {
    __u64 index;
    conn_tuple_t old_tup;
} dynamic_table_index;

typedef enum
{
    HTTP2_PACKET_UNKNOWN,
    HTTP2_REQUEST,
    HTTP2_RESPONSE
} http2_packet_t;

typedef enum
{
    HTTP2_SCHEMA_UNKNOWN,
    HTTP_SCHEMA,
} http2_schema_t;

typedef enum
{
    HTTP2_METHOD_UNKNOWN,
    HTTP2_GET,
    HTTP2_POST,
    HTTP2_PUT,
    HTTP2_DELETE,
    HTTP2_HEAD,
    HTTP2_OPTIONS,
    HTTP2_PATCH
} http2_method_t;

typedef struct {
    conn_tuple_t tup;
    char request_fragment[HTTP2_BUFFER_SIZE] __attribute__ ((aligned (8)));

    char *frag_head;
    char *frag_end;
} http2_connection_t;

// HTTP2 transaction information associated to a certain socket (tuple_t)
typedef struct {
    conn_tuple_t old_tup;
    conn_tuple_t tup;
    __u64 request_started;
    __u64 tags;
    __u64 response_last_seen;

    __u32 tcp_seq;
    __u32 current_offset_in_request_fragment;

    char request_fragment[HTTP2_BUFFER_SIZE] __attribute__ ((aligned (8)));

    __u16 response_status_code;
    __u16 owned_by_src_port;

    bool end_of_stream;
    __u8  request_method;
    __u8  packet_type;
    __u8  stream_id;
    __u64  path_size;
    char path[32] __attribute__ ((aligned (8)));
} http2_transaction_t;

// This struct is used in the map lookup that returns the active batch for a certain CPU core
typedef struct {
    __u32 cpu;
    // page_num can be obtained from (http_batch_state_t->idx % HTTP_BATCHES_PER_CPU)
    __u32 page_num;
} http2_batch_key_t;

typedef struct {
    __u64 idx;
    __u8 pos;
    http2_transaction_t txs[HTTP2_BATCH_SIZE];
} http2_batch_t;

#endif
