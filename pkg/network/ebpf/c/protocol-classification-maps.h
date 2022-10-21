#ifndef __PROTOCOL_CLASSIFICATION_MAPS_H
#define __PROTOCOL_CLASSIFICATION_MAPS_H

#include "protocol-classification-defs.h"
#include "map-defs.h"

// Maps a connection tuple to its classified protocol. Used to reduce redundant classification procedures on the same
// connection. Assumption: each connection has a single protocol.
BPF_HASH_MAP(connection_protocol, struct sock*, protocol_t, 1024)

// Maps connection tuple before NAT resolving to the socket pointer.
// We track the lifecycle of socket using tracepoint net/net_dev_queue.
BPF_HASH_MAP(conn_tuple_to_socket_map, conn_tuple_t, struct sock*, 1024)

// Maps a connection tuple to latest tcp segment we've processed. Helps to detect same packets that travels multiple
// interfaces or retransmissions.
BPF_HASH_MAP(connection_states, conn_tuple_t, u32, 1024)

#endif
