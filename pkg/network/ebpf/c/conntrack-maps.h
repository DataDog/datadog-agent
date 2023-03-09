#ifndef __CONNTRACK_MAPS_H
#define __CONNTRACK_MAPS_H

#include "bpf_helpers.h"
#include "map-defs.h"
#include "tracer.h"
#include "conntrack-types.h"

/* This map is used for tracking conntrack entries
 */
BPF_HASH_MAP(conntrack, conntrack_tuple_t, conntrack_tuple_t, 1)

#endif
