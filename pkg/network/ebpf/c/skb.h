#ifndef __SKB_H
#define __SKB_H

#include "bpf_telemetry.h"
#include "bpf_helpers.h"
#include "bpf_helpers_custom.h"
#include "bpf_endian.h"
#include "bpf_builtins.h"

#ifndef COMPILE_CORE
#include <linux/skbuff.h>
#include <uapi/linux/in.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/ipv6.h>
#include <uapi/linux/udp.h>
#include <uapi/linux/tcp.h>
#endif

#include "tracer.h"
#include "sock.h"
#include "ipv6.h"

#ifdef COMPILE_PREBUILT
static __always_inline __u64 offset_sk_buff_head() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_sk_buff_head", val);
    return val;
}

static __always_inline __u64 offset_sk_buff_transport_header() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_sk_buff_transport_header", val);
    return val;
}
#endif

static __always_inline unsigned char* sk_buff_head(struct sk_buff *skb) {
    unsigned char *h = NULL;
#ifdef COMPILE_PREBUILT
    int ret = bpf_probe_read_kernel_with_telemetry(&h, sizeof(h), ((char*)skb) + offset_sk_buff_head());
    if (ret < 0) {
        return NULL;
    }
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&h, skb, head);
#endif

    return h;
}

static __always_inline u16 sk_buff_network_header(struct sk_buff *skb) {
    u16 net_head = 0;
#ifdef COMPILE_PREBUILT
    int ret = bpf_probe_read_kernel_with_telemetry(&net_head, sizeof(net_head), ((char*)skb) + offset_sk_buff_transport_header() + 2);
    if (ret < 0) {
        log_debug("ERR reading network_header\n");
        return 0;
    }
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&net_head, skb, network_header);
#endif

    return net_head;
}

static __always_inline u16 sk_buff_transport_header(struct sk_buff *skb) {
    u16 trans_head = 0;
#ifdef COMPILE_PREBUILT
    int ret = bpf_probe_read_kernel_with_telemetry(&trans_head, sizeof(trans_head), ((char*)skb) + offset_sk_buff_transport_header());
    if (ret) {
        log_debug("ERR reading trans_head\n");
        return 0;
    }
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&trans_head, skb, transport_header);
#endif

    return trans_head;
}

// returns the data length of the skb or a negative value in case of an error
static __always_inline int sk_buff_to_tuple(struct sk_buff *skb, conn_tuple_t *tup) {
    unsigned char *head = sk_buff_head(skb);
    if (!head) {
        log_debug("ERR reading head\n");
        return -1;
    }

    u16 net_head = sk_buff_network_header(skb);
    if (!net_head) {
        log_debug("ERR reading network_header\n");
        return -1;
    }

    struct iphdr iph;
    bpf_memset(&iph, 0, sizeof(struct iphdr));
    int ret = bpf_probe_read_kernel_with_telemetry(&iph, sizeof(iph), (struct iphdr *)(head + net_head));
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
    }
#if !defined(COMPILE_RUNTIME) || defined(FEATURE_TCPV6_ENABLED) || defined(FEATURE_UDPV6_ENABLED)
    else if ((is_tcpv6_enabled() || is_udpv6_enabled()) && iph.version == 6) {
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
    }
#endif // !COMPILE_RUNTIME || FEATURE_TCPV6_ENABLED || FEATURE_UDPV6_ENABLED
    else {
        log_debug("unknown IP version: %d\n", iph.version);
        return 0;
    }

    u16 trans_head = sk_buff_transport_header(skb);
    if (!trans_head) {
        log_debug("ERR reading trans_head\n");
        return -1;
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

        log_debug("udp recv: udphdr.len=%d\n", bpf_ntohs(udph.len));
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
