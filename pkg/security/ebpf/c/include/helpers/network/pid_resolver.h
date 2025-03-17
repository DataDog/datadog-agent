#ifndef _HELPERS_NETWORK_PID_RESOLVER_H_
#define _HELPERS_NETWORK_PID_RESOLVER_H_

#include "maps.h"

__attribute__((always_inline)) s64 get_flow_pid(struct pid_route_t *key) {
    struct pid_route_entry_t *value = bpf_map_lookup_elem(&flow_pid, key);
    if (!value) {
        // Try with IP set to 0.0.0.0
        key->addr[0] = 0;
        key->addr[1] = 0;
        value = bpf_map_lookup_elem(&flow_pid, key);
        if (!value) {
            return -1;
        }
    }

    return value->pid;
}

__attribute__((always_inline)) void resolve_pid(struct packet_t *pkt) {
    struct pid_route_t pid_route = {};

    // resolve pid
    switch (pkt->network_direction) {
    case EGRESS: {
        pid_route.addr[0] = pkt->translated_ns_flow.flow.saddr[0];
        pid_route.addr[1] = pkt->translated_ns_flow.flow.saddr[1];
        pid_route.port = pkt->translated_ns_flow.flow.sport;
        pid_route.netns = pkt->translated_ns_flow.netns;
        break;
    }
    case INGRESS: {
        pid_route.addr[0] = pkt->translated_ns_flow.flow.daddr[0];
        pid_route.addr[1] = pkt->translated_ns_flow.flow.daddr[1];
        pid_route.port = pkt->translated_ns_flow.flow.dport;
        pid_route.netns = pkt->translated_ns_flow.netns;
        break;
    }
    }

    // TODO: l4_protocol should be used to uniquely identify the PID - wait for implementation on security_socket_bind
    // pid_route.l4_protocol = pkt->translated_ns_flow.flow.l4_protocol;

    pkt->pid = get_flow_pid(&pid_route);
}

#endif
