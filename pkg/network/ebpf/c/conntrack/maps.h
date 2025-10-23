#ifndef __CONNTRACK_MAPS_H
#define __CONNTRACK_MAPS_H

#include "bpf_helpers.h"
#include "map-defs.h"

#include "conn_tuple.h"
#include "conntrack/types.h"

/* This map is used for tracking conntrack entries
 */
BPF_HASH_MAP(conntrack, conntrack_tuple_t, conntrack_tuple_t, 1)

/* Second map for tracking NAT packet processing
 */
BPF_HASH_MAP(conntrack2, conntrack_tuple_t, conntrack_tuple_t, 1)

/* Third map for tracking confirmed NAT connections
 */
BPF_HASH_MAP(conntrack3, conntrack_tuple_t, conntrack_tuple_t, 1)

/* Map to track pending confirmations (ct pointer -> dummy value)
 */
BPF_HASH_MAP(pending_confirms, u64, u8, 10240)

#endif
