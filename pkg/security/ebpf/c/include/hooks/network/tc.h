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

__attribute__((always_inline)) struct raw_packet_event_t *get_raw_packet_event() {
    u32 key = 0;
    return bpf_map_lookup_elem(&raw_packet_event, &key);
}

__attribute__((always_inline)) int prepare_raw_packet_event(struct __sk_buff *skb) {
    struct raw_packet_event_t *evt = get_raw_packet_event();
    if (evt == NULL) {
        // should never happen
        return ACT_OK;
    }

    bpf_skb_pull_data(skb, 0);

    u32 len = *(u32 *)(skb + offsetof(struct __sk_buff, len));
    if (len > sizeof(evt->data)) {
        len = sizeof(evt->data);
    }

    if (len > 1) {
        if (bpf_skb_load_bytes(skb, 0, evt->data, len) < 0) {
            return ACT_OK;
        }
        evt->len = skb->len;
    } else {
        evt->len = 0;
    }

    return ACT_OK;
}
 

SEC("classifier/ingress")
int classifier_raw_packet_ingress(struct __sk_buff *skb) {
    struct packet_t *pkt = parse_packet(skb, INGRESS);
    if (!pkt) {
        return ACT_OK;
    }

    if (prepare_raw_packet_event(skb) != ACT_OK) {
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

    if (prepare_raw_packet_event(skb) != ACT_OK) {
        return ACT_OK;
    }

    tail_call_to_classifier(skb, RAW_PACKET_FILTER);

    return ACT_OK;
};

#endif
