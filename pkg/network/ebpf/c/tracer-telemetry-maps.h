#ifndef __TRACER_TELEMETRY_MAPS_H
#define __TRACER_TELEMETRY_MAPS_H

#include "tracer.h"
#include "bpf_helpers.h"

/* This map is used for telemetry in kernelspace
 * only key 0 is used
 * value is a telemetry object
 */
struct bpf_map_def SEC("maps/telemetry") telemetry = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(telemetry_t),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

#endif
