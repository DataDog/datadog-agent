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

__attribute__((always_inline)) void fill_network_device_context(struct network_device_context_t *device_ctx, struct __sk_buff *skb, struct packet_t *pkt) {
    device_ctx->netns = pkt->translated_ns_flow.netns;
    device_ctx->ifindex = skb->ifindex;
}

__attribute__((always_inline)) void fill_network_context(struct network_context_t *net_ctx, struct __sk_buff *skb, struct packet_t *pkt) {
    net_ctx->l3_protocol = htons(pkt->eth.h_proto);
    net_ctx->l4_protocol = pkt->l4_protocol;
    net_ctx->size = skb->len;
    net_ctx->flow = pkt->translated_ns_flow.flow;

    fill_network_device_context(&net_ctx->device, skb, pkt);
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


__attribute__((always_inline)) struct packet_t * parse_packet(struct __sk_buff *skb, int direction) {
    struct cursor c = {};
    tc_cursor_init(&c, skb);

    struct packet_t *pkt = reset_packet();
    if (pkt == NULL) {
        // should never happen
        return NULL;
    }

    if (!(parse_ethhdr(&c, &pkt->eth))) {
        return NULL;
    }

    switch (pkt->eth.h_proto) {
    case htons(ETH_P_IP):
        // parse IPv4 header
        if (!(parse_iphdr(&c, &pkt->ipv4))) {
            return NULL;
        }

        // adjust cursor with variable ipv4 options
        if (pkt->ipv4.ihl > 5) {
            c.pos += (pkt->ipv4.ihl - 5) * 4;
            if (c.pos > c.end) {
                return NULL;
            }
        }

        pkt->l4_protocol = pkt->ipv4.protocol;
        pkt->ns_flow.flow.saddr[0] = pkt->ipv4.saddr;
        pkt->ns_flow.flow.daddr[0] = pkt->ipv4.daddr;
        break;

    case htons(ETH_P_IPV6):
        // parse IPv6 header
        // TODO: handle multiple IPv6 extension headers
        if (!(parse_ipv6hdr(&c, &pkt->ipv6))) {
            return NULL;
        }

        pkt->l4_protocol = pkt->ipv6.nexthdr;
        pkt->ns_flow.flow.saddr[0] = *(u64 *)&pkt->ipv6.saddr;
        pkt->ns_flow.flow.saddr[1] = *((u64 *)(&pkt->ipv6.saddr) + 1);
        pkt->ns_flow.flow.daddr[0] = *(u64 *)&pkt->ipv6.daddr;
        pkt->ns_flow.flow.daddr[1] = *((u64 *)(&pkt->ipv6.daddr) + 1);
        break;

    default:
        // TODO: handle ARP, etc ...
        return NULL;
    }

    switch (pkt->l4_protocol) {
    case IPPROTO_TCP:
        // parse TCP header
        if (!(parse_tcphdr(&c, &pkt->tcp))) {
            return NULL;
        }

        // adjust cursor with variable tcp options
        c.pos += (pkt->tcp.doff << 2) - sizeof(struct tcphdr);

        // save current offset within the packet
        pkt->offset = ((u32)(long)c.pos - skb->data);
        pkt->payload_len = skb->len - pkt->offset;
        pkt->ns_flow.flow.sport = pkt->tcp.source;
        pkt->ns_flow.flow.dport = pkt->tcp.dest;
        break;

    case IPPROTO_UDP:
        // parse UDP header
        if (!(parse_udphdr(&c, &pkt->udp))) {
            return NULL;
        }

        // save current offset within the packet
        pkt->offset = ((u32)(long)c.pos - skb->data);
        pkt->payload_len = skb->len - pkt->offset;
        pkt->ns_flow.flow.sport = pkt->udp.source;
        pkt->ns_flow.flow.dport = pkt->udp.dest;
        break;

    default:
        // TODO: handle SCTP, etc ...
        return NULL;
    }

    struct pid_route_t pid_route = {};
    struct namespaced_flow_t tmp_ns_flow = pkt->ns_flow; // for compatibility with older kernels
    pkt->translated_ns_flow = pkt->ns_flow;

// lookup flow in conntrack table
#pragma unroll
    for (int i = 0; i < 10; i++) {
        struct namespaced_flow_t *translated_ns_flow = bpf_map_lookup_elem(&conntrack, &tmp_ns_flow);
        if (translated_ns_flow == NULL) {
            break;
        }

        pkt->translated_ns_flow = *translated_ns_flow;
        tmp_ns_flow = *translated_ns_flow;
    }

    // TODO: if nothing was found in the conntrack map, lookup ingress nat rules (nothing to do for egress though)

    // resolve pid
    switch (direction) {
    case EGRESS: {
        pid_route.addr[0] = pkt->translated_ns_flow.flow.saddr[0];
        pid_route.addr[1] = pkt->translated_ns_flow.flow.saddr[1];
        pid_route.port = pkt->translated_ns_flow.flow.sport;
        pid_route.netns = pkt->translated_ns_flow.netns;
        break;
    }
    case INGRESS: {
        pid_route.addr[0] = pkt->translated_ns_flow.flow.daddr[0];
        pid_route.addr[1] = pkt->translated_ns_flow.flow.daddr[1];
        pid_route.port = pkt->translated_ns_flow.flow.dport;
        pid_route.netns = pkt->translated_ns_flow.netns;
        break;
    }
    }
    pkt->pid = get_flow_pid(&pid_route);

    return pkt;
};

#endif
