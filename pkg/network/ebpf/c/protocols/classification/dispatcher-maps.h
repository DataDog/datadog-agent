#ifndef __PROTOCOL_DISPATCHER_MAPS_H
#define __PROTOCOL_DISPATCHER_MAPS_H

#include "map-defs.h"

#include "protocols/classification/defs.h"
#include "protocols/classification/shared-tracer-maps.h"

// Maps a connection tuple to latest tcp segment we've processed. Helps to detect same packets that travels multiple
// interfaces or retransmissions.
BPF_HASH_MAP(connection_states, conn_tuple_t, u32, 0)

// Map used to store the sub program actually used by the socket filter.
// This is done to avoid memory limitation when attaching a filter to
// a socket.
// See: https://datadoghq.atlassian.net/wiki/spaces/NET/pages/2326855913/HTTP#Program-size-limit-for-socket-filters
BPF_PROG_ARRAY(protocols_progs, PROG_MAX)

// Map used to store the sub programs responsible for decoding of TLS encrypted
// traffic, after getting plain data from our TLS implementations
BPF_PROG_ARRAY(tls_process_progs, TLS_PROG_MAX)

// This program array is needed to bypass a memory limit on socket filters.
// There is a limitation on number of instructions can be attached to a socket filter,
// as we dispatching more protocols, we reached that limit, thus we workaround it
// by using tail call.
BPF_PROG_ARRAY(dispatcher_classification_progs, DISPATCHER_PROG_MAX)

// A per-cpu array to share conn_tuple and skb_info between the dispatcher and the tail-calls.
BPF_PERCPU_ARRAY_MAP(dispatcher_arguments, __u32, dispatcher_arguments_t, 1)

#endif
