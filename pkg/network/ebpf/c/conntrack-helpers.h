#ifndef __CONNTRACK_HELPERS_H
#define __CONNTRACK_HELPERS_H

#include <net/netfilter/nf_conntrack.h>
#include <linux/types.h>
#include <linux/sched.h>

#include "bpf_builtins.h"
#include "tracer.h"
#include "conntrack-types.h"
#include "ip.h"
#include "netns.h"
#include "ipv6.h"

/* This map is used for conntrack telemetry in kernelspace
 * only key 0 is used
 * value is a telemetry object
 */
BPF_ARRAY_MAP(conntrack_telemetry, conntrack_telemetry_t, 1)

static __always_inline __u32 systemprobe_pid() {
    __u64 val = 0;
    LOAD_CONSTANT("systemprobe_pid", val);
    return (__u32)val;
}

static __always_inline void print_translation(const conntrack_tuple_t *t) {
    if (t->metadata & CONN_TYPE_TCP) {
        log_debug("TCP\n");
    } else {
        log_debug("UDP\n");
    }

    print_ip(t->saddr_h, t->saddr_l, t->sport, t->metadata);
    print_ip(t->daddr_h, t->daddr_l, t->dport, t->metadata);
}

static __always_inline int nf_conntrack_tuple_to_conntrack_tuple(conntrack_tuple_t *t, const struct nf_conntrack_tuple *ct) {
    bpf_memset(t, 0, sizeof(conntrack_tuple_t));

    switch (ct->dst.protonum) {
    case IPPROTO_TCP:
        t->metadata = CONN_TYPE_TCP;
        t->sport = ct->src.u.tcp.port;
        t->dport = ct->dst.u.tcp.port;
        break;
    case IPPROTO_UDP:
        t->metadata = CONN_TYPE_UDP;
        t->sport = ct->src.u.udp.port;
        t->dport = ct->dst.u.udp.port;
        break;
    default:
        log_debug("ERR(to_conn_tuple): unknown protocol number: %u\n", ct->dst.protonum);
        return 0;
    }

    t->sport = bpf_ntohs(t->sport);
    t->dport = bpf_ntohs(t->dport);
    if (t->sport == 0 || t->dport == 0) {
        log_debug("ERR(to_conn_tuple): src/dst port not set: src: %u, dst: %u\n", t->sport, t->dport);
        return 0;
    }

    if (ct->src.l3num == AF_INET) {
        t->metadata |= CONN_V4;
        t->saddr_l = ct->src.u3.ip;
        t->daddr_l = ct->dst.u3.ip;

        if (!t->saddr_l || !t->daddr_l) {
            log_debug("ERR(to_conn_tuple.v4): src/dst addr not set src:%u, dst:%u\n", t->saddr_l, t->daddr_l);
            return 0;
        }
    } else if (ct->src.l3num == AF_INET6 && (is_tcpv6_enabled() || is_udpv6_enabled())) {
        t->metadata |= CONN_V6;
        read_in6_addr(&t->saddr_h, &t->saddr_l, &ct->src.u3.in6);
        read_in6_addr(&t->daddr_h, &t->daddr_l, &ct->dst.u3.in6);

        if (!(t->saddr_h || t->saddr_l)) {
            log_debug("ERR(to_conn_tuple.v6): src addr not set: src_l: %llu, src_h: %llu\n",
                t->saddr_l, t->saddr_h);
            return 0;
        }
        if (!(t->daddr_h || t->daddr_l)) {
            log_debug("ERR(to_conn_tuple.v6): dst addr not set: dst_l: %llu, dst_h: %llu\n",
                t->daddr_l, t->daddr_h);
            return 0;
        }
    }

    return 1;
}

static __always_inline void increment_telemetry_registers_count() {
    u64 key = 0;
    conntrack_telemetry_t *val = bpf_map_lookup_elem(&conntrack_telemetry, &key);
    if (val == NULL) {
        return;
    }
    __sync_fetch_and_add(&val->registers, 1);
}



#endif /* __CONNTRACK_HELPERS_H */
