#ifndef __PROTOCOL_DISPATCHER_MAPS_H
#define __PROTOCOL_DISPATCHER_MAPS_H

#include "protocol-classification-defs.h"
#include "map-defs.h"

// Maps a connection tuple to latest tcp segment we've processed. Helps to detect same packets that travels multiple
// interfaces or retransmissions.
BPF_HASH_MAP(connection_states, conn_tuple_t, u32, 0)

// Maps a connection tuple to its classified protocol. Used to reduce redundant classification procedures on the same
// connection. Assumption: each connection has a single protocol.
BPF_LRU_MAP(dispatcher_connection_protocol, conn_tuple_t, protocol_t, 0)

// Map used to store the sub program actually used by the socket filter.
// This is done to avoid memory limitation when attaching a filter to
// a socket.
// See: https://datadoghq.atlassian.net/wiki/spaces/NET/pages/2326855913/HTTP#Program-size-limit-for-socket-filters
BPF_PROG_ARRAY(protocols_progs, MAX_PROTOCOLS)

#endif
