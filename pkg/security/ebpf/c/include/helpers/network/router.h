#ifndef _HELPERS_NETWORK_ROUTER_H_
#define _HELPERS_NETWORK_ROUTER_H_

#include "credentials.h"
#include "stats.h"
#include "maps.h"

__attribute__((always_inline)) int route_pkt(struct __sk_buff *skb, struct packet_t *pkt, int direction) {
    if (is_network_flow_monitor_enabled()) {
        count_pkt(skb, pkt);
    }

    u64 dns_port;
    LOAD_CONSTANT("dns_port", dns_port);

    // route DNS requests
    if (pkt->translated_ns_flow.flow.l4_protocol == IPPROTO_UDP) {
        if (pkt->translated_ns_flow.flow.tcp_udp.sport == dns_port) {
            bpf_tail_call_compat(skb, &classifier_router, DNS_RESPONSE);
        } else if (pkt->translated_ns_flow.flow.tcp_udp.dport == dns_port && is_event_enabled(EVENT_DNS)) {
                bpf_tail_call_compat(skb, &classifier_router, DNS_REQUEST);
        }
    }


    // route credential endpoint traffic (IMDS, EKS Pod Identity, ...)
    if (is_event_enabled(EVENT_IMDS)) {
        if (pkt->translated_ns_flow.flow.l4_protocol == IPPROTO_TCP && get_credential_source(pkt) != CREDENTIAL_SOURCE_UNKNOWN) {
            bpf_tail_call_compat(skb, &classifier_router, IMDS_REQUEST);
        }
    }

    return TC_ACT_UNSPEC;
}

#endif
