#ifndef __CONNTRACK_MAPS_H
#define __CONNTRACK_MAPS_H

#include "bpf_helpers.h"
#include "map-defs.h"

#include "conn_tuple.h"
#include "conntrack/types.h"

/* This map is used to hold struct nf_conn * from __nf_conntrack_confirm and
 * nf_conntrack_hash_check_insert kprobes to be used in their respective kretprobes
 */
BPF_HASH_MAP(conntrack_args, u64, struct nf_conn *, 1024)

/* This map is used for tracking conntrack entries
 */
BPF_HASH_MAP(conntrack, conntrack_tuple_t, conntrack_tuple_t, 1)

#endif
