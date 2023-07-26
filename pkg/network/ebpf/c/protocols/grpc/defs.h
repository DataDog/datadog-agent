#ifndef __GRPC_DEFS_H
#define __GRPC_DEFS_H

typedef enum {
  PAYLOAD_UNDETERMINED,
  PAYLOAD_GRPC,
  PAYLOAD_NOT_GRPC,
} grpc_status_t;

/* Header parsing helper macros */
#define is_indexed(x) ((x) & (1 << 7))
#define is_literal(x) ((x) & (1 << 6))

/* Header parsing helper structs */

// field_index represents the different way a header index can be formatted
// according to the HPACK specification (RFC 7541: 6. Binary format)
// We not handling the case of non-indexed field.
union field_index {
    struct {
        __u8 index : 7;
        __u8 reserved : 1;
    } __attribute__((packed)) indexed;
    struct {
        __u8 index : 6;
        __u8 reserved : 2;
    } __attribute__((packed)) literal;
    __u8 raw;
} __attribute__((packed));

// hpack_length represents the length of a string as represented in HPACK
// (see RFC 7541: 5.2 String Literal Representation).
struct hpack_length {
    __u8 length : 7;
    __u8 is_huffman : 1;
} __attribute__((packed));

#endif
