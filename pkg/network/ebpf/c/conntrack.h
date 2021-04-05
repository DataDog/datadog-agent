#ifndef __CONNTRACK_H
#define __CONNTRACK_H

#include <net/netfilter/nf_conntrack.h>
#include "tracer.h"
#include "conntrack-types.h"
#include "conntrack-maps.h"
#include "ip.h"

#ifdef FEATURE_IPV6_ENABLED
#include "ipv6.h"
#endif

static __always_inline u32 ct_status(const struct nf_conn *ct) {
    u32 status = 0;
    bpf_probe_read(&status, sizeof(status), (void *)&ct->status);
    return status;
}

static __always_inline void print_translation(const conn_tuple_t *t) {
    if (t->metadata & CONN_TYPE_TCP) {
        log_debug("TCP\n");
    } else {
        log_debug("UDP\n");
    }

    print_ip(t->saddr_h, t->saddr_l, t->sport, t->metadata);
    print_ip(t->daddr_h, t->daddr_l, t->dport, t->metadata);
}

static __always_inline int conntrack_tuple_to_conn_tuple(conn_tuple_t *t, const struct nf_conntrack_tuple *ct) {
    __builtin_memset(t, 0, sizeof(conn_tuple_t));

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
    }
#ifdef FEATURE_IPV6_ENABLED
    else if (ct->src.l3num == AF_INET6) {
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
#endif

    return 1;
}

static __always_inline void increment_telemetry_count(enum conntrack_telemetry_counter counter_name) {
    u64 key = 0;
    conntrack_telemetry_t *val = bpf_map_lookup_elem(&conntrack_telemetry, &key);
    if (val == NULL) {
        return;
    }

    switch (counter_name) {
    case registers:
        __sync_fetch_and_add(&val->registers, 1);
        break;
    case registers_dropped:
        __sync_fetch_and_add(&val->registers_dropped, 1);
    }
}

#endif
