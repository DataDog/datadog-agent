#ifndef _TC_H_
#define _TC_H_

SEC("classifier/ingress")
int classifier_ingress(struct __sk_buff *skb) {
    return ACT_OK;
};

SEC("classifier/egress")
int classifier_egress(struct __sk_buff *skb) {
    struct cursor c = {};
    tc_cursor_init(&c, skb);

    struct packet_t *pkt = reset_packet();
    if (pkt == NULL) {
        // should never happen
        return ACT_OK;
    }

    if (!(parse_ethhdr(&c, &pkt->eth)))
        return ACT_OK;

    switch (pkt->eth.h_proto) {
        case htons(ETH_P_IP):
            // parse IPv4 header
            if (!(parse_iphdr(&c, &pkt->ipv4)))
                return ACT_OK;

            pkt->l4_protocol = pkt->ipv4.protocol;
            pkt->ns_flow.flow.saddr[0] = pkt->ipv4.saddr;
            pkt->ns_flow.flow.daddr[0] = pkt->ipv4.daddr;
            break;

        case htons(ETH_P_IPV6):
            // parse IPv6 header
            // TODO: handle multiple IPv6 extension headers
            if (!(parse_ipv6hdr(&c, &pkt->ipv6)))
                return ACT_OK;

            pkt->l4_protocol = pkt->ipv6.nexthdr;
            pkt->ns_flow.flow.saddr[0] = *(u64*)&pkt->ipv6.saddr;
            pkt->ns_flow.flow.saddr[1] = *((u64*)(&pkt->ipv6.saddr) + 1);
            pkt->ns_flow.flow.daddr[0] = *(u64*)&pkt->ipv6.daddr;
            pkt->ns_flow.flow.daddr[1] = *((u64*)(&pkt->ipv6.daddr) + 1);
            break;

        default:
            // TODO: handle ARP, etc ...
            return ACT_OK;
    }

    switch (pkt->l4_protocol) {
        case IPPROTO_TCP:
            // parse TCP header
            if (!(parse_tcphdr(&c, &pkt->tcp)))
                return ACT_OK;

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
            if (!(parse_udphdr(&c, &pkt->udp)))
                return ACT_OK;

            // save current offset within the packet
            pkt->offset = ((u32)(long)c.pos - skb->data);
            pkt->payload_len = skb->len - pkt->offset;
            pkt->ns_flow.flow.sport = pkt->udp.source;
            pkt->ns_flow.flow.dport = pkt->udp.dest;
            break;

        default:
            // TODO: handle SCTP, etc ...
            return ACT_OK;
    }

    return route_pkt(skb, pkt, EGRESS);
};

#endif
