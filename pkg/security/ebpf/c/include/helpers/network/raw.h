#ifndef _HELPERS_NETWORK_RAW_H_
#define _HELPERS_NETWORK_RAW_H_

#include "maps.h"

__attribute__((always_inline)) struct raw_packet_event_t *get_raw_packet_event() {
    u32 key = 0;
    return bpf_map_lookup_elem(&raw_packet_event, &key);
}

__attribute__((always_inline)) int is_raw_packet_allowed(struct packet_t *pkt) {
    u64 filter = 0;
    LOAD_CONSTANT("raw_packet_filter", filter);
    if (!filter) {
        return 1;
    }

    // do not handle tcp packet outside of SYN without process context
    if (pkt->ns_flow.flow.l4_protocol == IPPROTO_TCP && !pkt->l4.tcp.syn && pkt->pid <= 0) {
        return 0;
    }
    return 1;
}

// Double-buffered raw packet classifier: userspace loads into the inactive router, then flips raw_packet_router_sel.
__attribute__((always_inline)) static void tail_call_raw_packet_router(struct __sk_buff *skb, u32 index) {
    u32 k = 0;
    u32 *sel = bpf_map_lookup_elem(&raw_packet_router_sel, &k);
    if (!sel) {
        return;
    }
    if (*sel == 0) {
        bpf_tail_call_compat(skb, &raw_packet_classifier_router_0, index);
    } else {
        bpf_tail_call_compat(skb, &raw_packet_classifier_router_1, index);
    }
}

#endif
