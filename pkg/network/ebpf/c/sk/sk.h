#ifndef __SK_H
#define __SK_H

#include "ktypes.h"

static __always_inline void print_sk_ip(u64 ip_h, u64 ip_l, u16 port, u32 metadata) {
    if (metadata & CONN_V6) {
        struct in6_addr addr;
        addr.in6_u.u6_addr32[0] = ip_h & 0xFFFFFFFF;
        addr.in6_u.u6_addr32[1] = (ip_h >> 32) & 0xFFFFFFFF;
        addr.in6_u.u6_addr32[2] = ip_l & 0xFFFFFFFF;
        addr.in6_u.u6_addr32[3] = (ip_l >> 32) & 0xFFFFFFFF;
        if (metadata & CONN_TYPE_TCP) {
            log_debug("TCPv6 %pI6c:%u", &addr, port);
        } else {
            log_debug("UDPv6 %pI6c:%u", &addr, port);
        }
    } else {
        if (metadata & CONN_TYPE_TCP) {
            log_debug("TCPv4 %pI4:%u", &ip_l, port);
        } else {
            log_debug("UDPv4 %pI4:%u", &ip_l, port);
        }
    }
}

static __always_inline void copy_conn_tuple(conn_tuple_t *t, conn_tuple_t *stats_tup) {
    if (stats_tup->saddr_l || stats_tup->saddr_h) {
        t->saddr_h = stats_tup->saddr_h;
        t->saddr_l = stats_tup->saddr_l;
    }
    if (stats_tup->daddr_l || stats_tup->daddr_h) {
        t->daddr_h = stats_tup->daddr_h;
        t->daddr_l = stats_tup->daddr_l;
    }
    if (stats_tup->sport) t->sport = stats_tup->sport;
    if (stats_tup->dport) t->dport = stats_tup->dport;
    if (stats_tup->netns) t->netns = stats_tup->netns;
    if (stats_tup->pid) t->pid = stats_tup->pid;
    if (stats_tup->metadata) t->metadata |= stats_tup->metadata;
}

static __always_inline int read_conn_tuple_sk(conn_tuple_t* t, struct sock* sk, struct task_struct *task) {
    int err = 0;

    if (!t->pid) {
        t->pid = task ? task->tgid : GET_USER_MODE_PID(bpf_get_current_pid_tgid());
    }

    if (!t->netns) {
        t->netns = get_netns_from_sock(sk);
    }

    u16 family = sk->sk_family;
    if (family == AF_INET) {
        if (!is_tcpv4_enabled() && !is_udpv4_enabled()) {
            return 0;
        }
        t->metadata |= CONN_V4;

        if (t->saddr_l == 0) {
            t->saddr_l = read_saddr_v4(sk);
        }
        if (t->daddr_l == 0) {
            t->daddr_l = read_daddr_v4(sk);
        }

        if (t->saddr_l == 0 || t->daddr_l == 0) {
            log_debug("ERR(read_conn_tuple_sk.v4): src or dst addr not set src=%llu, dst=%llu", t->saddr_l, t->daddr_l);
            err = 1;
        }
    } else if (family == AF_INET6) {
        if (!is_tcpv6_enabled() && !is_udpv6_enabled()) {
            return 0;
        }

        if (!(t->saddr_h || t->saddr_l)) {
            read_saddr_v6(sk, &t->saddr_h, &t->saddr_l);
        }
        if (!(t->daddr_h || t->daddr_l)) {
            read_daddr_v6(sk, &t->daddr_h, &t->daddr_l);
        }

        /* We can only pass 4 args to bpf_trace_printk */
        /* so split those 2 statements to be able to log everything */
        if (!(t->saddr_h || t->saddr_l)) {
            log_debug("ERR(read_conn_tuple.v6): src addr not set: src_l:%llu,src_h:%llu",
                t->saddr_l, t->saddr_h);
            err = 1;
        }
        if (!(t->daddr_h || t->daddr_l)) {
            log_debug("ERR(read_conn_tuple.v6): dst addr not set: dst_l:%llu,dst_h:%llu",
                t->daddr_l, t->daddr_h);
            err = 1;
        }

        // Check if we can map IPv6 to IPv4
        if (err == 0) {
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
    } else {
        return 0;
    }

    if (t->sport == 0) {
        t->sport = read_sport(sk);
    }
    if (t->dport == 0) {
        t->dport = read_dport(sk);
    }

    if (t->sport == 0 || t->dport == 0) {
        log_debug("ERR(read_conn_tuple.v4): src/dst port not set: src:%d, dst:%d", t->sport, t->dport);
        err = 1;
    }

    return err ? 0 : 1;
}

#endif
