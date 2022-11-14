#ifndef __PROTOCOL_CLASSIFICATION_MAPS_H
#define __PROTOCOL_CLASSIFICATION_MAPS_H

#include "protocol-classification-defs.h"
#include "map-defs.h"

// Maps a connection tuple to its classified protocol. Used to reduce redundant classification procedures on the same
// connection. Assumption: each connection has a single protocol.
BPF_HASH_MAP(connection_protocol, conn_tuple_t, protocol_t, 1024)

// Maps skb connection tuple to socket connection tuple.
// On ingress, skb connection tuple is pre NAT, and socket connection tuple is post NAT, and on egress, the opposite.
// We track the lifecycle of socket using tracepoint net/net_dev_queue.
// Some protocol can be classified in a single direction (for example HTTP/2 can be classified only by the first 24 bytes
// sent on the hand shake), and if we have NAT, then the conn tuple we extract from sk_buff will be different than the
// one we extract from the sock object, and then we are not able to correctly classify those protocols.
// To overcome those problems, we save two maps that translates from conn tuple of sk_buff to conn tuple of sock* and vice
// versa (the vice versa is used for cleanup purposes).
BPF_HASH_MAP(conn_tuple_to_socket_skb_conn_tuple, conn_tuple_t, conn_tuple_t, 1024)

// Maps a connection tuple to latest tcp segment we've processed. Helps to detect same packets that travels multiple
// interfaces or retransmissions.
BPF_HASH_MAP(connection_states, conn_tuple_t, u32, 1024)

#endif
