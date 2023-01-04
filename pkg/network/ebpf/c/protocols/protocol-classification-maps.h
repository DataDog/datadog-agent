#ifndef __PROTOCOL_CLASSIFICATION_MAPS_H
#define __PROTOCOL_CLASSIFICATION_MAPS_H

#include "protocol-classification-defs.h"
#include "protocol-classification-structs.h"
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

// Kernels before 4.7 do not know about per-cpu array maps.
#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 7, 0)

// A per-cpu buffer used to read requests fragments during protocol
// classification and avoid allocating a buffer on the stack. Some protocols
// requires us to read at offset that are not aligned. Such reads are forbidden
// if done on the stack and will make the verifier complain about it, but they
// are allowed on map elements, hence the need for this map.
BPF_PERCPU_ARRAY_MAP(classification_buf, __u32, char [CLASSIFICATION_MAX_BUFFER], 1)
#else
BPF_ARRAY_MAP(classification_buf, __u8, 1)
#endif

// A set (map from a key to a const bool value, we care only if the key exists in the map, and not its value) to
// mark if we've seen a specific mongo request, so we can eliminate false-positive classification on responses.
BPF_HASH_MAP(mongo_request_id, mongo_key, bool, 1024)

#endif
