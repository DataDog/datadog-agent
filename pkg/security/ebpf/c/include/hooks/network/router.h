#ifndef _HOOKS_NETWORK_ROUTER_H_
#define _HOOKS_NETWORK_ROUTER_H_

#include "helpers/network.h"

__attribute__((always_inline)) int route_pkt(struct __sk_buff *skb, struct packet_t *pkt, int network_direction) {
    struct pid_route_t pid_route = {};
    struct namespaced_flow_t tmp_ns_flow = pkt->ns_flow; // for compatibility with older kernels
    pkt->translated_ns_flow = pkt->ns_flow;

    // lookup flow in conntrack table
    #pragma unroll
    for (int i = 0; i < 10; i++) {
        struct namespaced_flow_t *translated_ns_flow = bpf_map_lookup_elem(&conntrack, &tmp_ns_flow);
        if (translated_ns_flow == NULL) {
            break;
        }

        pkt->translated_ns_flow = *translated_ns_flow;
        tmp_ns_flow = *translated_ns_flow;
    }

    // TODO: if nothing was found in the conntrack map, lookup ingress nat rules (nothing to do for egress though)

    // resolve pid
    switch (network_direction) {
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
    pkt->pid = get_flow_pid(&pid_route);

    // TODO: l3 / l4 firewall

    // route DNS requests
    if (pkt->l4_protocol == IPPROTO_UDP && pkt->translated_ns_flow.flow.dport == htons(53)) {
        tail_call_to_classifier(skb, DNS_REQUEST);
    }

    // route IMDS requests
    if (pkt->l4_protocol == IPPROTO_TCP && ((pkt->ns_flow.flow.saddr[0] & 0xFFFFFFFF) == get_imds_ip() || (pkt->ns_flow.flow.daddr[0] & 0xFFFFFFFF) == get_imds_ip() )) {
        tail_call_to_classifier(skb, IMDS_REQUEST);
    }

    return ACT_OK;
}

#endif
