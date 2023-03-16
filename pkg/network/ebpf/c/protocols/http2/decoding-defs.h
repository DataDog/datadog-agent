#ifndef __HTTP2_DECODING_DEFS_H
#define __HTTP2_DECODING_DEFS_H

#include "ktypes.h"

#include "protocols/http2/defs.h"

// Maximum number of frames to be processed in a single TCP packet. That's also the number of tail calls we'll have.
// NOTE: we may need to revisit this const if we need to capture more connections.
#define HTTP2_MAX_FRAMES_ITERATIONS 10

// A limit of max headers which we process in the request/response.
// NOTE: we may need to change the max size.
#define HTTP2_MAX_HEADERS_COUNT 20

// Maximum size for the path buffer.
// NOTE: we may need to change the max size.
#define HTTP2_MAX_PATH_LEN 30

// The maximum index which may be in the static table.
#define MAX_STATIC_TABLE_INDEX 61

// This determines the size of the payload fragment that is captured for each headers frame.
#define HTTP2_BUFFER_SIZE (8 * 20)

// The flag which will be sent in the data/header frame that indicates end of stream.
#define HTTP2_END_OF_STREAM 0x1

// Http2 max batch size.
#define HTTP2_BATCH_SIZE 10

typedef enum {
    kMethod = 2,
    kPath = 4,
    kStatus = 9,
} __attribute__ ((packed)) static_table_key_t;

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
} __attribute__ ((packed)) static_table_value_t;

typedef struct {
    static_table_key_t key;
    static_table_value_t value;
} static_table_entry_t;

typedef struct {
    char buffer[HTTP2_MAX_PATH_LEN] __attribute__ ((aligned (8)));
    __u8 string_len;
} dynamic_table_entry_t;

typedef struct {
    __u64 index;
    conn_tuple_t tup;
} dynamic_table_index_t;

typedef struct {
    conn_tuple_t tup;
    __u32  stream_id;
} http2_stream_key_t;

typedef struct {
    conn_tuple_t tup;
    __u64 response_last_seen;
    __u64 request_started;

    __u16 response_status_code;
    __u8 request_method;
    __u8 path_size;
    bool request_end_of_stream;

    __u8 request_path[HTTP2_MAX_PATH_LEN] __attribute__ ((aligned (8)));
} http2_stream_t;

typedef struct {
    dynamic_table_index_t dynamic_index;
    http2_stream_key_t http2_stream_key;
    http2_stream_t http2_stream;
} http2_ctx_t;

typedef struct {
    char fragment[HTTP2_BUFFER_SIZE];
    __u16 offset;
    __u16 size;
} heap_buffer_t;

typedef enum {
    kStaticHeader  = 0,
    kDynamicHeader = 1,
} __attribute__ ((packed)) http2_header_type_t;

typedef struct {
    __u32 stream_id;
    __u32 index;
    http2_header_type_t type;
} http2_header_t;

typedef struct {
    http2_header_t array[HTTP2_MAX_HEADERS_COUNT];
} http2_headers_t;

typedef struct {
    __u32 offset;
    __u8 iteration;
} http2_tail_call_state_t;

typedef enum {
    HEADER_ERROR = 0,
    HEADER_NOT_INTERESTING,
    HEADER_INTERESTING,
} parse_result_t;

#endif
