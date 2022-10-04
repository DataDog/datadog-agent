#ifndef __CONN_TUPLE_H
#define __CONN_TUPLE_H

#include "netns.h"

#ifdef FEATURE_IPV6_ENABLED
#include "ipv6.h"
#endif

#include <net/inet_sock.h>

static __always_inline __u16 read_sport(struct sock* skp) {
    __u16 sport = 0;
    bpf_probe_read_kernel_with_telemetry(&sport, sizeof(sport), &skp->sk_num);
    if (sport == 0) {
        bpf_probe_read_kernel_with_telemetry(&sport, sizeof(sport), &inet_sk(skp)->inet_sport);
        sport = bpf_ntohs(sport);
    }
    return sport;
}

/**
 * Reads values into a `conn_tuple_t` from a `sock`. Any values that are already set in conn_tuple_t
 * are not overwritten. Returns 1 success, 0 otherwise.
 */
static __always_inline int read_conn_tuple_partial(conn_tuple_t* t, struct sock* skp, u64 pid_tgid, metadata_mask_t type) {
    t->pid = pid_tgid >> 32;
    t->metadata = type;

    // Retrieve network namespace id first since addresses and ports may not be available for unconnected UDP
    // sends
    t->netns = get_netns(&skp->sk_net);
    u16 family = 0;
    bpf_probe_read_kernel_with_telemetry(&family, sizeof(family), &skp->sk_family);

    // Retrieve addresses
    if (family == AF_INET) {
        t->metadata |= CONN_V4;
        if (t->saddr_l == 0) {
            bpf_probe_read_kernel_with_telemetry(&t->saddr_l, sizeof(__be32), &skp->sk_rcv_saddr);
        }
        if (t->daddr_l == 0) {
            bpf_probe_read_kernel_with_telemetry(&t->daddr_l, sizeof(__be32), &skp->sk_daddr);
        }

        if (!t->saddr_l || !t->daddr_l) {
            log_debug("ERR(read_conn_tuple.v4): src/dst addr not set src:%d,dst:%d\n", t->saddr_l, t->daddr_l);
            return 0;
        }
    }
#ifdef FEATURE_IPV6_ENABLED
    else if (family == AF_INET6) {
        // TODO cleanup? having it split on 64 bits is not nice for kernel reads
        if (!(t->saddr_h || t->saddr_l)) {
            read_in6_addr(&t->saddr_h, &t->saddr_l, &skp->sk_v6_rcv_saddr);
        }
        if (!(t->daddr_h || t->daddr_l)) {
            read_in6_addr(&t->daddr_h, &t->daddr_l, &skp->sk_v6_daddr);
        }

        // We can only pass 4 args to bpf_trace_printk
        // so split those 2 statements to be able to log everything
        if (!(t->saddr_h || t->saddr_l)) {
            log_debug("ERR(read_conn_tuple.v6): src addr not set: src_l:%d,src_h:%d\n",
                t->saddr_l, t->saddr_h);
            return 0;
        }

        if (!(t->daddr_h || t->daddr_l)) {
            log_debug("ERR(read_conn_tuple.v6): dst addr not set: dst_l:%d,dst_h:%d\n",
                t->daddr_l, t->daddr_h);
            return 0;
        }

        // Check if we can map IPv6 to IPv4
        if (is_ipv4_mapped_ipv6(t->saddr_h, t->saddr_l, t->daddr_h, t->daddr_l)) {
            t->metadata |= CONN_V4;
            t->saddr_h = 0;
            t->daddr_h = 0;
            t->saddr_l = (__u32)(t->saddr_l >> 32);
            t->daddr_l = (__u32)(t->daddr_l >> 32);
        } else {
            t->metadata |= CONN_V6;
        }
    }
#endif

    // Retrieve ports
    if (t->sport == 0) {
        t->sport = read_sport(skp);
    }
    if (t->dport == 0) {
        bpf_probe_read_kernel_with_telemetry(&t->dport, sizeof(t->dport), &skp->sk_dport);
        t->dport = bpf_ntohs(t->dport);
    }

    if (t->sport == 0 || t->dport == 0) {
        log_debug("ERR(read_conn_tuple.v4): src/dst port not set: src:%d, dst:%d\n", t->sport, t->dport);
        return 0;
    }

    return 1;
}

/**
 * Reads values into a `conn_tuple_t` from a `sock`. Initializes all values in conn_tuple_t to `0`. Returns 1 success, 0 otherwise.
 */
static __always_inline int read_conn_tuple(conn_tuple_t* t, struct sock* skp, u64 pid_tgid, metadata_mask_t type) {
    __builtin_memset(t, 0, sizeof(conn_tuple_t));
    return read_conn_tuple_partial(t, skp, pid_tgid, type);
}

#endif
