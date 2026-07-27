#ifndef __GRPC_DEFS_H
#define __GRPC_DEFS_H

typedef enum {
    PAYLOAD_UNDETERMINED,
    PAYLOAD_GRPC,
    PAYLOAD_NOT_GRPC,
} grpc_status_t;

typedef struct {
    __u32 offset;
    __u32 length;
} frame_info_t;

// The HPACK specification defines the specific Huffman encoding used for string
// literals in HPACK. This allows us to precomputed the encoded string for
// "application/grpc". Even though it is huffman encoded, this particular string
// is byte-aligned and can be compared without any masking on the final byte.
#define GRPC_ENCODED_CONTENT_TYPE "\x1d\x75\xd0\x62\x0d\x26\x3d\x4c\x4d\x65\x64"
#define GRPC_CONTENT_TYPE_LEN (sizeof(GRPC_ENCODED_CONTENT_TYPE) - 1)
// Number of leading huffman-encoded bytes of "application/grpc" compared during HTTP/2 decoding.
// Kept short (a single 8-byte word) so the check stays small enough for the decoder's unrolled loops;
// 8 bytes of the content-type value are more than enough to distinguish gRPC from other content types.
#define GRPC_CONTENT_TYPE_PREFIX_LEN 8

#endif
