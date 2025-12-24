#ifndef __CONNTRACK_MAPS_H
#define __CONNTRACK_MAPS_H

#include "bpf_helpers.h"
#include "map-defs.h"

#include "conn_tuple.h"
#include "conntrack/types.h"

/* JMW update comment to mention both probes thatuse it */
/* This map is used for tracking JMW pending confirmations (pid_tgid -> ct pointer) to allow correlation between kprobe and kretprobe of
 * nf_conntrack_confirm JMW
 */
BPF_HASH_MAP(conntrack_args, u64, u64, 10240) // JMW size?  add config and resize like for conntrack map?

/* This map is used for tracking conntrack entries
 */
BPF_HASH_MAP(conntrack, conntrack_tuple_t, conntrack_tuple_t, 1)

#endif
