#ifndef __HTTP2_TELEMETRY_H
#define __HTTP2_TELEMETRY_H

#include "ktypes.h"
#include "maps-defs.h"

enum telemetry_counter
{
    end_of_stream_eos,
    end_of_stream_rst,
    str_len_greater_then_frame_loc,
    str_len_too_big_mid,
    str_len_too_big_large,
    request_seen,
    response_seen,
    frame_remainder,
    max_frames_in_packet,
};

static __always_inline void increment_telemetry_count(enum telemetry_counter counter_name) {
    __u64 key = 0;
    http2_telemetry_t *val = NULL;
    val = bpf_map_lookup_elem(&http2_telemetry, &key);
    if (val == NULL) {
        return;
    }

    switch (counter_name) {
    case end_of_stream_eos:
        __sync_fetch_and_add(&val->end_of_stream_eos, 1);
        break;
    case end_of_stream_rst:
        __sync_fetch_and_add(&val->end_of_stream_rst, 1);
        break;
    case str_len_greater_then_frame_loc:
        __sync_fetch_and_add(&val->str_len_greater_then_frame_loc, 1);
        break;
    case str_len_too_big_mid:
        __sync_fetch_and_add(&val->str_len_too_big_mid, 1);
        break;
    case str_len_too_big_large:
        __sync_fetch_and_add(&val->str_len_too_big_large, 1);
        break;
    case request_seen:
        __sync_fetch_and_add(&val->request_seen, 1);
        break;
    case response_seen:
        __sync_fetch_and_add(&val->response_seen, 1);
        break;
    case frame_remainder:
        __sync_fetch_and_add(&val->response_seen, 1);
        break;
    case max_frames_in_packet:
        __sync_fetch_and_add(&val->response_seen, 1);
        break;

    }
}

#endif // __HTTP2_TELEMETRY_H
