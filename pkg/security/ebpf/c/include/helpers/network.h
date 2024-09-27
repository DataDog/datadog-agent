#ifndef _HELPERS_NETWORK_H_
#define _HELPERS_NETWORK_H_

#include "constants/custom.h"
#include "constants/macros.h"
#include "maps.h"

__attribute__((always_inline)) u32 get_flow_pid(struct pid_route_t *key) {
    u32 *value = bpf_map_lookup_elem(&flow_pid, key);
    if (!value) {
        // Try with IP set to 0.0.0.0
        key->addr[0] = 0;
        key->addr[1] = 0;
        value = bpf_map_lookup_elem(&flow_pid, key);
        if (!value) {
            return 0;
        }
    }

    return *value;
}

__attribute__((always_inline)) void flip(struct flow_t *flow) {
    u64 tmp = 0;
    tmp = flow->sport;
    flow->sport = flow->dport;
    flow->dport = tmp;

    tmp = flow->saddr[0];
    flow->saddr[0] = flow->daddr[0];
    flow->daddr[0] = tmp;

    tmp = flow->saddr[1];
    flow->saddr[1] = flow->daddr[1];
    flow->daddr[1] = tmp;
}

__attribute__((always_inline)) void tc_cursor_init(struct cursor *c, struct __sk_buff *skb) {
    c->end = (void *)(long)skb->data_end;
    c->pos = (void *)(long)skb->data;
}

PARSE_FUNC(ethhdr)
PARSE_FUNC(iphdr)
PARSE_FUNC(ipv6hdr)
PARSE_FUNC(udphdr)
PARSE_FUNC(tcphdr)

__attribute__((always_inline)) struct packet_t *get_packet() {
    u32 key = PACKET_KEY;
    return bpf_map_lookup_elem(&packets, &key);
}

__attribute__((always_inline)) struct packet_t *reset_packet() {
    u32 key = PACKET_KEY;
    struct packet_t new_pkt = {
        .ns_flow = {
            .netns = get_netns(),
        },
    };
    bpf_map_update_elem(&packets, &key, &new_pkt, BPF_ANY);
    return get_packet();
}

__attribute__((always_inline)) void fill_network_process_context(struct process_context_t *process, struct packet_t *pkt) {
    process->pid = pkt->pid;
    process->tid = pkt->pid;
    process->netns = pkt->translated_ns_flow.netns;
}

__attribute__((always_inline)) void fill_network_context(struct network_context_t *net_ctx, struct __sk_buff *skb, struct packet_t *pkt) {
    net_ctx->l3_protocol = htons(pkt->eth.h_proto);
    net_ctx->l4_protocol = pkt->l4_protocol;
    net_ctx->size = skb->len;
    net_ctx->flow = pkt->translated_ns_flow.flow;

    // network device context
    net_ctx->device.netns = pkt->translated_ns_flow.netns;
    net_ctx->device.ifindex = skb->ifindex;
}

__attribute__((always_inline)) void tail_call_to_classifier(struct __sk_buff *skb, int classifier_id) {
    bpf_tail_call_compat(skb, &classifier_router, classifier_id);
}

__attribute__((always_inline)) void parse_tuple(struct nf_conntrack_tuple *tuple, struct flow_t *flow) {
    flow->sport = tuple->src.u.all;
    flow->dport = tuple->dst.u.all;

    bpf_probe_read(&flow->saddr, sizeof(flow->saddr), &tuple->src.u3.all);
    bpf_probe_read(&flow->daddr, sizeof(flow->daddr), &tuple->dst.u3.all);
}

#endif
