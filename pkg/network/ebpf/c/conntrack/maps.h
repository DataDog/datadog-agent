#ifndef __CONNTRACK_MAPS_H
#define __CONNTRACK_MAPS_H

#include "bpf_helpers.h"
#include "map-defs.h"

#include "conn_tuple.h"
#include "conntrack/types.h"

/* This map is used for tracking conntrack entries
 */
BPF_HASH_MAP(conntrack, conntrack_tuple_t, conntrack_tuple_t, 1)

/* Map to track pending confirmations (pid_tgid -> ct pointer) JMW
 */
BPF_HASH_MAP(nf_conntrack_confirm_args, u64, u64, 10240) // JMW size?  add config and resize like for conntrack map?

#endif
