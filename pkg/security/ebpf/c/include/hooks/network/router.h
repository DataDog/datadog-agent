#ifndef _HOOKS_NETWORK_ROUTER_H_
#define _HOOKS_NETWORK_ROUTER_H_

#include "helpers/network.h"

__attribute__((always_inline)) int route_pkt(struct __sk_buff *skb, struct packet_t *pkt, int direction) {
    // TODO: l3 / l4 firewall

    // route DNS requests
    if (is_event_enabled(EVENT_DNS)) {
        if (pkt->l4_protocol == IPPROTO_UDP && pkt->translated_ns_flow.flow.dport == htons(53)) {
            tail_call_to_classifier(skb, DNS_REQUEST);
        }
    }

    // route IMDS requests
    if (is_event_enabled(EVENT_IMDS)) {
        if (pkt->l4_protocol == IPPROTO_TCP && ((pkt->ns_flow.flow.saddr[0] & 0xFFFFFFFF) == get_imds_ip() || (pkt->ns_flow.flow.daddr[0] & 0xFFFFFFFF) == get_imds_ip())) {
            tail_call_to_classifier(skb, IMDS_REQUEST);
        }
    }

    return ACT_OK;
}

#endif
