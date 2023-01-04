#ifndef __HTTP2_DEFS_H
#define __HTTP2_DEFS_H

#include <linux/types.h>

// A limit of max frames we will upload from a single connection to the user mode.
// NOTE: we may need to revisit this const if we need to capture more connections.
#define HTTP2_MAX_FRAMES 5

// A limit of max headers frames which we except to see in the request/response.
// NOTE: we may need to change the max size.
#define HTTP2_MAX_HEADERS_COUNT 10

// A limit of max frame size in order to be able to load a max size and pass the varifier.
// NOTE: we may need to change the max size.
#define HTTP2_MAX_PATH_LEN 32

#define MAX_STATIC_TABLE_INDEX 64

#define HTTP2_FRAME_HEADER_SIZE 9

#define HTTP2_SETTINGS_SIZE 6

typedef enum {
    kAuthority = 1,
    kMethod = 2,
    kPath = 4,
    kScheme = 6,
    kStatus = 9,
} __attribute__ ((packed)) header_key;

typedef enum {
    kGET = 2,
    kPOST = 3,
    kEmptyPath = 4,
    kIndexPath = 5,
    kHTTP = 6,
    kHTTPS = 7,
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
} __attribute__ ((packed)) dynamic_string_value;

typedef struct {
    __u64 index;
    dynamic_string_value value;
} dynamic_table_value;

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

#endif
