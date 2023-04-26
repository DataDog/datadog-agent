#ifndef __PROTOCOL_CLASSIFICATION_TRACER_MAPS_H
#define __PROTOCOL_CLASSIFICATION_TRACER_MAPS_H

#include "tracer.h"
#include "map-defs.h"

// Maps skb connection tuple to socket connection tuple.
// On ingress, skb connection tuple is pre NAT, and socket connection tuple is post NAT, and on egress, the opposite.
// We track the lifecycle of socket using tracepoint net/net_dev_queue.
// Some protocol can be classified in a single direction (for example HTTP/2 can be classified only by the first 24 bytes
// sent on the hand shake), and if we have NAT, then the conn tuple we extract from sk_buff will be different than the
// one we extract from the sock object, and then we are not able to correctly classify those protocols.
// To overcome those problems, we save two maps that translates from conn tuple of sk_buff to conn tuple of sock* and vice
// versa (the vice versa is used for cleanup purposes).
BPF_HASH_MAP(conn_tuple_to_socket_skb_conn_tuple, conn_tuple_t, conn_tuple_t, 0)

// Maps a connection tuple to its classified protocol. Used to reduce redundant classification procedures on the same
// connection. Assumption: each connection has a single protocol.
BPF_HASH_MAP(connection_protocol, conn_tuple_t, protocol_t, 0)

// Maps a connection tuple to its classified TLS protocol on socket layer only.
BPF_HASH_MAP(tls_connection, conn_tuple_t, bool, 0)

// This entry point is needed to bypass a memory limit on socket filters.
// There is a limitation on number of instructions can be attached to a socket filter,
// as we classify more protocols, we reached that limit, thus we workaround it
// by using tail call.
BPF_PROG_ARRAY(classification_progs, CLASSIFICATION_PROG_MAX)

#endif
