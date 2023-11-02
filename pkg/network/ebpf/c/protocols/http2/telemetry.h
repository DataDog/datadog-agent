#ifndef __HTTP2_TELEMETRY_H
#define __HTTP2_TELEMETRY_H

#include "ktypes.h"
#include "maps-defs.h"

enum telemetry_counter
{
    end_of_stream_eos,
    end_of_stream_rst,
//    str_len_greater_then_frame_loc,
    large_path_in_delta,
    large_path_outside_delta,
//    request_seen,
//    response_seen,
//    frame_remainder,
//    max_frames_in_packet,
};

static __always_inline void increment_telemetry_count(enum telemetry_counter counter_name) {
    __u64 zero = 0;
    http2_telemetry_t *http2_tel = bpf_map_lookup_elem(&http2_telemetry, &zero);
    if (http2_tel == NULL) {
        return;
    }

    switch (counter_name) {
    case end_of_stream_eos:
        __sync_fetch_and_add(&http2_tel->end_of_stream_eos, 1);
        break;
    case end_of_stream_rst:
        __sync_fetch_and_add(&http2_tel->end_of_stream_rst, 1);
        break;
//    case str_len_greater_then_frame_loc:
//        __sync_fetch_and_add(&val->str_len_greater_then_frame_loc, 1);
//        break;
    case large_path_in_delta:
        __sync_fetch_and_add(&http2_tel->large_path_in_delta, 1);
        break;
    case large_path_outside_delta:
        __sync_fetch_and_add(&http2_tel->large_path_outside_delta, 1);
        break;
//    case request_seen:
//        __sync_fetch_and_add(&val->request_seen, 1);
//        break;
//    case response_seen:
//        __sync_fetch_and_add(&val->response_seen, 1);
//        break;
//    case frame_remainder:
//        __sync_fetch_and_add(&val->response_seen, 1);
//        break;
//    case max_frames_in_packet:
//        __sync_fetch_and_add(&val->response_seen, 1);
//        break;

    }
}

#endif // __HTTP2_TELEMETRY_H
