#ifndef __TIMESTAMP_MS__
#define __TIMESTAMP_MS__

#include "ktypes.h"
#include "bpf_helpers.h"

#include "defs.h"
#include "tracer/tracer.h"

// 48-bit limit for an integer
#define TIME_MS_LIMIT (((__u64) 1 << 48) - 1)

// convert_ns_to_ms converts a 64-bit nanosecond timestamp into a 48-bit millisecond timestamp
static __always_inline time_ms_t convert_ns_to_ms(__u64 timestamp) {
    __u64 ms = timestamp / 1000;
    if (ms > TIME_MS_LIMIT) {
        ms = 0;
    }

    time_ms_t t = {0};
    for (int i = 2; i >= 0; i--) {
        t.timestamp[i] = ms & 0xffff;
        ms >>= 16;
    }

    return t;
}

// convert_ns_to_ms converts a 48-bit millisecond timestamp into a 64-bit nanosecond timestamp
static __always_inline __u64 convert_ms_to_ns(time_ms_t t) {
    __u64 ms = 0;
    for (int i = 0; i < 3; i++) {
        ms <<= 16;
        ms += t.timestamp[i];
    }

    return ms * 1000;
}

#endif // __TIMESTAMP_MS__
