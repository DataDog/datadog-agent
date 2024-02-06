#ifndef __HTTP2_DECODING_DEFS_H
#define __HTTP2_DECODING_DEFS_H

#include "ktypes.h"

#include "protocols/http2/defs.h"

// Represents the maximum number of frames we'll process in a single tail call in `handle_eos_frames` program.
#define HTTP2_MAX_FRAMES_FOR_EOS_PARSER_PER_TAIL_CALL 200
// Represents the maximum number of tail calls to process EOS frames.
// Currently we have up to 120 frames in a packet, thus 1 tail call is enough.
#define HTTP2_MAX_TAIL_CALLS_FOR_EOS_PARSER 2
#define HTTP2_MAX_FRAMES_FOR_EOS_PARSER (HTTP2_MAX_FRAMES_FOR_EOS_PARSER_PER_TAIL_CALL * HTTP2_MAX_TAIL_CALLS_FOR_EOS_PARSER)

// Represents the maximum number of frames we'll process in a single tail call in `handle_headers_frames` program.
#define HTTP2_MAX_FRAMES_FOR_HEADERS_PARSER_PER_TAIL_CALL 19
// Represents the maximum number of tail calls to process headers frames.
// Currently we have up to 240 frames in a packet, thus 13 (13*19 = 247) tail calls is enough.
#define HTTP2_MAX_TAIL_CALLS_FOR_HEADERS_PARSER 13
#define HTTP2_MAX_FRAMES_FOR_HEADERS_PARSER (HTTP2_MAX_FRAMES_FOR_HEADERS_PARSER_PER_TAIL_CALL * HTTP2_MAX_TAIL_CALLS_FOR_HEADERS_PARSER)
// Maximum number of frames to be processed in a single tail call.
#define HTTP2_MAX_FRAMES_ITERATIONS 240
// This represents a limit on the number of tail calls that can be executed
// within the frame filtering  programs (for both TLS and plain text decoding).
// The actual maximum number of frames to parse is defined by HTTP2_MAX_FRAMES_ITERATIONS,
// whose value is computed with following formula:
// HTTP2_MAX_FRAMES_ITERATIONS = HTTP2_MAX_FRAMES_TO_FILTER * HTTP2_MAX_TAIL_CALLS_FOR_FRAMES_FILTER
#define HTTP2_MAX_TAIL_CALLS_FOR_FRAMES_FILTER 1
#define HTTP2_MAX_FRAMES_TO_FILTER 240

// Represents the maximum number octets we will process in the dynamic table update size.
#define HTTP2_MAX_DYNAMIC_TABLE_UPDATE_ITERATIONS 5

// Represents the maximum number of frames we'll process in a single tail call in `uprobe__http2_tls_headers_parser` program.
#define HTTP2_TLS_MAX_FRAMES_FOR_HEADERS_PARSER_PER_TAIL_CALL 15
// Represents the maximum number of tail calls to process headers frames.
// Currently we have up to 120 frames in a packet, thus 8 (8*15 = 120) tail calls is enough.
#define HTTP2_TLS_MAX_TAIL_CALLS_FOR_HEADERS_PARSER 8
#define HTTP2_TLS_MAX_FRAMES_FOR_HEADERS_PARSER (HTTP2_TLS_MAX_FRAMES_FOR_HEADERS_PARSER_PER_TAIL_CALL * HTTP2_TLS_MAX_TAIL_CALLS_FOR_HEADERS_PARSER)

// A limit of max non pseudo headers which we process in the request/response.
// In HTTP/2 we know that we start with pseudo headers and then we have non pseudo headers.
// The max number of headers we process in the request/response is HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING + HTTP2_MAX_PSEUDO_HEADERS_COUNT_FOR_FILTERING.
#define HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING 33

// A limit of max pseudo headers which we process in the request/response.
#define HTTP2_MAX_PSEUDO_HEADERS_COUNT_FOR_FILTERING 4

// Per request or response we have fewer headers than HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING that are interesting us.
// For request - those are method, path. For response - status code.
// Thus differentiating between the limits can allow reducing code size.
#define HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING 2

// Maximum size for the path buffer.
#define HTTP2_MAX_PATH_LEN 160

// Maximum size for the path buffer for telemetry.
#define HTTP2_TELEMETRY_MAX_PATH_LEN 120

// The amount of buckets we have for the path size telemetry.
#define HTTP2_TELEMETRY_PATH_BUCKETS 7

// The size of each bucket we have for the path size telemetry.
#define HTTP2_TELEMETRY_PATH_BUCKETS_SIZE 10

// The maximum index which may be in the static table.
#define MAX_STATIC_TABLE_INDEX 61

// The flag which will be sent in the data/header frame that indicates end of stream.
#define HTTP2_END_OF_STREAM 0x1

// Http2 max batch size.
#define HTTP2_BATCH_SIZE 15

// The max number of events we can have in a single page in the batch_events array.
// See more details in the comments of the USM_EVENTS_INIT.
#define HTTP2_TERMINATED_BATCH_SIZE 80

// MAX_4_BITS represents the maximum number that can be represented with 4 bits or less.
// 1 << 4 - 1
#define MAX_4_BITS 15

// MAX_6_BITS represents the maximum number that can be represented with 6 bits or less.
// 1 << 6 - 1
#define MAX_6_BITS 63

// MAX_6_BITS represents the maximum number that can be represented with 6 bits or less.
// 1 << 7 - 1
#define MAX_7_BITS 127

#define HTTP2_CONTENT_TYPE_IDX 31

#define MAX_FRAME_SIZE 16384

// Definitions representing empty and /index.html paths. These types are sent using the static table.
// We include these to eliminate the necessity of copying the specified encoded path to the buffer.
#define HTTP2_ROOT_PATH      "/"
#define HTTP2_ROOT_PATH_LEN  (sizeof(HTTP2_ROOT_PATH) - 1)
#define HTTP2_INDEX_PATH     "/index.html"
#define HTTP2_INDEX_PATH_LEN (sizeof(HTTP2_INDEX_PATH) - 1)

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
    __u32 original_index;
    __u8 string_len;
    bool is_huffman_encoded;
} dynamic_table_entry_t;

typedef struct {
    __u64 index;
    conn_tuple_t tup;
} dynamic_table_index_t;

typedef struct {
    conn_tuple_t tup;
    __u32 stream_id;
} http2_stream_key_t;

// If the path is huffman encoded then the length is 2, but if it is not, then the length is 3.
#define HTTP2_STATUS_CODE_MAX_LEN 3

// Max length of the method is 7.
#define HTTP2_METHOD_MAX_LEN 7

typedef struct {
    __u8 raw_buffer[HTTP2_STATUS_CODE_MAX_LEN];
    bool is_huffman_encoded;

    __u8 static_table_entry;
    bool finalized;
} status_code_t;

typedef struct {
    __u8 raw_buffer[HTTP2_METHOD_MAX_LEN];
    bool is_huffman_encoded;

    __u8 static_table_entry;
    __u8 length;
    bool finalized;
} method_t;

typedef struct {
    __u64 response_last_seen;
    __u64 request_started;

    status_code_t status_code;
    method_t request_method;
    __u8 path_size;
    bool request_end_of_stream;
    bool is_huffman_encoded;

    __u8 request_path[HTTP2_MAX_PATH_LEN] __attribute__((aligned(8)));
} http2_stream_t;

typedef struct {
    conn_tuple_t tuple;
    http2_stream_t stream;
} http2_event_t;

typedef struct {
    dynamic_table_index_t dynamic_index;
    http2_stream_key_t http2_stream_key;
} http2_ctx_t;

typedef enum {
    kStaticHeader = 0,
    kExistingDynamicHeader = 1,
    kNewDynamicHeader = 2,
    kNewDynamicHeaderNotIndexed = 3,
} __attribute__((packed)) http2_header_type_t;

typedef struct {
    __u32 original_index;
    __u32 index;
    __u32 new_dynamic_value_offset;
    __u32 new_dynamic_value_size;
    http2_header_type_t type;
    bool is_huffman_encoded;
} http2_header_t;

typedef struct {
    http2_frame_t frame;
    __u32 offset;
} http2_frame_with_offset;

typedef struct {
    __u16 iteration;
    __u16 frames_count;
    // Maintains the count of executions performed by the filter program.
    // Its purpose is to restrict the usage of tail calls within the filter program.
    __u16 filter_iterations;
    // Saving the data offset is crucial for maintaining the current read position and ensuring proper utilization
    // of tail calls.
    __u32 data_off;
    http2_frame_with_offset frames_array[HTTP2_MAX_FRAMES_ITERATIONS] __attribute__((aligned(8)));
} http2_tail_call_state_t;

typedef struct {
    __u32 remainder;
    __u32 header_length;
    char buf[HTTP2_FRAME_HEADER_SIZE];
} frame_header_remainder_t;

// http2_telemetry_t is used to hold the HTTP/2 kernel telemetry.
// request_seen                         Count of HTTP/2 requests seen
// response_seen                        Count of HTTP/2 responses seen
// end_of_stream                        Count of END STREAM flag seen
// end_of_stream_rst                    Count of RST flags seen
// path_exceeds_frame                   Count of times we couldn't retrieve the path due to reaching the end of the frame.
// exceeding_max_interesting_frames		Count of times we reached the max number of frames per iteration.
// exceeding_max_frames_to_filter		Count of times we have left with more frames to filter than the max number of frames to filter.
// path_size_bucket                     Count of path sizes and divided into buckets.
typedef struct {
    __u64 request_seen;
    __u64 response_seen;
    __u64 end_of_stream;
    __u64 end_of_stream_rst;
    __u64 path_exceeds_frame;
    __u64 exceeding_max_interesting_frames;
    __u64 exceeding_max_frames_to_filter;
    __u64 path_size_bucket[HTTP2_TELEMETRY_PATH_BUCKETS+1];
} http2_telemetry_t;

#endif
