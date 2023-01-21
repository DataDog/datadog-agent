#ifndef __HTTP2_DECODING_DEFS_H
#define __HTTP2_DECODING_DEFS_H

#include <linux/types.h>
#include "http2-defs.h"

// A limit of max frames we will upload from a single connection to the user mode.
// NOTE: we may need to revisit this const if we need to capture more connections.
#define HTTP2_MAX_FRAMES 10

// A limit of max headers frames which we except to see in the request/response.
// NOTE: we may need to change the max size.
#define HTTP2_MAX_HEADERS_COUNT 10

// A limit of max frame size in order to be able to load a max size and pass the varifier.
// NOTE: we may need to change the max size.
#define HTTP2_MAX_PATH_LEN 32

#define MAX_STATIC_TABLE_INDEX 61

// This determines the size of the payload fragment that is captured for each HTTP request
#define HTTP2_BUFFER_SIZE (8 * 20)

#define HTTP2_END_OF_STREAM 0x1

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
    char buffer[32] __attribute__ ((aligned (8)));
    __u64 string_len;
} string_value_t;

// TODO: Do we need the index? Should it be static_table_key_t?
typedef struct {
    __u64 index;
    string_value_t value;
} dynamic_table_entry_t;

typedef struct {
    __u64 index;
    conn_tuple_t old_tup;
} dynamic_table_index_t;

typedef enum {
    HTTP2_PACKET_UNKNOWN,
    HTTP2_REQUEST,
    HTTP2_RESPONSE
} http2_packet_t;

typedef enum {
    HTTP2_METHOD_UNKNOWN,
    HTTP2_GET,
    HTTP2_POST,
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
    char path[HTTP2_MAX_PATH_LEN] __attribute__ ((aligned (8)));
} http2_transaction_t;

typedef struct {
    conn_tuple_t tup;
    skb_info_t skb_info;
    __u8 frames_processed;
    __u16 owned_by_src_port;
} http2_ctx_t;

#endif
