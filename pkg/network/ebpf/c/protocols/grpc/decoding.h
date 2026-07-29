#ifndef __GRPC_DECODING_H
#define __GRPC_DECODING_H

#include "protocols/http2/decoding.h"
#include "protocols/http2/decoding-defs.h"
#include "protocols/grpc/defs.h"
#include "protocols/grpc/helpers.h"

// pktbuf-based gRPC classification.
//
// This is the shared-path (plaintext socket-filter *and* decrypted-TLS) equivalent of the skb-only
// `is_grpc` in grpc/helpers.h. It reuses the HTTP/2 pktbuf primitives from decoding.h so it can run
// over a decrypted TLS user buffer, and is invoked from a dedicated tail-call program rather than
// from the (verifier-budget-constrained) HTTP/2 headers parser.

// Inspects an indexed content-type header value for the huffman-encoded "application/grpc" prefix.
// `idx` is the HPACK index that was just read; `frame_end` bounds the header block. The value is
// consumed (offset advanced) exactly like the skb variant so the caller's header loop stays in sync.
static __always_inline grpc_status_t pktbuf_is_content_type_grpc(pktbuf_t pkt, __u32 frame_end, __u64 idx) {
    // We only care about the indexed content-type name (static table index 31).
    if (idx != HTTP2_CONTENT_TYPE_IDX) {
        return PAYLOAD_UNDETERMINED;
    }

    string_literal_header_t len;
    if (pktbuf_data_offset(pkt) + sizeof(len) > frame_end) {
        return PAYLOAD_NOT_GRPC;
    }

    pktbuf_load_bytes_from_current_offset(pkt, &len, sizeof(len));
    pktbuf_advance(pkt, sizeof(len));

    // The content-type length must be able to hold *at least* "application/grpc". It can be larger,
    // e.g. "application/grpc+proto", which we still want to match.
    if (len.length < GRPC_CONTENT_TYPE_LEN) {
        return PAYLOAD_NOT_GRPC;
    }

    char content_type_buf[GRPC_CONTENT_TYPE_LEN];
    pktbuf_load_bytes_from_current_offset(pkt, content_type_buf, GRPC_CONTENT_TYPE_LEN);
    pktbuf_advance(pkt, len.length);

    return bpf_memcmp(content_type_buf, GRPC_ENCODED_CONTENT_TYPE, GRPC_CONTENT_TYPE_LEN) == 0 ? PAYLOAD_GRPC : PAYLOAD_NOT_GRPC;
}

// Scans the headers of a single HEADERS frame looking for a content-type header.
static __always_inline grpc_status_t pktbuf_scan_headers_grpc(pktbuf_t pkt, __u32 frame_length) {
    __u8 current_ch;
    grpc_status_t status = PAYLOAD_UNDETERMINED;
    __u64 index = 0;
    __u64 max_bits = 0;

    __u32 frame_end = pktbuf_data_offset(pkt) + frame_length;
    // Ensure frame_end does not go beyond the packet.
    frame_end = frame_end < pktbuf_data_end(pkt) + 1 ? frame_end : pktbuf_data_end(pkt) + 1;

    pktbuf_handle_dynamic_table_update(pkt);

#pragma unroll(GRPC_MAX_HEADERS_TO_PROCESS)
    for (__u8 i = 0; i < GRPC_MAX_HEADERS_TO_PROCESS; ++i) {
        if (pktbuf_data_offset(pkt) >= frame_end) {
            break;
        }

        pktbuf_load_bytes_from_current_offset(pkt, &current_ch, sizeof(current_ch));
        pktbuf_advance(pkt, sizeof(current_ch));

        if ((current_ch & 128) != 0) {
            // Fully indexed header - nothing to read, skip it.
            continue;
        }

        // Literal header with indexing uses 6-bit prefix; without/never-indexed use 4-bit.
        max_bits = (current_ch & 192) == 64 ? MAX_6_BITS : MAX_4_BITS;
        index = 0;
        if (!pktbuf_read_hpack_int_with_given_current_char(pkt, current_ch, max_bits, &index)) {
            break;
        }

        status = pktbuf_is_content_type_grpc(pkt, frame_end, index);
        if (status != PAYLOAD_UNDETERMINED) {
            break;
        }

        if (!pktbuf_process_and_skip_literal_headers(pkt, index)) {
            break;
        }
    }

    return status;
}

// pktbuf_is_grpc walks the HTTP/2 frames in the (decrypted) buffer to the first HEADERS frame and
// scans it for a "content-type: application/grpc" header. Returns PAYLOAD_UNDETERMINED when no
// HEADERS frame / content-type is present in this buffer, so the caller can retry on later buffers.
static __always_inline grpc_status_t pktbuf_is_grpc(pktbuf_t pkt) {
    http2_frame_t current_frame;
    bool found_headers = false;

    // Skip the HTTP/2 connection preface if present.
    pktbuf_skip_preface(pkt);

#pragma unroll(GRPC_MAX_FRAMES_TO_FILTER)
    for (__u8 i = 0; i < GRPC_MAX_FRAMES_TO_FILTER; ++i) {
        if (pktbuf_data_offset(pkt) + HTTP2_FRAME_HEADER_SIZE > pktbuf_data_end(pkt)) {
            break;
        }

        // read_frame reads the frame header, advances past it, and validates it.
        if (!read_frame(pkt, &current_frame)) {
            break;
        }

        if (current_frame.type == kHeadersFrame) {
            found_headers = true;
            break;
        }

        pktbuf_advance(pkt, current_frame.length);
    }

    if (!found_headers) {
        return PAYLOAD_UNDETERMINED;
    }

    return pktbuf_scan_headers_grpc(pkt, current_frame.length);
}

#endif
