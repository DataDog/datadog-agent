#ifndef __SK_BUFF_H
#define __SK_BUFF_H

#include "tracer.h"
#include "bpf_helpers.h"
#include "bpf_endian.h"
#include "bpf_builtins.h"
#include "ipv6.h"

#ifndef COMPILE_CORE
#include <linux/skbuff.h>
#include <uapi/linux/in.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/ipv6.h>
#include <uapi/linux/udp.h>
#include <uapi/linux/tcp.h>
#endif

// returns the data length of the skb or a negative value in case of an error
static __always_inline int sk_buff_to_tuple(struct sk_buff *skb, conn_tuple_t *tup) {
    unsigned char *head = NULL;
    int ret = BPF_CORE_READ_INTO(&head, skb, head);
    if (ret || !head) {
        log_debug("ERR reading head\n");
        return ret;
    }
    u16 net_head = 0;
    ret = BPF_CORE_READ_INTO(&net_head, skb, network_header);
    if (ret) {
        log_debug("ERR reading network_header\n");
        return ret;
    }

    struct iphdr iph;
    bpf_memset(&iph, 0, sizeof(struct iphdr));
    ret = bpf_probe_read_kernel_with_telemetry(&iph, sizeof(iph), (struct iphdr *)(head + net_head));
    if (ret) {
        log_debug("ERR reading iphdr\n");
        return ret;
    }

    int trans_len = 0;
    if (iph.version == 4) {
        tup->metadata |= CONN_V4;
        switch (iph.protocol) {
            case IPPROTO_UDP:
                tup->metadata |= CONN_TYPE_UDP;
                break;
            case IPPROTO_TCP:
                tup->metadata |= CONN_TYPE_TCP;
                break;
            default:
                log_debug("unknown protocol: %d\n", iph.protocol);
                return 0;
        }
        trans_len = iph.tot_len - (iph.ihl * 4);
        bpf_probe_read_kernel_with_telemetry(&tup->saddr_l, sizeof(__be32), &iph.saddr);
        bpf_probe_read_kernel_with_telemetry(&tup->daddr_l, sizeof(__be32), &iph.daddr);
    } else if (is_ipv6_enabled() && iph.version == 6) {
        struct ipv6hdr ip6h;
        bpf_memset(&ip6h, 0, sizeof(struct ipv6hdr));
        ret = bpf_probe_read_kernel_with_telemetry(&ip6h, sizeof(ip6h), (struct ipv6hdr *)(head + net_head));
        if (ret) {
            log_debug("ERR reading ipv6 hdr\n");
            return ret;
        }
        tup->metadata |= CONN_V6;
        switch (ip6h.nexthdr) {
            case IPPROTO_UDP:
                tup->metadata |= CONN_TYPE_UDP;
                break;
            case IPPROTO_TCP:
                tup->metadata |= CONN_TYPE_TCP;
                break;
            default:
                log_debug("unknown protocol: %d\n", ip6h.nexthdr);
                return 0;
        }

        trans_len = bpf_ntohs(ip6h.payload_len) - sizeof(struct ipv6hdr);
        read_in6_addr(&tup->saddr_h, &tup->saddr_l, &ip6h.saddr);
        read_in6_addr(&tup->daddr_h, &tup->daddr_l, &ip6h.daddr);
    } else {
        log_debug("unknown IP version: %d\n", iph.version);
        return 0;
    }

    u16 trans_head = 0;
    ret = BPF_CORE_READ_INTO(&trans_head, skb, transport_header);
    if (ret) {
        log_debug("ERR reading trans_head\n");
        return ret;
    }

    int proto = get_proto(tup);
    if (proto == CONN_TYPE_UDP) {
        struct udphdr udph;
        bpf_memset(&udph, 0, sizeof(struct udphdr));
        ret = bpf_probe_read_kernel_with_telemetry(&udph, sizeof(udph), (struct udphdr *)(head + trans_head));
        if (ret) {
            log_debug("ERR reading udphdr\n");
            return ret;
        }
        tup->sport = bpf_ntohs(udph.source);
        tup->dport = bpf_ntohs(udph.dest);

        //log_debug("udp recv: udphdr.len=%d\n", bpf_ntohs(udph.len));
        return (int)(bpf_ntohs(udph.len) - sizeof(struct udphdr));
    } else if (proto == CONN_TYPE_TCP) {
        struct tcphdr tcph;
        bpf_memset(&tcph, 0, sizeof(struct tcphdr));
        ret = bpf_probe_read_kernel_with_telemetry(&tcph, sizeof(tcph), (struct tcphdr *)(head + trans_head));
        if (ret) {
            log_debug("ERR reading tcphdr\n");
            return ret;
        }
        tup->sport = bpf_ntohs(tcph.source);
        tup->dport = bpf_ntohs(tcph.dest);

        //log_debug("tcp recv: trans_len=%u tcphdr.doff=%u\n", trans_len, tcph.doff*4);
        return trans_len - (tcph.doff * 4);
    }

    log_debug("ERR unknown connection type\n");
    return 0;
}

#endif
