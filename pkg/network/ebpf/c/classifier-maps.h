#ifndef __CLASSIFIER_MAPS_H
#define __CLASSIFIER_MAPS_H

#include "tracer.h"
#include "bpf_helpers.h"

/* This map is used for telemetry in kernelspace
 * only key 0 is used
 * value is a telemetry object
 */
struct bpf_map_def SEC("maps/classifier_telemetry") classifier_telemetry = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(classifier_telemetry_t),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

#endif
