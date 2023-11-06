#ifndef __HTTP2_TELEMETRY_H
#define __HTTP2_TELEMETRY_H

#include "ktypes.h"
#include "maps-defs.h"

enum telemetry_counter
{
    END_OF_STREAM_EOS,
    END_OF_STREAM_RST,
    STR_LEN_EXCEEDS_FRAME,
    LARGE_PATH_IN_DELTA,
    LARGE_PATH_OUTSIDE_DELTA,
    REQUEST_SEEN,
    RESPONSE_SEEN,
    FRAMEֹֹ_REMAINDER,
};

static __always_inline void increment_telemetry_count(enum telemetry_counter counter_name) {
    __u64 zero = 0;
    http2_telemetry_t *http2_tel = bpf_map_lookup_elem(&http2_telemetry, &zero);
    if (http2_tel == NULL) {
        return;
    }

    switch (counter_name) {
    case END_OF_STREAM_EOS:
        __sync_fetch_and_add(&http2_tel->end_of_stream_eos, 1);
        break;
    case END_OF_STREAM_RST:
        __sync_fetch_and_add(&http2_tel->end_of_stream_rst, 1);
        break;
    case STR_LEN_EXCEEDS_FRAME:
        __sync_fetch_and_add(&http2_tel->str_len_exceeds_frame, 1);
        break;
    case LARGE_PATH_IN_DELTA:
        __sync_fetch_and_add(&http2_tel->large_path_in_delta, 1);
        break;
    case LARGE_PATH_OUTSIDE_DELTA:
        __sync_fetch_and_add(&http2_tel->large_path_outside_delta, 1);
        break;
    case REQUEST_SEEN:
        __sync_fetch_and_add(&http2_tel->request_seen, 1);
        break;
    case RESPONSE_SEEN:
        __sync_fetch_and_add(&http2_tel->response_seen, 1);
        break;
    case FRAMEֹֹ_REMAINDER:
        __sync_fetch_and_add(&http2_tel->response_seen, 1);
        break;
    }
}

#endif // __HTTP2_TELEMETRY_H
