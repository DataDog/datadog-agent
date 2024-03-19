#ifndef __HTTP2_DEFS_H
#define __HTTP2_DEFS_H

#include "ktypes.h"

#define bool _Bool
#define true 1
#define false 0

// Checkout https://datatracker.ietf.org/doc/html/rfc7540 under "HTTP/2 Connection Preface" section
#define HTTP2_MARKER_SIZE 24

// Checkout https://datatracker.ietf.org/doc/html/rfc7540 under "Frame Format" section
#define HTTP2_FRAME_HEADER_SIZE 9
#define HTTP2_SETTINGS_SIZE 6

// All types of http2 frames exist in the protocol.
// Checkout https://datatracker.ietf.org/doc/html/rfc7540 under "Frame Type Registry" section.
typedef enum {
    kDataFrame          = 0,
    kHeadersFrame       = 1,
    kPriorityFrame      = 2,
    kRSTStreamFrame     = 3,
    kSettingsFrame      = 4,
    kPushPromiseFrame   = 5,
    kPingFrame          = 6,
    kGoAwayFrame        = 7,
    kWindowUpdateFrame  = 8,
    kContinuationFrame  = 9,
} __attribute__ ((packed)) frame_type_t;

// Struct which represent the http2 frame by its fields.
// Checkout https://datatracker.ietf.org/doc/html/rfc7540#section-4.1 for frame format.
typedef struct {
    __u32 length : 24;
    frame_type_t type;
    __u8 flags;
    __u8 reserved : 1;
    __u32 stream_id : 31;
} __attribute__ ((packed)) http2_frame_t;


/* Header parsing helper macros */
#define is_indexed(x) ((x) & (1 << 7))
#define is_literal(x) ((x) & (1 << 6))

/* Header parsing helper structs */

// string_literal_header represents the length of a string as represented in HPACK
// (see RFC 7541: 5.2 String Literal Representation).
typedef struct {
    __u8 length : 7;
    __u8 is_huffman : 1;
} __attribute__((packed)) string_literal_header_t;

#endif
