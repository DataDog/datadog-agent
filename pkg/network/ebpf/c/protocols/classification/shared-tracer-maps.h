#ifndef __PROTOCOL_CLASSIFICATION_SHARED_TRACER_MAPS_H
#define __PROTOCOL_CLASSIFICATION_SHARED_TRACER_MAPS_H

#include "map-defs.h"
#include "port_range.h"
#include "protocols/classification/stack-helpers.h"

// Maps a connection tuple to its classified protocol. Used to reduce redundant
// classification procedures on the same connection
BPF_HASH_MAP(connection_protocol, conn_tuple_t, protocol_stack_t, 0)

static __always_inline protocol_stack_t* get_protocol_stack(conn_tuple_t *skb_tup) {
    conn_tuple_t normalized_tup = *skb_tup;
    normalize_tuple(&normalized_tup);
    protocol_stack_t* stack = bpf_map_lookup_elem(&connection_protocol, &normalized_tup);
    if (stack) {
        return stack;
    }

    // this code path is executed once during the entire connection lifecycle
    protocol_stack_t empty_stack = {0};
    bpf_map_update_with_telemetry(connection_protocol, &normalized_tup, &empty_stack, BPF_NOEXIST);
    return bpf_map_lookup_elem(&connection_protocol, &normalized_tup);
}

static __always_inline void update_protocol_stack(conn_tuple_t* skb_tup, protocol_t cur_fragment_protocol) {
    protocol_stack_t *stack = get_protocol_stack(skb_tup);
    if (!stack) {
        return;
    }

    set_protocol(stack, cur_fragment_protocol);
}

#endif
