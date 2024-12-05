#ifndef _HELPERS_NETWORK_STATS_H_
#define _HELPERS_NETWORK_STATS_H_

#include "utils.h"

//TODO: handle exit

__attribute__((always_inline)) void count_pkt(struct __sk_buff *skb, struct packet_t *pkt) {
    // prepare a "struct flow_msg_t" for flow_stats computation and message queuing.
    struct flow_queue_msg_t flow_msg = {};

    flow_msg.ns_flow = pkt->translated_ns_flow;
    if (pkt->network_direction == INGRESS) {
        // EGRESS was arbitrarily chosen as "the 5-tuple order for indexing flow statistics".
        // Reverse ingress flow now
        flip(&flow_msg.ns_flow.flow);
    }

    if (flow_msg.ns_flow.flow.l3_protocol == 0 && flow_msg.ns_flow.flow.l4_protocol == 0) {
        bpf_printk("Empty packet !\n");
    }

    u8 should_queue_flow = 0;
    // fetch flow_stats_t entry
    struct network_stats_t *stats = bpf_map_lookup_elem(&ns_flow_to_network_stats, &flow_msg.ns_flow);
    if (stats == NULL) {
        // create new entry now
        should_queue_flow = 1;
        struct network_stats_t new_stats = {};
        bpf_map_update_elem(&ns_flow_to_network_stats, &flow_msg.ns_flow, &new_stats, BPF_ANY);
        stats = bpf_map_lookup_elem(&ns_flow_to_network_stats, &flow_msg.ns_flow);
        if (stats == NULL) {
            // should never happen, ignore
            return;
        }
    }

    // update stats
    switch (pkt->network_direction) {
        case EGRESS: {
            __sync_fetch_and_add(&stats->egress.pkt_count, 1);
            __sync_fetch_and_add(&stats->egress.data_size, skb->len);
            break;
        }
        case INGRESS: {
            __sync_fetch_and_add(&stats->ingress.pkt_count, 1);
            __sync_fetch_and_add(&stats->ingress.data_size, skb->len);
            break;
        }
    }

    // Queue flow so that we can register it in the pid <-> active_flows map
    flow_msg.pid = pkt->pid;
    flow_msg.ifindex = skb->ifindex;

    // BPF_EXIST is used to make room in case the queue is full: expire the oldest element
    bpf_map_push_elem(&flows_queue, &flow_msg, BPF_EXIST);
}

__attribute__((always_inline)) struct active_flows_t *get_empty_active_flows() {
    u32 key = 0;
    return bpf_map_lookup_elem(&active_flows_gen, &key);
}

__attribute__((always_inline)) struct network_monitor_event_t *get_network_monitor_event() {
    u32 key = 0;
    struct network_monitor_event_t *evt = bpf_map_lookup_elem(&network_monitor_event_gen, &key);
    // __builtin_memset doesn't work here because evt is too large and memset is allocating too much memory
    return evt;
}

#endif
