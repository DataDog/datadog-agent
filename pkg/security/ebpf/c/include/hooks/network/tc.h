#ifndef _HOOKS_NETWORK_TC_H_
#define _HOOKS_NETWORK_TC_H_

#include "helpers/network/parser.h"
#include "helpers/network/router.h"
#include "helpers/network/pid_resolver.h"
#include "raw.h"

SEC("classifier/ingress")
int classifier_ingress(struct __sk_buff *skb) {
    struct packet_t *pkt = parse_packet(skb, INGRESS);
    if (!pkt) {
        return TC_ACT_UNSPEC;
    }
    resolve_pid(pkt);

    return route_pkt(skb, pkt, INGRESS);
};

SEC("classifier/egress")
int classifier_egress(struct __sk_buff *skb) {
    struct packet_t *pkt = parse_packet(skb, EGRESS);
    if (!pkt) {
        return TC_ACT_UNSPEC;
    }
    resolve_pid(pkt);

    return route_pkt(skb, pkt, EGRESS);
};

__attribute__((always_inline)) int prepare_raw_packet_event(struct __sk_buff *skb) {
    struct raw_packet_event_t *evt = get_raw_packet_event();
    if (evt == NULL) {
        // should never happen
        return TC_ACT_UNSPEC;
    }

    bpf_skb_pull_data(skb, 0);

    u32 len = *(u32 *)(skb + offsetof(struct __sk_buff, len));
    if (len > sizeof(evt->data)) {
        len = sizeof(evt->data);
    }

    if (len > 1) {
        if (bpf_skb_load_bytes(skb, 0, evt->data, len) < 0) {
            return TC_ACT_UNSPEC;
        }
        evt->len = skb->len;
    } else {
        evt->len = 0;
    }

    return TC_ACT_UNSPEC;
}

__attribute__((always_inline)) int is_raw_packet_enabled() {
    u32 key = 0;
    u32 *enabled = bpf_map_lookup_elem(&raw_packet_enabled, &key);
    return enabled && *enabled;
}

__attribute__((always_inline)) int is_raw_packet_allowed(struct packet_t *pkt) {
    u64 filter = 0;
    LOAD_CONSTANT("raw_packet_filter", filter);
    if (!filter) {
        return 1;
    }

    // do not handle tcp packet outside of SYN without process context
    if (pkt->ns_flow.flow.l4_protocol == IPPROTO_TCP && !pkt->tcp.syn && pkt->pid <= 0) {
        return 0;
    }
    return 1;
}

SEC("classifier/ingress")
int classifier_raw_packet_ingress(struct __sk_buff *skb) {
    if (!is_raw_packet_enabled()) {
        return TC_ACT_UNSPEC;
    }

    struct packet_t *pkt = parse_packet(skb, INGRESS);
    if (!pkt) {
        return TC_ACT_UNSPEC;
    }
    resolve_pid(pkt);

    if (!is_raw_packet_allowed(pkt)) {
        return TC_ACT_UNSPEC;
    }

    if (prepare_raw_packet_event(skb) != TC_ACT_UNSPEC) {
        return TC_ACT_UNSPEC;
    }

    bpf_tail_call_compat(skb, &raw_packet_classifier_router, RAW_PACKET_FILTER);

    return TC_ACT_UNSPEC;
};

SEC("classifier/egress")
int classifier_raw_packet_egress(struct __sk_buff *skb) {
    if (!is_raw_packet_enabled()) {
        return TC_ACT_UNSPEC;
    }

    struct packet_t *pkt = parse_packet(skb, EGRESS);
    if (!pkt) {
        return TC_ACT_UNSPEC;
    }
    resolve_pid(pkt);

    if (!is_raw_packet_allowed(pkt)) {
        return TC_ACT_UNSPEC;
    }

    if (prepare_raw_packet_event(skb) != TC_ACT_UNSPEC) {
        return TC_ACT_UNSPEC;
    }

    bpf_tail_call_compat(skb, &raw_packet_classifier_router, RAW_PACKET_FILTER);

    return TC_ACT_UNSPEC;
};

#endif
