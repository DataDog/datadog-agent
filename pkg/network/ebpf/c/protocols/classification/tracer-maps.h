#ifndef __PROTOCOL_CLASSIFICATION_TRACER_MAPS_H
#define __PROTOCOL_CLASSIFICATION_TRACER_MAPS_H

#include "conn_tuple.h"
#include "map-defs.h"

#include "protocols/classification/shared-tracer-maps.h"

// Maps skb connection tuple to socket connection tuple.
// On ingress, skb connection tuple is pre NAT, and socket connection tuple is post NAT, and on egress, the opposite.
// We track the lifecycle of socket using tracepoint net/net_dev_queue.
// Some protocol can be classified in a single direction (for example HTTP/2 can be classified only by the first 24 bytes
// sent on the hand shake), and if we have NAT, then the conn tuple we extract from sk_buff will be different than the
// one we extract from the sock object, and then we are not able to correctly classify those protocols.
// To overcome those problems, we save two maps that translates from conn tuple of sk_buff to conn tuple of sock* and vice
// versa (the vice versa is used for cleanup purposes).
BPF_HASH_MAP(conn_tuple_to_socket_skb_conn_tuple, conn_tuple_t, conn_tuple_t, 0)

// Maps a connection tuple to its classified TLS protocol on socket layer only.
BPF_HASH_MAP(tls_connection, conn_tuple_t, bool, 0)

// This entry point is needed to bypass a memory limit on socket filters.
// There is a limitation on number of instructions can be attached to a socket filter,
// as we classify more protocols, we reached that limit, thus we workaround it
// by using tail call.
BPF_PROG_ARRAY(classification_progs, CLASSIFICATION_PROG_MAX)

// Map to hold conn_tuple_t parameter for tcp_close calls
// to be used in kretprobe/tcp_close.
BPF_HASH_MAP(tcp_close_args, __u64, conn_tuple_t, 1024)

// This program array is needed to bypass a memory limit on socket filters.
// There is a limitation on number of instructions can be attached to a socket filter,
// as we dispatching more protocols, we reached that limit, thus we workaround it
// by using tail call.
BPF_PROG_ARRAY(tcp_close_progs, 1)

#endif
