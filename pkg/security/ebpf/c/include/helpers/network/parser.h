#ifndef _HELPERS_NETWORK_PARSER_H_
#define _HELPERS_NETWORK_PARSER_H_

#include "constants/custom.h"
#include "constants/macros.h"
#include "maps.h"

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

    if (!(parse_ethhdr(skb, &c, &pkt->eth))) {
        return NULL;
    }

    pkt->network_direction = direction;
    pkt->ns_flow.flow.l3_protocol = ntohs(pkt->eth.h_proto);

    switch (pkt->ns_flow.flow.l3_protocol) {
    case ETH_P_IP:
        // parse IPv4 header
        if (!(parse_iphdr(skb, &c, &pkt->ipv4))) {
            return NULL;
        }

        // adjust cursor with variable ipv4 options
        if (pkt->ipv4.ihl > 5) {
            c.pos += (pkt->ipv4.ihl - 5) * 4;
            if (c.pos > c.end) {
                return NULL;
            }
        }

        pkt->ns_flow.flow.l4_protocol = pkt->ipv4.protocol;
        pkt->ns_flow.flow.saddr[0] = pkt->ipv4.saddr;
        pkt->ns_flow.flow.daddr[0] = pkt->ipv4.daddr;
        break;

    case ETH_P_IPV6:
        // parse IPv6 header
        // TODO: handle multiple IPv6 extension headers
        if (!(parse_ipv6hdr(skb, &c, &pkt->ipv6))) {
            return NULL;
        }

        pkt->ns_flow.flow.l4_protocol = pkt->ipv6.nexthdr;
        pkt->ns_flow.flow.saddr[0] = *(u64 *)&pkt->ipv6.saddr;
        pkt->ns_flow.flow.saddr[1] = *((u64 *)(&pkt->ipv6.saddr) + 1);
        pkt->ns_flow.flow.daddr[0] = *(u64 *)&pkt->ipv6.daddr;
        pkt->ns_flow.flow.daddr[1] = *((u64 *)(&pkt->ipv6.daddr) + 1);
        break;

    default:
        // TODO: handle ARP, etc ...
        return NULL;
    }

    switch (pkt->ns_flow.flow.l4_protocol) {
    case IPPROTO_TCP:
        // parse TCP header
        if (!(parse_tcphdr(skb, &c, &pkt->tcp))) {
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
        if (!(parse_udphdr(skb, &c, &pkt->udp))) {
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

    return pkt;
};

#endif
