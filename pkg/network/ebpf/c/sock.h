#ifndef __SOCK_H
#define __SOCK_H

#include "ktypes.h"

#include "bpf_core_read.h"

#include "tracer.h"
#include "ipv6.h"
#include "netns.h"

#ifdef COMPILE_CORE

#include "ip.h" // for AF_INET and AF_INET6

static __always_inline struct tcp_sock *tcp_sk(const struct sock *sk)
{
    return (struct tcp_sock *)sk;
}

static __always_inline struct inet_sock *inet_sk(const struct sock *sk)
{
    return (struct inet_sock *)sk;
}

// source include/net/inet_sock.h
#define inet_daddr sk.__sk_common.skc_daddr
#define inet_rcv_saddr sk.__sk_common.skc_rcv_saddr
#define inet_dport sk.__sk_common.skc_dport
#define inet_num sk.__sk_common.skc_num
// source include/net/sock.h
#define sk_num __sk_common.skc_num
#define sk_dport __sk_common.skc_dport
#define sk_v6_rcv_saddr __sk_common.skc_v6_rcv_saddr
#define sk_v6_daddr __sk_common.skc_v6_daddr
#define sk_daddr __sk_common.skc_daddr
#define sk_rcv_saddr __sk_common.skc_rcv_saddr
#define sk_family __sk_common.skc_family
// source include/net/flow.h
#define fl4_sport uli.ports.sport
#define fl4_dport uli.ports.dport
#define fl6_sport uli.ports.sport
#define fl6_dport uli.ports.dport

#elif defined(COMPILE_RUNTIME) || defined(COMPILE_PREBUILT)

#include <linux/socket.h>
#include <linux/tcp.h>
#include <net/inet_sock.h>

#endif // COMPILE_CORE

#ifdef COMPILE_PREBUILT
static __always_inline u64 offset_socket_sk();
#endif

static __always_inline struct sock * socket_sk(struct socket *sock) {
    struct sock * sk = NULL;
#ifdef COMPILE_PREBUILT
    if (bpf_probe_read_kernel_with_telemetry(&sk, sizeof(sk), ((char*)sock) + offset_socket_sk()) < 0) {
        return NULL;
    }
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&sk, sock, sk);
#endif
    return sk;
}

static __always_inline void get_tcp_segment_counts(struct sock* skp, __u32* packets_in, __u32* packets_out) {
#ifdef COMPILE_PREBUILT
    // counting segments/packets not currently supported on prebuilt
    // to implement, would need to do the offset-guess on the following
    // fields in the tcp_sk: packets_in & packets_out (respectively)
    *packets_in = 0;
    *packets_out = 0;
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(packets_out, tcp_sk(skp), segs_out);
    BPF_CORE_READ_INTO(packets_in, tcp_sk(skp), segs_in);
#endif
}

static __always_inline u16 read_sport(struct sock* skp) {
    // try skc_num, then inet_sport
    u16 sport = 0;
#ifdef COMPILE_PREBUILT
    // try skc_num, then inet_sport
    bpf_probe_read_kernel_with_telemetry(&sport, sizeof(sport), ((char*)skp) + offset_dport() + sizeof(sport));
    if (sport == 0) {
        bpf_probe_read_kernel_with_telemetry(&sport, sizeof(sport), ((char*)skp) + offset_sport());
        sport = bpf_ntohs(sport);
    }
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&sport, skp, sk_num);
    if (sport == 0) {
        BPF_CORE_READ_INTO(&sport, inet_sk(skp), inet_sport);
        sport = bpf_ntohs(sport);
    }
#endif

    return sport;
}

static u16 read_dport(struct sock *skp) {
    u16 dport = 0;
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel_with_telemetry(&dport, sizeof(dport), ((char*)skp) + offset_dport());
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    bpf_probe_read_kernel(&dport, sizeof(dport), &skp->sk_dport);
    BPF_CORE_READ_INTO(&dport, skp, sk_dport);
    if (dport == 0) {
        BPF_CORE_READ_INTO(&dport, inet_sk(skp), inet_dport);
    }
#endif

    return bpf_ntohs(dport);
}

static __always_inline u32 read_saddr_v4(struct sock *skp) {
    u32 saddr = 0;
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel_with_telemetry(&saddr, sizeof(u32), ((char*)skp) + offset_saddr());
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&saddr, skp, sk_rcv_saddr);
    if (saddr == 0) {
        BPF_CORE_READ_INTO(&saddr, inet_sk(skp), inet_saddr);
    }
#endif

    return saddr;
}

static __always_inline u32 read_daddr_v4(struct sock *skp) {
    u32 daddr = 0;
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel_with_telemetry(&daddr, sizeof(u32), ((char*)skp) + offset_daddr());
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&daddr, skp, sk_daddr);
    if (daddr == 0) {
        BPF_CORE_READ_INTO(&daddr, inet_sk(skp), inet_daddr);
    }
#endif

    return daddr;
}

static __always_inline void read_saddr_v6(struct sock *skp, u64 *addr_h, u64 *addr_l) {
    struct in6_addr in6 = {};
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel_with_telemetry(&in6, sizeof(in6), ((char*)skp) + offset_daddr_ipv6() + 2 * sizeof(u64));
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&in6, skp, sk_v6_rcv_saddr);
#endif
    read_in6_addr(addr_h, addr_l, &in6);
}

static __always_inline void read_daddr_v6(struct sock *skp, u64 *addr_h, u64 *addr_l) {
    struct in6_addr in6 = {};
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel_with_telemetry(&in6, sizeof(in6), ((char*)skp) + offset_daddr_ipv6());
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&in6, skp, sk_v6_daddr);
#endif
    read_in6_addr(addr_h, addr_l, &in6);
}

static __always_inline u16 _sk_family(struct sock *skp) {
    u16 family = 0;
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel_with_telemetry(&family, sizeof(family), ((char*)skp) + offset_family());
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&family, skp, sk_family);
#endif
    return family;
}

/**
 * Reads values into a `conn_tuple_t` from a `sock`. Any values that are already set in conn_tuple_t
 * are not overwritten. Returns 1 success, 0 otherwise.
 */
static __always_inline int read_conn_tuple_partial(conn_tuple_t* t, struct sock* skp, u64 pid_tgid, metadata_mask_t type) {
    int err = 0;
    t->pid = pid_tgid >> 32;
    t->metadata = type;

    // Retrieve network namespace id first since addresses and ports may not be available for unconnected UDP
    // sends
    t->netns = get_netns_from_sock(skp);
    u16 family = _sk_family(skp);
    // Retrieve addresses
    if (family == AF_INET) {
        t->metadata |= CONN_V4;
        if (t->saddr_l == 0) {
            t->saddr_l = read_saddr_v4(skp);
        }
        if (t->daddr_l == 0) {
            t->daddr_l = read_daddr_v4(skp);
        }

        if (t->saddr_l == 0 || t->daddr_l == 0) {
            log_debug("ERR(read_conn_tuple.v4): src or dst addr not set src=%d, dst=%d\n", t->saddr_l, t->daddr_l);
            err = 1;
        }
    } else if (family == AF_INET6) {
        if (!is_ipv6_enabled()) {
            return 0;
        }

        if (!(t->saddr_h || t->saddr_l)) {
            read_saddr_v6(skp, &t->saddr_h, &t->saddr_l);
        }
        if (!(t->daddr_h || t->daddr_l)) {
            read_daddr_v6(skp, &t->daddr_h, &t->daddr_l);
        }

        /* We can only pass 4 args to bpf_trace_printk */
        /* so split those 2 statements to be able to log everything */
        if (!(t->saddr_h || t->saddr_l)) {
            log_debug("ERR(read_conn_tuple.v6): src addr not set: src_l:%d,src_h:%d\n",
                t->saddr_l, t->saddr_h);
            err = 1;
        }

        if (!(t->daddr_h || t->daddr_l)) {
            log_debug("ERR(read_conn_tuple.v6): dst addr not set: dst_l:%d,dst_h:%d\n",
                t->daddr_l, t->daddr_h);
            err = 1;
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
    } else {
        log_debug("ERR(read_conn_tuple): unknown family %d\n", family);
        err = 1;
    }

    // Retrieve ports
    if (t->sport == 0) {
        t->sport = read_sport(skp);
    }
    if (t->dport == 0) {
        t->dport = read_dport(skp);
    }

    if (t->sport == 0 || t->dport == 0) {
        log_debug("ERR(read_conn_tuple.v4): src/dst port not set: src:%d, dst:%d\n", t->sport, t->dport);
        err = 1;
    }

    return err ? 0 : 1;
}

/**
 * Reads values into a `conn_tuple_t` from a `sock`. Initializes all values in conn_tuple_t to `0`. Returns 1 success, 0 otherwise.
 */
static __always_inline int read_conn_tuple(conn_tuple_t* t, struct sock* skp, u64 pid_tgid, metadata_mask_t type) {
    bpf_memset(t, 0, sizeof(conn_tuple_t));
    return read_conn_tuple_partial(t, skp, pid_tgid, type);
}

static __always_inline int get_proto(conn_tuple_t *t) {
    return (t->metadata & CONN_TYPE_TCP) ? CONN_TYPE_TCP : CONN_TYPE_UDP;
}

#endif // __SOCK_H
