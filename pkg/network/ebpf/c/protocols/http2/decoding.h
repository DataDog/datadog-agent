#ifndef __HTTP2_DECODING_H
#define __HTTP2_DECODING_H

#include "protocols/http2/decoding-common.h"
#include "protocols/http2/usm-events.h"
#include "protocols/http/types.h"

// Similar to read_hpack_int, but with a small optimization of getting the
// current character as input argument.
static __always_inline bool read_hpack_int_with_given_current_char(struct __sk_buff *skb, skb_info_t *skb_info, __u64 current_char_as_number, __u64 max_number_for_bits, __u64 *out) {
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
static __always_inline bool read_hpack_int(struct __sk_buff *skb, skb_info_t *skb_info, __u64 max_number_for_bits, __u64 *out, bool *is_huffman_encoded) {
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

// Handles the case in which a header is not a pseudo header. We don't need to save it as interesting or modify our telemetry.
// https://datatracker.ietf.org/doc/html/rfc7540#section-8.1.2.3
// https://datatracker.ietf.org/doc/html/rfc7540#section-8.1.2.4
static __always_inline bool handle_non_pseudo_headers(struct __sk_buff *skb, skb_info_t *skb_info, __u64 index) {
    __u64 str_len = 0;
    bool is_huffman_encoded = false;
    // String length supposed to be represented with at least 7 bits representation -https://datatracker.ietf.org/doc/html/rfc7541#section-5.2
    if (!read_hpack_int(skb, skb_info, MAX_7_BITS, &str_len, &is_huffman_encoded)) {
        return false;
    }

    // The header name is new and inserted in the dynamic table - we skip the new value.
    if (index == 0) {
        skb_info->data_off += str_len;
        str_len = 0;
        // String length supposed to be represented with at least 7 bits representation -https://datatracker.ietf.org/doc/html/rfc7541#section-5.2
        // At this point the huffman code is not interesting due to the fact that we already read the string length,
        // We are reading the current size in order to skip it.
        if (!read_hpack_int(skb, skb_info, MAX_7_BITS, &str_len, &is_huffman_encoded)) {
            return false;
        }
    }
    skb_info->data_off += str_len;
    return true;
}

READ_INTO_BUFFER(path, HTTP2_MAX_PATH_LEN, BLK_SIZE)

// parse_field_literal parses a header with a literal value.
//
// We are only interested in path headers, that we will store in our internal
// dynamic table, and will skip headers that are not path headers.
static __always_inline bool parse_field_literal(struct __sk_buff *skb, skb_info_t *skb_info, http2_header_t *headers_to_process, __u64 index, __u64 global_dynamic_counter, __u8 *interesting_headers_counter, http2_telemetry_t *http2_tel, bool save_header) {
    __u64 str_len = 0;
    bool is_huffman_encoded = false;
    // String length supposed to be represented with at least 7 bits representation -https://datatracker.ietf.org/doc/html/rfc7541#section-5.2
    if (!read_hpack_int(skb, skb_info, MAX_7_BITS, &str_len, &is_huffman_encoded)) {
        return false;
    }

    // The header name is new and inserted in the dynamic table - we skip the new value.
    if (index == 0) {
        skb_info->data_off += str_len;
        str_len = 0;
        // String length supposed to be represented with at least 7 bits representation -https://datatracker.ietf.org/doc/html/rfc7541#section-5.2
        // At this point the huffman code is not interesting due to the fact that we already read the string length,
        // We are reading the current size in order to skip it.
        if (!read_hpack_int(skb, skb_info, MAX_7_BITS, &str_len, &is_huffman_encoded)) {
            return false;
        }
        goto end;
    }

    // Path headers in HTTP2 that are not "/" or "/index.html"  are represented
    // with an indexed name, literal value, reusing the index 4 and 5 in the
    // static table. A different index means that the header is not a path, so
    // we skip it.
    if (index != kIndexPath && index != kEmptyPath) {
        goto end;
    }
    update_path_size_telemetry(http2_tel, str_len);

    // We skip if:
    // - The string is too big
    // - This is not a path
    // - We won't be able to store the header info
    if (headers_to_process == NULL) {
        goto end;
    }

    if (skb_info->data_off + str_len > skb_info->data_end) {
        __sync_fetch_and_add(&http2_tel->path_exceeds_frame, 1);
        goto end;
    }

    if (save_header) {
        headers_to_process->index = global_dynamic_counter - 1;
        headers_to_process->type = kNewDynamicHeader;
    } else {
        headers_to_process->type = kNewDynamicHeaderNotIndexed;
    }
    headers_to_process->new_dynamic_value_offset = skb_info->data_off;
    headers_to_process->new_dynamic_value_size = str_len;
    headers_to_process->is_huffman_encoded = is_huffman_encoded;
    // If the string len (`str_len`) is in the range of [0, HTTP2_MAX_PATH_LEN], and we don't exceed packet boundaries
    // (skb_info->data_off + str_len <= skb_info->data_end) and the index is kIndexPath, then we have a path header,
    // and we're increasing the counter. In any other case, we're not increasing the counter.
    *interesting_headers_counter += (str_len > 0 && str_len <= HTTP2_MAX_PATH_LEN);
end:
    skb_info->data_off += str_len;
    return true;
}

// handle_dynamic_table_update handles the dynamic table size update.
static __always_inline void handle_dynamic_table_update(struct __sk_buff *skb, skb_info_t *skb_info){
    // To determine the size of the dynamic table update, we read an integer representation byte by byte.
    // We continue reading bytes until we encounter a byte without the Most Significant Bit (MSB) set,
    // indicating that we've consumed the complete integer. While in the context of the dynamic table
    // update, we set the state as true if the MSB is set, and false otherwise. Then, we proceed to the next byte.
    // More on the feature - https://httpwg.org/specs/rfc7541.html#rfc.section.6.3.
    __u8 current_ch;
    bpf_skb_load_bytes(skb, skb_info->data_off, &current_ch, sizeof(current_ch));
    // If the top 3 bits are 001, then we have a dynamic table size update.
    if ((current_ch & 224) == 32) {
        skb_info->data_off++;
    #pragma unroll(HTTP2_MAX_DYNAMIC_TABLE_UPDATE_ITERATIONS)
        for (__u8 iter = 0; iter < HTTP2_MAX_DYNAMIC_TABLE_UPDATE_ITERATIONS; ++iter) {
            bpf_skb_load_bytes(skb, skb_info->data_off, &current_ch, sizeof(current_ch));
            skb_info->data_off++;
            if ((current_ch & 128) == 0) {
                return;
            }
        }
    }
}

// filter_relevant_headers parses the http2 headers frame, and filters headers
// that are relevant for us, to be processed later on.
// The return value is the number of relevant headers that were found and inserted
// in the `headers_to_process` table.
static __always_inline __u8 filter_relevant_headers(struct __sk_buff *skb, skb_info_t *skb_info, conn_tuple_t *tup, dynamic_table_index_t *dynamic_index, http2_header_t *headers_to_process, __u32 frame_length, http2_telemetry_t *http2_tel) {
    __u8 current_ch;
    __u8 interesting_headers = 0;
    http2_header_t *current_header;
    const __u32 frame_end = skb_info->data_off + frame_length;
    const __u32 end = frame_end < skb_info->data_end + 1 ? frame_end : skb_info->data_end + 1;
    bool is_indexed = false;
    bool is_literal = false;
    __u64 max_bits = 0;
    __u64 index = 0;

    __u64 *global_dynamic_counter = get_dynamic_counter(tup);
    if (global_dynamic_counter == NULL) {
        return 0;
    }

    handle_dynamic_table_update(skb, skb_info);

#pragma unroll(HTTP2_MAX_PSEUDO_HEADERS_COUNT_FOR_FILTERING)
    for (__u8 headers_index = 0; headers_index < HTTP2_MAX_PSEUDO_HEADERS_COUNT_FOR_FILTERING; ++headers_index) {
        if (skb_info->data_off >= end) {
            break;
        }
        bpf_skb_load_bytes(skb, skb_info->data_off, &current_ch, sizeof(current_ch));
        skb_info->data_off++;

        is_indexed = (current_ch & 128) != 0;
        is_literal = (current_ch & 192) == 64;
        // If all (is_indexed, is_literal, is_dynamic_table_update) are false, then we
        // have a literal header field without indexing (prefix 0000) or literal header field never indexed (prefix 0001).

        max_bits = MAX_4_BITS;
        // If we're in an indexed header - the max bits are 7.
        max_bits = is_indexed ? MAX_7_BITS : max_bits;
        // else, if we're in a literal header - the max bits are 6.
        max_bits = is_literal ? MAX_6_BITS : max_bits;
        // otherwise, we're in literal header without indexing or literal header never indexed - and for both, the
        // max bits are 4.

        index = 0;
        if (!read_hpack_int_with_given_current_char(skb, skb_info, current_ch, max_bits, &index)) {
            break;
        }

        current_header = NULL;
        if (interesting_headers < HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING) {
            current_header = &headers_to_process[interesting_headers];
        }

        if (is_indexed) {
            // Indexed representation.
            // MSB bit set.
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.1
            parse_field_indexed(dynamic_index, current_header, index, *global_dynamic_counter, &interesting_headers);
            continue;
        }
        // Increment the global dynamic counter for each literal header field.
        // We're not increasing the counter for literal without indexing or literal never indexed.
        __sync_fetch_and_add(global_dynamic_counter, is_literal);
        // 6.2.1 Literal Header Field with Incremental Indexing
        // top two bits are 11
        // https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.1
        if (!parse_field_literal(skb, skb_info, current_header, index, *global_dynamic_counter, &interesting_headers, http2_tel, is_literal)) {
            break;
        }
    }

#pragma unroll(HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING)
    for (__u8 headers_index = 0; headers_index < HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING; ++headers_index) {
        if (skb_info->data_off >= end) {
            break;
        }

        bpf_skb_load_bytes(skb, skb_info->data_off, &current_ch, sizeof(current_ch));
        skb_info->data_off++;

        is_indexed = (current_ch & 128) != 0;
        is_literal = (current_ch & 192) == 64;
        // If all (is_indexed, is_literal, is_dynamic_table_update) are false, then we
        // have a literal header field without indexing (prefix 0000) or literal header field never indexed (prefix 0001).

        max_bits = MAX_4_BITS;
        // If we're in an indexed header - the max bits are 7.
        max_bits = is_indexed ? MAX_7_BITS : max_bits;
        // else, if we're in a literal header - the max bits are 6.
        max_bits = is_literal ? MAX_6_BITS : max_bits;
        // otherwise, we're in literal header without indexing or literal header never indexed - and for both, the
        // max bits are 4.

        index = 0;
        if (!read_hpack_int_with_given_current_char(skb, skb_info, current_ch, max_bits, &index)) {
            break;
        }

        if (is_indexed) {
            // Indexed representation.
            // MSB bit set.
            // https://httpwg.org/specs/rfc7541.html#rfc.section.6.1
            continue;
        }
        // Increment the global dynamic counter for each literal header field.
        // We're not increasing the counter for literal without indexing or literal never indexed.
        __sync_fetch_and_add(global_dynamic_counter, is_literal);
        // Handle frame headers which are not pseudo headers fields.
        if (!handle_non_pseudo_headers(skb, skb_info, index)){
            break;
        }
    }

    return interesting_headers;
}

// process_headers processes the headers that were filtered in filter_relevant_headers,
// looking for requests path, status code, and method.
static __always_inline void process_headers(struct __sk_buff *skb, dynamic_table_index_t *dynamic_index, http2_stream_t *current_stream, http2_header_t *headers_to_process, __u8 interesting_headers,  http2_telemetry_t *http2_tel) {
    http2_header_t *current_header;
    dynamic_table_entry_t dynamic_value = {};

#pragma unroll(HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING)
    for (__u8 iteration = 0; iteration < HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING; ++iteration) {
        if (iteration >= interesting_headers) {
            break;
        }

        current_header = &headers_to_process[iteration];

        if (current_header->type == kStaticHeader) {
            if (current_header->index == kPOST || current_header->index == kGET) {
                // TODO: mark request
                current_stream->request_started = bpf_ktime_get_ns();
                current_stream->request_method = current_header->index;
                __sync_fetch_and_add(&http2_tel->request_seen, 1);
            } else if (current_header->index >= k200 && current_header->index <= k500) {
                current_stream->response_status_code = current_header->index;
                __sync_fetch_and_add(&http2_tel->response_seen, 1);
            } else if (current_header->index == kEmptyPath) {
                current_stream->path_size = HTTP2_ROOT_PATH_LEN;
                bpf_memcpy(current_stream->request_path, HTTP2_ROOT_PATH, HTTP2_ROOT_PATH_LEN);
            } else if (current_header->index == kIndexPath) {
                current_stream->path_size = HTTP2_INDEX_PATH_LEN;
                bpf_memcpy(current_stream->request_path, HTTP2_INDEX_PATH, HTTP2_INDEX_PATH_LEN);
            }
            continue;
        }

        dynamic_index->index = current_header->index;
        if (current_header->type == kExistingDynamicHeader) {
            dynamic_table_entry_t *dynamic_value = bpf_map_lookup_elem(&http2_dynamic_table, dynamic_index);
            if (dynamic_value == NULL) {
                break;
            }
            current_stream->path_size = dynamic_value->string_len;
            current_stream->is_huffman_encoded = dynamic_value->is_huffman_encoded;
            bpf_memcpy(current_stream->request_path, dynamic_value->buffer, HTTP2_MAX_PATH_LEN);
        } else {
            // create the new dynamic value which will be added to the internal table.
            read_into_buffer_path(dynamic_value.buffer, skb, current_header->new_dynamic_value_offset);
            // If the value is indexed - add it to the dynamic table.
            if (current_header->type == kNewDynamicHeader) {
                dynamic_value.string_len = current_header->new_dynamic_value_size;
                dynamic_value.is_huffman_encoded = current_header->is_huffman_encoded;
                bpf_map_update_elem(&http2_dynamic_table, dynamic_index, &dynamic_value, BPF_ANY);
            }
            current_stream->path_size = current_header->new_dynamic_value_size;
            current_stream->is_huffman_encoded = current_header->is_huffman_encoded;
            bpf_memcpy(current_stream->request_path, dynamic_value.buffer, HTTP2_MAX_PATH_LEN);
        }
    }
}

static __always_inline void process_headers_frame(struct __sk_buff *skb, http2_stream_t *current_stream, skb_info_t *skb_info, conn_tuple_t *tup, dynamic_table_index_t *dynamic_index, http2_frame_t *current_frame_header, http2_telemetry_t *http2_tel) {
    const __u32 zero = 0;

    // Allocating an array of headers, to hold all interesting headers from the frame.
    http2_header_t *headers_to_process = bpf_map_lookup_elem(&http2_headers_to_process, &zero);
    if (headers_to_process == NULL) {
        return;
    }
    bpf_memset(headers_to_process, 0, HTTP2_MAX_HEADERS_COUNT_FOR_PROCESSING * sizeof(http2_header_t));

    __u8 interesting_headers = filter_relevant_headers(skb, skb_info, tup, dynamic_index, headers_to_process, current_frame_header->length, http2_tel);
    process_headers(skb, dynamic_index, current_stream, headers_to_process, interesting_headers, http2_tel);
}

// skip_preface is a helper function to check for the HTTP2 magic sent at the beginning
// of an HTTP2 connection, and skip it if present.
static __always_inline void skip_preface(struct __sk_buff *skb, skb_info_t *skb_info) {
    char preface[HTTP2_MARKER_SIZE];
    bpf_memset((char *)preface, 0, HTTP2_MARKER_SIZE);
    bpf_skb_load_bytes(skb, skb_info->data_off, preface, HTTP2_MARKER_SIZE);
    if (is_http2_preface(preface, HTTP2_MARKER_SIZE)) {
        skb_info->data_off += HTTP2_MARKER_SIZE;
    }
}

// The function is trying to read the remaining of a split frame header. We have the first part in
// `frame_state->buf` (from the previous packet), and now we're trying to read the remaining (`frame_state->remainder`
// bytes from the current packet).
static __always_inline void fix_header_frame(struct __sk_buff *skb, skb_info_t *skb_info, char *out, frame_header_remainder_t *frame_state) {
    bpf_memcpy(out, frame_state->buf, HTTP2_FRAME_HEADER_SIZE);
    // Verifier is unhappy with a single call to `bpf_skb_load_bytes` with a variable length (although checking boundaries)
    switch (frame_state->remainder) {
    case 1:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 1, 1);
        break;
    case 2:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 2, 2);
        break;
    case 3:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 3, 3);
        break;
    case 4:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 4, 4);
        break;
    case 5:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 5, 5);
        break;
    case 6:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 6, 6);
        break;
    case 7:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 7, 7);
        break;
    case 8:
        bpf_skb_load_bytes(skb, skb_info->data_off, out + HTTP2_FRAME_HEADER_SIZE - 8, 8);
        break;
    }
    return;
}

static __always_inline bool get_first_frame(struct __sk_buff *skb, skb_info_t *skb_info, frame_header_remainder_t *frame_state, http2_frame_t *current_frame, http2_telemetry_t *http2_tel) {
    // No state, try reading a frame.
    if (frame_state == NULL) {
        // Checking we have enough bytes in the packet to read a frame header.
        if (skb_info->data_off + HTTP2_FRAME_HEADER_SIZE > skb_info->data_end) {
            // Not enough bytes, cannot read frame, so we have 0 interesting frames in that packet.
            return false;
        }

        // Reading frame, and ensuring the frame is valid.
        bpf_skb_load_bytes(skb, skb_info->data_off, (char *)current_frame, HTTP2_FRAME_HEADER_SIZE);
        skb_info->data_off += HTTP2_FRAME_HEADER_SIZE;
        if (!format_http2_frame_header(current_frame)) {
            // Frame is not valid, so we have 0 interesting frames in that packet.
            return false;
        }
        return true;
    }

    // Getting here means we have a frame state from the previous packets.
    // Scenarios in order:
    //  1. Check if we have a frame-header remainder - if so, we must try and read the rest of the frame header.
    //     In case of a failure, we abort.
    //  2. If we don't have a frame-header remainder, then we're trying to read a valid frame.
    //     HTTP2 can send valid frames (like SETTINGS and PING) during a split DATA frame. If such a frame exists,
    //     then we won't have the rest of the split frame in the same packet.
    //  3. If we reached here, and we have a remainder, then we're consuming the remainder and checking we can read the
    //     next frame header.
    //  4. We failed reading any frame. Aborting.

    // Frame-header-remainder.
    if (frame_state->header_length > 0) {
        fix_header_frame(skb, skb_info, (char*)current_frame, frame_state);
        if (format_http2_frame_header(current_frame)) {
            skb_info->data_off += frame_state->remainder;
            frame_state->remainder = 0;
            return true;
        }

        // We couldn't read frame header using the remainder.
        return false;
    }

    // Checking if we can read a frame header.
    if (skb_info->data_off + HTTP2_FRAME_HEADER_SIZE <= skb_info->data_end) {
        bpf_skb_load_bytes(skb, skb_info->data_off, (char *)current_frame, HTTP2_FRAME_HEADER_SIZE);
        if (format_http2_frame_header(current_frame)) {
            // We successfully read a valid frame.
            skb_info->data_off += HTTP2_FRAME_HEADER_SIZE;
            return true;
        }
    }

    // We failed to read a frame, if we have a remainder trying to consume it and read the following frame.
    if (frame_state->remainder > 0) {
        skb_info->data_off += frame_state->remainder;
        // The remainders "ends" the current packet. No interesting frames were found.
        if (skb_info->data_off == skb_info->data_end) {
            frame_state->remainder = 0;
            return false;
        }
        reset_frame(current_frame);
        bpf_skb_load_bytes(skb, skb_info->data_off, (char *)current_frame, HTTP2_FRAME_HEADER_SIZE);
        if (format_http2_frame_header(current_frame)) {
            frame_state->remainder = 0;
            skb_info->data_off += HTTP2_FRAME_HEADER_SIZE;
            return true;
        }
    }
    // still not valid / does not have a remainder - abort.
    return false;
}

// find_relevant_frames iterates over the packet and finds frames that are
// relevant for us. The frames info and location are stored in the `iteration_value->frames_array` array,
// and the number of frames found is being stored at iteration_value->frames_count.
// This function returns true if there are more frames to filter and if the number of frames found is less than
// HTTP2_MAX_FRAMES_ITERATIONS. This indicates that there are additional frames to filter, allowing parsing frames by
// the next tail call. If false is returned, the subsequent tail call should not be executed.
//
// We consider frames as relevant if they are either:
// - HEADERS frames
// - RST_STREAM frames
// - DATA frames with the END_STREAM flag set
static __always_inline bool find_relevant_frames(struct __sk_buff *skb, skb_info_t *skb_info, http2_tail_call_state_t *iteration_value, http2_telemetry_t *http2_tel) {
    bool is_headers_or_rst_frame, is_data_end_of_stream;
    http2_frame_t current_frame = {};

    // if we already processed part of the packet, we should start from the last offset we processed.
    if (iteration_value->filter_iterations != 0) {
        skb_info->data_off = iteration_value->data_off;
    }

   // If we have found enough interesting frames, we should not process any new frame.
   // The value of iteration_value->frames_count may potentially be greater than 0.
   // It's essential to validate that this increase doesn't surpass the maximum number of frames we can process.
   if (iteration_value->frames_count >= HTTP2_MAX_FRAMES_ITERATIONS) {
       return false;
   }

    __u32 iteration = 0;
#pragma unroll(HTTP2_MAX_FRAMES_TO_FILTER)
    for (; iteration < HTTP2_MAX_FRAMES_TO_FILTER; ++iteration) {
        // Checking we can read HTTP2_FRAME_HEADER_SIZE from the skb.
        if (skb_info->data_off + HTTP2_FRAME_HEADER_SIZE > skb_info->data_end) {
            break;
        }

        bpf_skb_load_bytes(skb, skb_info->data_off, (char *)&current_frame, HTTP2_FRAME_HEADER_SIZE);
        skb_info->data_off += HTTP2_FRAME_HEADER_SIZE;
        if (!format_http2_frame_header(&current_frame)) {
            break;
        }

        // END_STREAM can appear only in Headers and Data frames.
        // Check out https://datatracker.ietf.org/doc/html/rfc7540#section-6.1 for data frame, and
        // https://datatracker.ietf.org/doc/html/rfc7540#section-6.2 for headers frame.
        is_headers_or_rst_frame = current_frame.type == kHeadersFrame || current_frame.type == kRSTStreamFrame;
        is_data_end_of_stream = ((current_frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM) && (current_frame.type == kDataFrame);
        if (iteration_value->frames_count < HTTP2_MAX_FRAMES_ITERATIONS && (is_headers_or_rst_frame || is_data_end_of_stream)) {
            iteration_value->frames_array[iteration_value->frames_count].frame = current_frame;
            iteration_value->frames_array[iteration_value->frames_count].offset = skb_info->data_off;
            iteration_value->frames_count++;
        }
        skb_info->data_off += current_frame.length;

        // If we have found enough interesting frames, we can stop iterating.
        if (iteration_value->frames_count >= HTTP2_MAX_FRAMES_ITERATIONS) {
            break;
        }
    }

    if (iteration_value->frames_count == HTTP2_MAX_FRAMES_ITERATIONS) {
        __sync_fetch_and_add(&http2_tel->exceeding_max_interesting_frames, 1);
    }

    // This function returns true if there are more frames to filter, which will be parsed by the next tail call,
    // and if we have not yet reached the maximum number of frames we can process.
    return (((iteration == HTTP2_MAX_FRAMES_TO_FILTER) &&
            (skb_info->data_off + HTTP2_FRAME_HEADER_SIZE <= skb_info->data_end))&&
            iteration_value->frames_count < HTTP2_MAX_FRAMES_ITERATIONS);
}

SEC("socket/http2_handle_first_frame")
int socket__http2_handle_first_frame(struct __sk_buff *skb) {
    const __u32 zero = 0;
    http2_frame_t current_frame = {};

    dispatcher_arguments_t dispatcher_args_copy;
    bpf_memset(&dispatcher_args_copy, 0, sizeof(dispatcher_arguments_t));
    // We're not calling fetch_dispatching_arguments as, we need to modify the `data_off` field of skb_info, so
    // the next prog will start to read from the next valid frame.
    dispatcher_arguments_t *args = bpf_map_lookup_elem(&dispatcher_arguments, &zero);
    if (args == NULL) {
        return false;
    }
    bpf_memcpy(&dispatcher_args_copy.tup, &args->tup, sizeof(conn_tuple_t));
    bpf_memcpy(&dispatcher_args_copy.skb_info, &args->skb_info, sizeof(skb_info_t));

    // If we detected a tcp termination we should stop processing the packet, and clear its dynamic table by deleting the counter.
    if (is_tcp_termination(&dispatcher_args_copy.skb_info)) {
        // Deleting the entry for the original tuple.
        bpf_map_delete_elem(&http2_dynamic_counter_table, &dispatcher_args_copy.tup);
        terminated_http2_batch_enqueue(&dispatcher_args_copy.tup);
        // In case of local host, the protocol will be deleted for both (client->server) and (server->client),
        // so we won't reach for that path again in the code, so we're deleting the opposite side as well.
        flip_tuple(&dispatcher_args_copy.tup);
        bpf_map_delete_elem(&http2_dynamic_counter_table, &dispatcher_args_copy.tup);
        return 0;
    }

    // A single packet can contain multiple HTTP/2 frames, due to instruction limitations we have divided the
    // processing into multiple tail calls, where each tail call process a single frame. We must have context when
    // we are processing the frames, for example, to know how many bytes have we read in the packet, or it we reached
    // to the maximum number of frames we can process. For that we are checking if the iteration context already exists.
    // If not, creating a new one to be used for further processing
    http2_tail_call_state_t *iteration_value = bpf_map_lookup_elem(&http2_frames_to_process, &zero);
    if (iteration_value == NULL) {
        return 0;
    }
    iteration_value->frames_count = 0;
    iteration_value->iteration = 0;
    iteration_value->filter_iterations = 0;
    iteration_value->data_off = 0;

    // skip HTTP2 magic, if present
    skip_preface(skb, &dispatcher_args_copy.skb_info);

    frame_header_remainder_t *frame_state = bpf_map_lookup_elem(&http2_remainder, &dispatcher_args_copy.tup);

    http2_telemetry_t *http2_tel = bpf_map_lookup_elem(&http2_telemetry, &zero);
    if (http2_tel == NULL) {
        return 0;
    }

    // A case where we read an interesting valid frame header in the previous call, and now we're trying to read the
    // rest of the frame payload. But, since we already read a valid frame, we just fill it as an interesting frame,
    // and continue to the next tail call.
    if (frame_state != NULL && frame_state->header_length == HTTP2_FRAME_HEADER_SIZE) {
        // Copy the cached frame header to the current frame.
        bpf_memcpy((char *)&current_frame, frame_state->buf, HTTP2_FRAME_HEADER_SIZE);
        // Delete the cached frame header.
        bpf_map_delete_elem(&http2_remainder, &dispatcher_args_copy.tup);
        // Save the frame as an interesting frame (a.k.a, restoring the state we had in the previous call).
        // We need to do so, as we're zeroing the iteration_value at the beginning of this function.
        iteration_value->frames_array[0].frame = current_frame;
        iteration_value->frames_array[0].offset = 0;
        iteration_value->frames_count = 1;
        // Continuing to the next tail call.
        bpf_tail_call_compat(skb, &protocols_progs, PROG_HTTP2_FRAME_FILTER);
    }
    if (!get_first_frame(skb, &dispatcher_args_copy.skb_info, frame_state, &current_frame, http2_tel)) {
        return 0;
    }

    // If we have a state and we consumed it, then delete it.
    if (frame_state != NULL && frame_state->remainder == 0) {
        bpf_map_delete_elem(&http2_remainder, &dispatcher_args_copy.tup);
    }

    bool is_headers_or_rst_frame = current_frame.type == kHeadersFrame || current_frame.type == kRSTStreamFrame;
    bool is_data_end_of_stream = ((current_frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM) && (current_frame.type == kDataFrame);
    if (is_headers_or_rst_frame || is_data_end_of_stream) {
        iteration_value->frames_array[0].frame = current_frame;
        iteration_value->frames_array[0].offset = dispatcher_args_copy.skb_info.data_off;
        iteration_value->frames_count = 1;
    }
    dispatcher_args_copy.skb_info.data_off += current_frame.length;
    // We're exceeding the packet boundaries, so we have a remainder.
    if (dispatcher_args_copy.skb_info.data_off > dispatcher_args_copy.skb_info.data_end) {
        frame_header_remainder_t new_frame_state = { 0 };

        // Saving the remainder.
        new_frame_state.remainder = dispatcher_args_copy.skb_info.data_off - dispatcher_args_copy.skb_info.data_end;
        // We did find an interesting frame (as frames_count == 1), so we cache the current frame and waiting for the
        // next call.
        if (iteration_value->frames_count == 1) {
            new_frame_state.header_length = HTTP2_FRAME_HEADER_SIZE;
            bpf_memcpy(new_frame_state.buf, (char *)&current_frame, HTTP2_FRAME_HEADER_SIZE);
        }

        bpf_map_update_elem(&http2_remainder, &dispatcher_args_copy.tup, &new_frame_state, BPF_ANY);
        // Not calling the next tail call as we have nothing to process.
        return 0;
    }
    // Overriding the data_off field of the cached skb_info. The next prog will start from the offset of the next valid
    // frame.
    args->skb_info.data_off = dispatcher_args_copy.skb_info.data_off;

    bpf_tail_call_compat(skb, &protocols_progs, PROG_HTTP2_FRAME_FILTER);
    return 0;
}

SEC("socket/http2_filter")
int socket__http2_filter(struct __sk_buff *skb) {
    dispatcher_arguments_t dispatcher_args_copy;
    bpf_memset(&dispatcher_args_copy, 0, sizeof(dispatcher_arguments_t));
    if (!fetch_dispatching_arguments(&dispatcher_args_copy.tup, &dispatcher_args_copy.skb_info)) {
        return 0;
    }

    const __u32 zero = 0;

    // A single packet can contain multiple HTTP/2 frames, due to instruction limitations we have divided the
    // processing into multiple tail calls, where each tail call process a single frame. We must have context when
    // we are processing the frames, for example, to know how many bytes have we read in the packet, or it we reached
    // to the maximum number of frames we can process. For that we are checking if the iteration context already exists.
    // If not, creating a new one to be used for further processing
    http2_tail_call_state_t *iteration_value = bpf_map_lookup_elem(&http2_frames_to_process, &zero);
    if (iteration_value == NULL) {
        return 0;
    }

    http2_telemetry_t *http2_tel = bpf_map_lookup_elem(&http2_telemetry, &zero);
    if (http2_tel == NULL) {
        return 0;
    }

    // Some functions might change and override fields in dispatcher_args_copy.skb_info. Since it is used as a key
    // in a map, we cannot allow it to be modified. Thus, having a local copy of skb_info.
    skb_info_t local_skb_info = dispatcher_args_copy.skb_info;

    bool have_more_frames_to_process = find_relevant_frames(skb, &local_skb_info, iteration_value, http2_tel);
    // We have found there are more frames to filter, so we will call frame_filter again.
    // Max current amount of tail calls would be 2, which will allow us to currently parse
    // HTTP2_MAX_TAIL_CALLS_FOR_FRAMES_FILTER*HTTP2_MAX_FRAMES_ITERATIONS.
    iteration_value->filter_iterations++;
    if (have_more_frames_to_process && iteration_value->filter_iterations < HTTP2_MAX_TAIL_CALLS_FOR_FRAMES_FILTER) {
        // save local copy of the skb_info, so the next prog will start from the offset of the next valid frame.
        iteration_value->data_off = local_skb_info.data_off;
        bpf_tail_call_compat(skb, &protocols_progs, PROG_HTTP2_FRAME_FILTER);
    }

    // if we left with more headers to process and we reached the max amount of tail calls we should update the telemetry.
    if (have_more_frames_to_process && iteration_value->filter_iterations >= HTTP2_MAX_TAIL_CALLS_FOR_FRAMES_FILTER) {
        __sync_fetch_and_add(&http2_tel->exceeding_max_frames_to_filter, 1);
    }

    frame_header_remainder_t new_frame_state = { 0 };
    if (local_skb_info.data_off > local_skb_info.data_end) {
        // We have a remainder
        new_frame_state.remainder = local_skb_info.data_off - local_skb_info.data_end;
        bpf_map_update_elem(&http2_remainder, &dispatcher_args_copy.tup, &new_frame_state, BPF_ANY);
    } else if (local_skb_info.data_off < local_skb_info.data_end && local_skb_info.data_off + HTTP2_FRAME_HEADER_SIZE > local_skb_info.data_end) {
        // We have a frame header remainder
        new_frame_state.remainder = HTTP2_FRAME_HEADER_SIZE - (local_skb_info.data_end - local_skb_info.data_off);
        bpf_memset(new_frame_state.buf, 0, HTTP2_FRAME_HEADER_SIZE);
    #pragma unroll(HTTP2_FRAME_HEADER_SIZE)
        for (__u32 iteration = 0; iteration < HTTP2_FRAME_HEADER_SIZE && new_frame_state.remainder + iteration < HTTP2_FRAME_HEADER_SIZE; ++iteration) {
            bpf_skb_load_bytes(skb, local_skb_info.data_off + iteration, new_frame_state.buf + iteration, 1);
        }
        new_frame_state.header_length = HTTP2_FRAME_HEADER_SIZE - new_frame_state.remainder;
        bpf_map_update_elem(&http2_remainder, &dispatcher_args_copy.tup, &new_frame_state, BPF_ANY);
    }

    if (iteration_value->frames_count == 0) {
        return 0;
    }

    // We have couple of interesting headers, launching tail calls to handle them.
    if (bpf_map_update_elem(&http2_iterations, &dispatcher_args_copy, iteration_value, BPF_NOEXIST) >= 0) {
        // We managed to cache the iteration_value in the http2_iterations map.
        bpf_tail_call_compat(skb, &protocols_progs, PROG_HTTP2_HEADERS_PARSER);
    }

    return 0;
}

// The program is responsible for parsing all headers frames. For each headers frame we parse the headers,
// fill the dynamic table with the new interesting literal headers, and modifying the streams accordingly.
// The program can be called multiple times (via "self call" of tail calls) in case we have more frames to parse
// than the maximum number of frames we can process in a single tail call.
// The program is being called after socket__http2_filter, and it is being called only if we have interesting frames.
// The program calls socket__http2_eos_parser to finalize the streams and enqueue them to be sent to the user mode.
SEC("socket/http2_headers_parser")
int socket__http2_headers_parser(struct __sk_buff *skb) {
    dispatcher_arguments_t dispatcher_args_copy;
    bpf_memset(&dispatcher_args_copy, 0, sizeof(dispatcher_arguments_t));
    if (!fetch_dispatching_arguments(&dispatcher_args_copy.tup, &dispatcher_args_copy.skb_info)) {
        return 0;
    }

    // Some functions might change and override data_off field in dispatcher_args_copy.skb_info. Since it is used as a key
    // in a map, we cannot allow it to be modified. Thus, storing the original value of the offset.
    __u32 original_off = dispatcher_args_copy.skb_info.data_off;

    // A single packet can contain multiple HTTP/2 frames, due to instruction limitations we have divided the
    // processing into multiple tail calls, where each tail call process a single frame. We must have context when
    // we are processing the frames, for example, to know how many bytes have we read in the packet, or it we reached
    // to the maximum number of frames we can process. For that we are checking if the iteration context already exists.
    // If not, creating a new one to be used for further processing
    http2_tail_call_state_t *tail_call_state = bpf_map_lookup_elem(&http2_iterations, &dispatcher_args_copy);
    if (tail_call_state == NULL) {
        // We didn't find the cached context, aborting.
        return 0;
    }

    const __u32 zero = 0;
    http2_ctx_t *http2_ctx = bpf_map_lookup_elem(&http2_ctx_heap, &zero);
    if (http2_ctx == NULL) {
        goto delete_iteration;
    }

    http2_telemetry_t *http2_tel = bpf_map_lookup_elem(&http2_telemetry, &zero);
    if (http2_tel == NULL) {
        goto delete_iteration;
    }

    http2_frame_with_offset *frames_array = tail_call_state->frames_array;
    http2_frame_with_offset current_frame;

    // create the http2 ctx for the current http2 frame.
    bpf_memset(http2_ctx, 0, sizeof(http2_ctx_t));
    http2_ctx->http2_stream_key.tup = dispatcher_args_copy.tup;
    normalize_tuple(&http2_ctx->http2_stream_key.tup);
    http2_ctx->dynamic_index.tup = dispatcher_args_copy.tup;

    http2_stream_t *current_stream = NULL;

    #pragma unroll(HTTP2_MAX_FRAMES_FOR_HEADERS_PARSER_PER_TAIL_CALL)
    for (__u16 index = 0; index < HTTP2_MAX_FRAMES_FOR_HEADERS_PARSER_PER_TAIL_CALL; index++) {
        if (tail_call_state->iteration >= tail_call_state->frames_count) {
            break;
        }
        // This check must be next to the access of the array, otherwise the verifier will complain.
        if (tail_call_state->iteration >= HTTP2_MAX_FRAMES_ITERATIONS) {
            break;
        }
        current_frame = frames_array[tail_call_state->iteration];
        tail_call_state->iteration += 1;

        if (current_frame.frame.type != kHeadersFrame) {
            continue;
        }

        http2_ctx->http2_stream_key.stream_id = current_frame.frame.stream_id;
        current_stream = http2_fetch_stream(&http2_ctx->http2_stream_key);
        if (current_stream == NULL) {
            continue;
        }
        dispatcher_args_copy.skb_info.data_off = current_frame.offset;
        process_headers_frame(skb, current_stream, &dispatcher_args_copy.skb_info, &dispatcher_args_copy.tup, &http2_ctx->dynamic_index, &current_frame.frame, http2_tel);
    }

    if (tail_call_state->iteration < HTTP2_MAX_FRAMES_ITERATIONS &&
        tail_call_state->iteration < tail_call_state->frames_count &&
        tail_call_state->iteration < HTTP2_MAX_FRAMES_FOR_HEADERS_PARSER) {
        bpf_tail_call_compat(skb, &protocols_progs, PROG_HTTP2_HEADERS_PARSER);
    }
    // Zeroing the iteration index to call EOS parser
    tail_call_state->iteration = 0;
    bpf_tail_call_compat(skb, &protocols_progs, PROG_HTTP2_EOS_PARSER);

delete_iteration:
    // restoring the original value.
    dispatcher_args_copy.skb_info.data_off = original_off;
    bpf_map_delete_elem(&http2_iterations, &dispatcher_args_copy);

    return 0;
}

// The program is responsible for parsing all frames that mark the end of a stream.
// We consider a frame as marking the end of a stream if it is either:
//  - An headers or data frame with END_STREAM flag set.
//  - An RST_STREAM frame.
// The program is being called after socket__http2_headers_parser, and it finalizes the streams and enqueue them
// to be sent to the user mode.
// The program is ready to be called multiple times (via "self call" of tail calls) in case we have more frames to
// process than the maximum number of frames we can process in a single tail call.
SEC("socket/http2_eos_parser")
int socket__http2_eos_parser(struct __sk_buff *skb) {
    dispatcher_arguments_t dispatcher_args_copy;
    bpf_memset(&dispatcher_args_copy, 0, sizeof(dispatcher_arguments_t));
    if (!fetch_dispatching_arguments(&dispatcher_args_copy.tup, &dispatcher_args_copy.skb_info)) {
        return 0;
    }

    // A single packet can contain multiple HTTP/2 frames, due to instruction limitations we have divided the
    // processing into multiple tail calls, where each tail call process a single frame. We must have context when
    // we are processing the frames, for example, to know how many bytes have we read in the packet, or it we reached
    // to the maximum number of frames we can process. For that we are checking if the iteration context already exists.
    // If not, creating a new one to be used for further processing
    http2_tail_call_state_t *tail_call_state = bpf_map_lookup_elem(&http2_iterations, &dispatcher_args_copy);
    if (tail_call_state == NULL) {
        // We didn't find the cached context, aborting.
        return 0;
    }

    const __u32 zero = 0;
    http2_telemetry_t *http2_tel = bpf_map_lookup_elem(&http2_telemetry, &zero);
    if (http2_tel == NULL) {
        goto delete_iteration;
    }

    http2_frame_with_offset *frames_array = tail_call_state->frames_array;
    http2_frame_with_offset current_frame;

    http2_ctx_t *http2_ctx = bpf_map_lookup_elem(&http2_ctx_heap, &zero);
    if (http2_ctx == NULL) {
        goto delete_iteration;
    }
    bpf_memset(http2_ctx, 0, sizeof(http2_ctx_t));
    http2_ctx->http2_stream_key.tup = dispatcher_args_copy.tup;
    normalize_tuple(&http2_ctx->http2_stream_key.tup);

    bool is_rst = false, is_end_of_stream = false;
    http2_stream_t *current_stream = NULL;

    #pragma unroll(HTTP2_MAX_FRAMES_FOR_EOS_PARSER_PER_TAIL_CALL)
    for (__u16 index = 0; index < HTTP2_MAX_FRAMES_FOR_EOS_PARSER_PER_TAIL_CALL; index++) {
        if (tail_call_state->iteration >= HTTP2_MAX_FRAMES_ITERATIONS) {
            break;
        }

        current_frame = frames_array[tail_call_state->iteration];
        // Having this condition after assignment and not before is due to a verifier issue.
        if (tail_call_state->iteration >= tail_call_state->frames_count) {
            break;
        }
        tail_call_state->iteration += 1;

        is_rst = current_frame.frame.type == kRSTStreamFrame;
        is_end_of_stream = (current_frame.frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM;
        if (!is_rst && !is_end_of_stream) {
            continue;
        }

        http2_ctx->http2_stream_key.stream_id = current_frame.frame.stream_id;
        current_stream = http2_fetch_stream(&http2_ctx->http2_stream_key);
        if (current_stream == NULL) {
            continue;
        }

        // When we accept an RST, it means that the current stream is terminated.
        // See: https://datatracker.ietf.org/doc/html/rfc7540#section-6.4
        // If rst, and stream is empty (no status code, or no response) then delete from inflight
        if (is_rst && (current_stream->response_status_code == 0 || current_stream->request_started == 0)) {
            bpf_map_delete_elem(&http2_in_flight, &http2_ctx->http2_stream_key);
            continue;
        }

        if (is_rst) {
            __sync_fetch_and_add(&http2_tel->end_of_stream_rst, 1);
        } else if ((current_frame.frame.flags & HTTP2_END_OF_STREAM) == HTTP2_END_OF_STREAM) {
            __sync_fetch_and_add(&http2_tel->end_of_stream, 1);
        }
        handle_end_of_stream(current_stream, &http2_ctx->http2_stream_key, http2_tel);
    }

    if (tail_call_state->iteration < HTTP2_MAX_FRAMES_ITERATIONS &&
        tail_call_state->iteration < tail_call_state->frames_count &&
        tail_call_state->iteration < HTTP2_MAX_FRAMES_FOR_EOS_PARSER) {
        bpf_tail_call_compat(skb, &protocols_progs, PROG_HTTP2_EOS_PARSER);
    }

delete_iteration:
    bpf_map_delete_elem(&http2_iterations, &dispatcher_args_copy);

    return 0;
}
#endif
