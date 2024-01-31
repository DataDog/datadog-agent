#ifndef __HTTP2_SKB_COMMON_H
#define __HTTP2_SKB_COMMON_H

// skip_preface is a helper function to check for the HTTP2 magic sent at the beginning
// of an HTTP2 connection, and skip it if present.
static __always_inline void skip_preface(const struct __sk_buff *skb, skb_info_t *skb_info) {
    char preface[HTTP2_MARKER_SIZE];
    bpf_memset((char *)preface, 0, HTTP2_MARKER_SIZE);
    bpf_skb_load_bytes(skb, skb_info->data_off, preface, HTTP2_MARKER_SIZE);
    if (is_http2_preface(preface, HTTP2_MARKER_SIZE)) {
        skb_info->data_off += HTTP2_MARKER_SIZE;
    }
}

// Similar to read_hpack_int, but with a small optimization of getting the
// current character as input argument.
static __always_inline bool read_hpack_int_with_given_current_char(const struct __sk_buff *skb, skb_info_t *skb_info, __u64 current_char_as_number, __u64 max_number_for_bits, __u64 *out) {
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
    if (bpf_skb_load_bytes(skb, skb_info->data_off, &next_char, 1) >= 0 && (next_char & 128) == 0) {
        skb_info->data_off++;
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
static __always_inline bool read_hpack_int(const struct __sk_buff *skb, skb_info_t *skb_info, __u64 max_number_for_bits, __u64 *out, bool *is_huffman_encoded) {
    __u64 current_char_as_number = 0;
    if (bpf_skb_load_bytes(skb, skb_info->data_off, &current_char_as_number, 1) < 0) {
        return false;
    }
    skb_info->data_off++;
    // We are only interested in the first bit of the first byte, which indicates if it is huffman encoded or not.
    // See: https://datatracker.ietf.org/doc/html/rfc7541#appendix-B for more details on huffman code.
    *is_huffman_encoded = (current_char_as_number & 128) > 0;

    return read_hpack_int_with_given_current_char(skb, skb_info, current_char_as_number, max_number_for_bits, out);
}

#define SKIP_DYNAMIC_TABLE_UPDATE_SIZE 4

static __always_inline void skip_dynamic_table_update(const struct __sk_buff *skb, skb_info_t *skb_info, __u32 frame_end) {
    __u8 current_ch;
    bool is_dynamic_table_update = false;

#pragma unroll(SKIP_DYNAMIC_TABLE_UPDATE_SIZE)
    for (__u8 i = 0; i < SKIP_DYNAMIC_TABLE_UPDATE_SIZE; ++i) {
        if (skb_info->data_off >= frame_end) {
            break;
        }

        bpf_skb_load_bytes(skb, skb_info->data_off, &current_ch, sizeof(current_ch));
        if (is_dynamic_table_update) {
            is_dynamic_table_update = (current_ch & 128) != 0;
            skb_info->data_off++;
            continue;
        }
        is_dynamic_table_update = (current_ch & 224) == 32;
        if (is_dynamic_table_update) {
            skb_info->data_off++;
            continue;
        }
        break;
    }
}

#endif // __HTTP2_SKB_COMMON_H
