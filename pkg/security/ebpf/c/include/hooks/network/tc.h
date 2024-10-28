#ifndef _HOOKS_NETWORK_TC_H_
#define _HOOKS_NETWORK_TC_H_

#include "helpers/network.h"

#include "router.h"
#include "raw.h"

SEC("classifier/ingress")
int classifier_ingress(struct __sk_buff *skb) {
    struct packet_t *pkt = parse_packet(skb, INGRESS);
    if (!pkt) {
        return ACT_OK;
    }

    return route_pkt(skb, pkt, INGRESS);
};

SEC("classifier/egress")
int classifier_egress(struct __sk_buff *skb) {
    struct packet_t *pkt = parse_packet(skb, EGRESS);
    if (!pkt) {
        return ACT_OK;
    }

    return route_pkt(skb, pkt, EGRESS);
};

SEC("classifier/ingress")
int classifier_raw_packet_ingress(struct __sk_buff *skb) {
    struct packet_t *pkt = parse_packet(skb, INGRESS);
    if (!pkt) {
        return ACT_OK;
    }

    tail_call_to_classifier(skb, RAW_PACKET_FILTER);

    return ACT_OK;
};

SEC("classifier/egress")
int classifier_raw_packet_egress(struct __sk_buff *skb) {
    struct packet_t *pkt = parse_packet(skb, EGRESS);
    if (!pkt) {
        return ACT_OK;
    }

    tail_call_to_classifier(skb, RAW_PACKET_FILTER);

    return ACT_OK;
};

#endif
