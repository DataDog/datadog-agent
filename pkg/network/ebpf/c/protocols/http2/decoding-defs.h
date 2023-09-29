#ifndef __HTTP2_DECODING_DEFS_H
#define __HTTP2_DECODING_DEFS_H

#include "ktypes.h"

#include "protocols/http2/defs.h"

// Maximum number of frames to be processed in a single TCP packet. That's also the number of tail calls we'll have.
// NOTE: we may need to revisit this const if we need to capture more connections.
#define HTTP2_MAX_FRAMES_ITERATIONS 10
#define HTTP2_MAX_FRAMES_TO_FILTER  500

// A limit of max headers which we process in the request/response.
#define HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING 20

// Per request or response we have fewer headers than HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING that are interesting us.
// For request - those are method, path, and soon to be content type. For response - status code.
// Thus differentiating between the limits can allow reducing code size.
#define HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING 3

// Maximum size for the path buffer.
#define HTTP2_MAX_PATH_LEN 160

// The maximum index which may be in the static table.
#define MAX_STATIC_TABLE_INDEX 61

// The flag which will be sent in the data/header frame that indicates end of stream.
#define HTTP2_END_OF_STREAM 0x1

// Http2 max batch size.
#define HTTP2_BATCH_SIZE 17

// MAX_6_BITS represents the maximum number that can be represented with 6 bits or less.
// 1 << 6 - 1
#define MAX_6_BITS 63

// MAX_6_BITS represents the maximum number that can be represented with 6 bits or less.
// 1 << 7 - 1
#define MAX_7_BITS 127

#define HTTP2_CONTENT_TYPE_IDX 31

// Huffman-encoded strings for paths "/" and "/index.html". Needed for HTTP2
// decoding, as these two paths are in the static table, we need to add the
// encoded string ourselves instead of reading them from the Header.
#define HTTP_ROOT_PATH      "\x63"
#define HTTP_ROOT_PATH_LEN  (sizeof(HTTP_ROOT_PATH) - 1)
#define HTTP_INDEX_PATH     "\x60\xd5\x48\x5f\x2b\xce\x9a\x68"
#define HTTP_INDEX_PATH_LEN (sizeof(HTTP_INDEX_PATH) - 1)

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

    __MAX_STATIC_TABLE_INDEX = 255,
} __attribute__((packed)) static_table_value_t;

typedef struct {
    char buffer[HTTP2_MAX_PATH_LEN] __attribute__((aligned(8)));
    __u8 string_len;
} dynamic_table_entry_t;

typedef struct {
    __u64 index;
    conn_tuple_t tup;
} dynamic_table_index_t;

typedef struct {
    conn_tuple_t tup;
    __u32 stream_id;
} http2_stream_key_t;

typedef struct {
    conn_tuple_t tup;
    __u64 response_last_seen;
    __u64 request_started;

    __u16 response_status_code;
    __u8 request_method;
    __u8 path_size;
    bool request_end_of_stream;

    __u8 request_path[HTTP2_MAX_PATH_LEN] __attribute__((aligned(8)));
} http2_stream_t;

typedef struct {
    dynamic_table_index_t dynamic_index;
    http2_stream_key_t http2_stream_key;
} http2_ctx_t;

typedef enum {
    kStaticHeader = 0,
    kExistingDynamicHeader = 1,
    kNewDynamicHeader = 2,
} __attribute__((packed)) http2_header_type_t;

typedef struct {
    __u32 index;
    __u32 new_dynamic_value_offset;
    __u32 new_dynamic_value_size;
    http2_header_type_t type;
} http2_header_t;

typedef struct {
    struct http2_frame frame;
    __u32 offset;
} http2_frame_with_offset;

typedef struct {
    __u8 iteration;
    __u8 frames_count;
    http2_frame_with_offset frames_array[HTTP2_MAX_FRAMES_ITERATIONS] __attribute__((aligned(8)));
} http2_tail_call_state_t;

#endif
