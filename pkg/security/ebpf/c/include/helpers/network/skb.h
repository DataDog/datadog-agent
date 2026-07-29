#ifndef _HELPERS_NETWORK_SKB_H_
#define _HELPERS_NETWORK_SKB_H_

__attribute__((always_inline)) unsigned char *sk_buff_head(struct sk_buff *skb) {
    unsigned char *h = NULL;
    u64 offset;
    LOAD_CONSTANT("sk_buff_head_offset", offset);
    bpf_probe_read(&h, sizeof(h), (void *)skb + offset);
    return h;
}

__attribute__((always_inline)) u16 sk_buff_network_header(struct sk_buff *skb) {
    u16 net_head = 0;
    u64 offset;
    LOAD_CONSTANT("sk_buff_transport_header_offset", offset);
    bpf_probe_read(&net_head, sizeof(net_head), (void *)skb + offset + 2);
    return net_head;
}

// Parses an IPv4 ICMP echo request from a kernel sk_buff and fills key fields.
// Returns 1 on success, 0 otherwise.
__attribute__((always_inline)) int parse_icmp_echo_flow_key_from_skb(struct sk_buff *skb, struct pid_route_t *key) {
    unsigned char *head = sk_buff_head(skb);
    if (head == NULL) {
        return 0;
    }

    u16 net_head = sk_buff_network_header(skb);
    if (net_head == 0) {
        return 0;
    }

    struct iphdr iph;
    bpf_probe_read(&iph, sizeof(iph), head + net_head);
    if (iph.version != 4) {
        return 0;
    }
    if (iph.protocol != IPPROTO_ICMP) {
        return 0;
    }

    u8 ihl = iph.ihl;
    if (ihl < 5) {
        return 0;
    }

    struct icmphdr icmph;
    bpf_probe_read(&icmph, sizeof(icmph), head + net_head + (ihl * 4));
    if (icmph.type != ICMP_ECHO) {
        return 0;
    }

    u16 ident = 0;
    bpf_probe_read(&ident, sizeof(ident), &icmph.un.echo.id);
    key->port = htons(ident);
    if (key->port == 0) {
        return 0;
    }

    key->l4_protocol = IPPROTO_ICMP;
    bpf_probe_read(&key->addr[0], sizeof(u32), &iph.saddr);
    return 1;
}

#endif
