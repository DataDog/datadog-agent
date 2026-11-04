#ifndef __TRACER_NAT_H
#define __TRACER_NAT_H

#include "bpf_helpers.h"
#include "bpf_builtins.h"
#include "conn_tuple.h"
#include "conntrack/types.h"
#include "port_range.h"
#include "tracer/maps.h"

// get_skb_tuple finds the tuple the protocol classifier saw for a socket whose packets were NAT'd. sock_tup is the
// socket's tuple, netns included; norm_sock_tup is the same tuple normalized, without netns and pid. On success
// skb_tup holds the packets' tuple, normalized, without netns and pid.
static __always_inline bool get_skb_tuple(conn_tuple_t *sock_tup, conn_tuple_t *norm_sock_tup, conn_tuple_t *skb_tup) {
    conntrack_tuple_t key;
    bpf_memset(&key, 0, sizeof(key));
    key.saddr_h = sock_tup->saddr_h;
    key.saddr_l = sock_tup->saddr_l;
    key.daddr_h = sock_tup->daddr_h;
    key.daddr_l = sock_tup->daddr_l;
    key.sport = sock_tup->sport;
    key.dport = sock_tup->dport;
    key.netns = sock_tup->netns;
    key.metadata = sock_tup->metadata & (CONN_TYPE_TCP | CONN_V6);

    // keyed by the socket's netns: like net_dev_queue, this only sees NAT in the socket's own namespace. The value is
    // the other direction of the connection, so the packets on the socket's side are its inverse.
    conntrack_tuple_t *other = bpf_map_lookup_elem(&conntrack, &key);
    if (other) {
        bpf_memset(skb_tup, 0, sizeof(conn_tuple_t));
        skb_tup->saddr_h = other->daddr_h;
        skb_tup->saddr_l = other->daddr_l;
        skb_tup->daddr_h = other->saddr_h;
        skb_tup->daddr_l = other->saddr_l;
        skb_tup->sport = other->dport;
        skb_tup->dport = other->sport;
        skb_tup->metadata = sock_tup->metadata;
        normalize_tuple(skb_tup);
        return true;
    }

    // without the conntracker's map, the net_dev_queue probe recorded the mapping
    conn_tuple_t *cached = bpf_map_lookup_elem(&conn_tuple_to_socket_skb_conn_tuple, norm_sock_tup);
    if (!cached) {
        return false;
    }
    *skb_tup = *cached;
    normalize_tuple(skb_tup);
    return true;
}

#endif
