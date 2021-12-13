#ifndef _TC_H_
#define _TC_H_

SEC("classifier/ingress")
int classifier_ingress(struct __sk_buff *skb) {
    return TC_ACT_OK;
};

SEC("classifier/egress")
int classifier_egress(struct __sk_buff *skb) {
    struct flow_pid_key_t flow = {};
    struct cursor c = {};
    tc_cursor_init(&c, skb);

    struct packet_t *pkt = reset_packet();
    if (pkt == NULL) {
        // should never happen
        return TC_ACT_OK;
    }
    flow.netns = pkt->netns;

    if (!(parse_ethhdr(&c, &pkt->eth)))
        return TC_ACT_OK;

    switch (pkt->eth.h_proto) {
        case htons(ETH_P_IP):
            // parse IPv4 header
            if (!(parse_iphdr(&c, &pkt->ipv4)))
                return TC_ACT_OK;

            pkt->l4_protocol = pkt->ipv4.protocol;
            flow.addr[0] = pkt->ipv4.saddr;
            break;

        case htons(ETH_P_IPV6):
            // parse IPv6 header
            // TODO: handle multiple IPv6 extension headers
            if (!(parse_ipv6hdr(&c, &pkt->ipv6)))
                return TC_ACT_OK;

            pkt->l4_protocol = pkt->ipv6.nexthdr;
            flow.addr[0] = *(u64*)&pkt->ipv6.saddr;
            flow.addr[1] = *((u64*)(&pkt->ipv6.saddr) + 1);
            break;

        default:
            // TODO: handle ARP, ... etc
            return TC_ACT_OK;
    }

    switch (pkt->l4_protocol) {
        case IPPROTO_TCP:
            // parse TCP header
            if (!(parse_tcphdr(&c, &pkt->tcp)))
                return TC_ACT_OK;

            // adjust cursor with variable tcp options
            c.pos += (pkt->tcp.doff << 2) - sizeof(struct tcphdr);
            return TC_ACT_OK;

        case IPPROTO_UDP:
            // parse UDP header
            if (!(parse_udphdr(&c, &pkt->udp)) || pkt->udp.dest != htons(DNS_PORT))
                return TC_ACT_OK;

            // save current offset within the packet
            pkt->offset = ((u32)(long)c.pos - skb->data);

            // resolve pid
            flow.port = pkt->udp.source;
            pkt->pid = get_flow_pid(&flow);

            return handle_dns_req(skb, pkt);

        default:
            // TODO: handle SCTP, ... etc
            return TC_ACT_OK;
    }

    return TC_ACT_OK;
};

#endif
