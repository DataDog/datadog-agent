#ifndef __CONNTRACK_MAPS_H
#define __CONNTRACK_MAPS_H

#include "bpf_helpers.h"
#include "map-defs.h"
#include "tracer.h"
#include "conntrack-types.h"

/* This map is used for tracking conntrack entries
 */
#ifdef BPF_F_NO_COMMON_LRU
BPF_LRU_MAP(conntrack, conntrack_tuple_t, conntrack_tuple_t, 1024)
#else
BPF_HASH_MAP(conntrack, conntrack_tuple_t, conntrack_tuple_t, 1024)
#endif

/* This map is used for conntrack telemetry in kernelspace
 * only key 0 is used
 * value is a telemetry object
 */
BPF_ARRAY_MAP(conntrack_telemetry, conntrack_telemetry_t, 1)
    
#endif
