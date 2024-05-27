#ifndef __HTTP2_SKB_COMMON_H
#define __HTTP2_SKB_COMMON_H

#include "protocols/helpers/pktbuf.h"
#include "protocols/http2/helpers.h"

// Similar to read_hpack_int, but with a small optimization of getting the
// current character as input argument.
static __always_inline bool read_hpack_int_with_given_current_char(pktbuf pkt, __u64 current_char_as_number, __u64 max_number_for_bits, __u64 *out) {
    current_char_as_number &= max_number_for_bits;

    // In HPACK, if the number is too big to be stored in max_number_for_bits
    // bits, then those bits are all set to one, and the rest of the number must
    // be read from subsequent bytes.
    if (current_char_as_number < max_number_for_bits) {
        *out = current_char_as_number;
        return true;
    }

    // Read the next byte, and check if it is the last byte of the number.
    // While HPACK does support arbitrary sized numbers, we are limited by the
    // number of instructions we can use in a single eBPF program, so we only
    // parse one additional byte. The max value that can be parsed is
    // `(2^max_number_for_bits - 1) + 127`.
    __u64 next_char = 0;
    if (pktbuf_load_bytes(pktbuf, pktbuf_data_offset(pkt), &next_char, 1) >= 0 && (next_char & 128) == 0) {
        pktbuf_advance(pktbuf, 1);
        *out = current_char_as_number + (next_char & 127);
        return true;
    }

    return false;
}

// read_hpack_int reads an unsigned variable length integer as specified in the
// HPACK specification, from an skb.
//
// See https://httpwg.org/specs/rfc7541.html#rfc.section.5.1 for more details on
// how numbers are represented in HPACK.
//
// max_number_for_bits represents the number of bits in the first byte that are
// used to represent the MSB of number. It must always be between 1 and 8.
//
// The parsed number is stored in out.
//
// read_hpack_int returns true if the integer was successfully parsed, and false
// otherwise.
static __always_inline bool read_hpack_int(pktbuf pkt, __u64 max_number_for_bits, __u64 *out, bool *is_huffman_encoded) {
    __u64 current_char_as_number = 0;
    if (pktbuf_load_bytes(pkt, pktbuf_data_offset(pkt), &current_char_as_number, 1) < 0) {
        return false;
    }
    pktbuf_advance(pkt, 1);
    // We are only interested in the first bit of the first byte, which indicates if it is huffman encoded or not.
    // See: https://datatracker.ietf.org/doc/html/rfc7541#appendix-B for more details on huffman code.
    *is_huffman_encoded = (current_char_as_number & 128) > 0;

    return read_hpack_int_with_given_current_char(pkt, current_char_as_number, max_number_for_bits, out);
}

// Handles a literal header, and updates the offset. This function is meant to run on not interesting literal headers.
static __always_inline bool process_and_skip_literal_headers(pktbuf pkt, __u64 index) {
    __u64 str_len = 0;
    bool is_huffman_encoded = false;
    // String length supposed to be represented with at least 7 bits representation -https://datatracker.ietf.org/doc/html/rfc7541#section-5.2
    if (!read_hpack_int(pkt, MAX_7_BITS, &str_len, &is_huffman_encoded)) {
        return false;
    }

    // The header name is new and inserted in the dynamic table - we skip the new value.
    if (index == 0) {
        pktbuf_advance(pkt, str_len);
        str_len = 0;
        // String length supposed to be represented with at least 7 bits representation -https://datatracker.ietf.org/doc/html/rfc7541#section-5.2
        // At this point the huffman code is not interesting due to the fact that we already read the string length,
        // We are reading the current size in order to skip it.
        if (!read_hpack_int(pkt, MAX_7_BITS, &str_len, &is_huffman_encoded)) {
            return false;
        }
    }
    pktbuf_advance(pkt, str_len);
    return true;
}

// handle_dynamic_table_update handles the dynamic table size update.
static __always_inline void handle_dynamic_table_update(pktbuf pkt){
    // To determine the size of the dynamic table update, we read an integer representation byte by byte.
    // We continue reading bytes until we encounter a byte without the Most Significant Bit (MSB) set,
    // indicating that we've consumed the complete integer. While in the context of the dynamic table
    // update, we set the state as true if the MSB is set, and false otherwise. Then, we proceed to the next byte.
    // More on the feature - https://httpwg.org/specs/rfc7541.html#rfc.section.6.3.
    __u8 current_ch;
    pktbuf_load_bytes(pkt, pktbuf_data_offset(pkt), &current_ch, sizeof(current_ch));
    // If the top 3 bits are 001, then we have a dynamic table size update.
    if ((current_ch & 224) == 32) {
        pktbuf_advance(pktbuf, 1);
    #pragma unroll(HTTP2_MAX_DYNAMIC_TABLE_UPDATE_ITERATIONS)
        for (__u8 iter = 0; iter < HTTP2_MAX_DYNAMIC_TABLE_UPDATE_ITERATIONS; ++iter) {
            pktbuf_load_bytes(pkt, pktbuf_data_offset(pkt), &current_ch, sizeof(current_ch));
            pktbuf_advance(pkt, 1);
            if ((current_ch & 128) == 0) {
                return;
            }
        }
    }
}

// skip_preface is a helper function to check for the HTTP2 magic sent at the beginning
// of an HTTP2 connection, and skip it if present.
static __always_inline void skip_preface(pktbuf pkt) {
    char preface[HTTP2_MARKER_SIZE];
    bpf_memset((char *)preface, 0, HTTP2_MARKER_SIZE);
    pktbuf_load_bytes(pkt, pktbuf_data_offset(pkt), preface, HTTP2_MARKER_SIZE);
    if (is_http2_preface(preface, HTTP2_MARKER_SIZE)) {
        pktbuf_advance(pkt, HTTP2_MARKER_SIZE);
    }
}

#endif
