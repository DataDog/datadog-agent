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
    resolve_pid(skb, pkt);

    return route_pkt(skb, pkt, INGRESS);
};

SEC("classifier/egress")
int classifier_egress(struct __sk_buff *skb) {
    struct packet_t *pkt = parse_packet(skb, EGRESS);
    if (!pkt) {
        return TC_ACT_UNSPEC;
    }
    resolve_pid(skb, pkt);

    return route_pkt(skb, pkt, EGRESS);
};

__attribute__((always_inline)) int prepare_raw_packet_event(struct __sk_buff *skb, struct packet_t *pkt) {
    struct raw_packet_event_t *evt = get_raw_packet_event();
    if (evt == NULL) {
        // should never happen
        return 0;
    }

    evt->process.pid = pkt->pid;
    evt->cgroup.cgroup_file.ino = pkt->cgroup_id;

    bpf_skb_pull_data(skb, 0);

    u32 len = *(u32 *)(skb + offsetof(struct __sk_buff, len));
    if (len > sizeof(evt->data)) {
        len = sizeof(evt->data);
    }

    if (len > 1) {
        if (bpf_skb_load_bytes(skb, 0, evt->data, len) < 0) {
            return 0;
        }
        evt->len = skb->len;
    } else {
        evt->len = 0;
    }

    return 1;
}

__attribute__((always_inline)) int is_raw_packet_enabled() {
    u32 key = 0;
    u32 *enabled = bpf_map_lookup_elem(&raw_packet_enabled, &key);
    return enabled && *enabled;
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
    resolve_pid(skb, pkt);

    if (!is_raw_packet_allowed(pkt)) {
        return TC_ACT_UNSPEC;
    }

    if (!prepare_raw_packet_event(skb, pkt)) {
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
    resolve_pid(skb, pkt);

    pkt->cgroup_id = get_cgroup_id(pkt->pid);
    if (!pkt->cgroup_id) {
        u64 sched_cls_has_current_cgroup_id_helper = 0;
        LOAD_CONSTANT("sched_cls_has_current_cgroup_id_helper", sched_cls_has_current_cgroup_id_helper);
        if (sched_cls_has_current_cgroup_id_helper) {
            pkt->cgroup_id = bpf_get_current_cgroup_id();
        }
    }

    if (!prepare_raw_packet_event(skb, pkt)) {
        return TC_ACT_UNSPEC;
    }

    // call the drop action
    if (pkt->pid > 0 || pkt->cgroup_id > 0) {
        bpf_tail_call_compat(skb, &raw_packet_classifier_router, RAW_PACKET_DROP_ACTION);
    }

    // mostly a rate limiter
    if (!is_raw_packet_allowed(pkt)) {
        return TC_ACT_UNSPEC;
    }

    // call regular filter
    bpf_tail_call_compat(skb, &raw_packet_classifier_router, RAW_PACKET_FILTER);

    return TC_ACT_UNSPEC;
};

#endif
