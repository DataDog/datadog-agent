#ifndef _HELPERS_NETWORK_ROUTER_H_
#define _HELPERS_NETWORK_ROUTER_H_

#include "stats.h"
#include "maps.h"

__attribute__((always_inline)) int route_pkt(struct __sk_buff *skb, struct packet_t *pkt, int direction) {
    count_pkt(skb, pkt);

    // route DNS requests
    if (is_event_enabled(EVENT_DNS)) {
        if (pkt->translated_ns_flow.flow.l4_protocol == IPPROTO_UDP && pkt->translated_ns_flow.flow.dport == htons(53)) {
            bpf_tail_call_compat(skb, &classifier_router, DNS_REQUEST);
        }
    }

    // route IMDS requests
    if (is_event_enabled(EVENT_IMDS)) {
        if (pkt->translated_ns_flow.flow.l4_protocol == IPPROTO_TCP && ((pkt->ns_flow.flow.saddr[0] & 0xFFFFFFFF) == get_imds_ip() || (pkt->ns_flow.flow.daddr[0] & 0xFFFFFFFF) == get_imds_ip())) {
            bpf_tail_call_compat(skb, &classifier_router, IMDS_REQUEST);
        }
    }

    return ACT_OK;
}

#endif
