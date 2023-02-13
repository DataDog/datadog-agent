#ifndef __CONN_TUPLE_H
#define __CONN_TUPLE_H

#include "kconfig.h"
#include "bpf_builtins.h"
#include "bpf_telemetry.h"

#include "offsets.h"

#include "tracer.h"
#include "defs.h"
#include "ipv6.h"

// source include/linux/socket.h
#define __AF_INET   2
#define __AF_INET6 10

static __always_inline __u32 get_netns_from_sock(struct sock* sk) {
    void* skc_net = NULL;
    __u32 net_ns_inum = 0;
    bpf_probe_read_kernel_with_telemetry(&skc_net, sizeof(void*), ((char*)sk) + offset_netns());
    bpf_probe_read_kernel_with_telemetry(&net_ns_inum, sizeof(net_ns_inum), ((char*)skc_net) + offset_ino());
    return net_ns_inum;
}

static __always_inline __u16 read_sport(struct sock* sk) {
    __u16 sport = 0;
    // try skc_num, then inet_sport
    bpf_probe_read_kernel_with_telemetry(&sport, sizeof(sport), ((char*)sk) + offset_dport() + sizeof(sport));
    if (sport == 0) {
        bpf_probe_read_kernel_with_telemetry(&sport, sizeof(sport), ((char*)sk) + offset_sport());
        sport = bpf_ntohs(sport);
    }
    return sport;
}

static __always_inline bool check_family(struct sock* sk, u16 expected_family) {
    u16 family = 0;
    bpf_probe_read_kernel_with_telemetry(&family, sizeof(u16), ((char*)sk) + offset_family());
    return family == expected_family;
}

/**
 * Reads values into a `conn_tuple_t` from a `sock`. Any values that are already set in conn_tuple_t
 * are not overwritten. Returns 1 success, 0 otherwise.
 */
static __always_inline int read_conn_tuple_partial(conn_tuple_t * t, struct sock* skp, u64 pid_tgid, metadata_mask_t type) {
    t->pid = pid_tgid >> 32;
    t->metadata = type;

    // Retrieve network namespace id first since addresses and ports may not be available for unconnected UDP
    // sends
    t->netns = get_netns_from_sock(skp);

    // Retrieve addresses
    if (check_family(skp, __AF_INET)) {
        t->metadata |= CONN_V4;
        if (t->saddr_l == 0) {
            bpf_probe_read_kernel_with_telemetry(&t->saddr_l, sizeof(u32), ((char*)skp) + offset_saddr());
        }
        if (t->daddr_l == 0) {
            bpf_probe_read_kernel_with_telemetry(&t->daddr_l, sizeof(u32), ((char*)skp) + offset_daddr());
        }

        if (!t->saddr_l || !t->daddr_l) {
            log_debug("ERR(read_conn_tuple.v4): src or dst addr not set src=%d, dst=%d\n", t->saddr_l, t->daddr_l);
            return 0;
        }
    } else if (check_family(skp, __AF_INET6)) {
        if (!is_ipv6_enabled()) {
            return 0;
        }

        if (!(t->saddr_h || t->saddr_l)) {
            bpf_probe_read_kernel_with_telemetry(&t->saddr_h, sizeof(t->saddr_h), ((char*)skp) + offset_daddr_ipv6() + 2 * sizeof(u64));
            bpf_probe_read_kernel_with_telemetry(&t->saddr_l, sizeof(t->saddr_l), ((char*)skp) + offset_daddr_ipv6() + 3 * sizeof(u64));
        }
        if (!(t->daddr_h || t->daddr_l)) {
            bpf_probe_read_kernel_with_telemetry(&t->daddr_h, sizeof(t->daddr_h), ((char*)skp) + offset_daddr_ipv6());
            bpf_probe_read_kernel_with_telemetry(&t->daddr_l, sizeof(t->daddr_l), ((char*)skp) + offset_daddr_ipv6() + sizeof(u64));
        }

        // We can only pass 4 args to bpf_trace_printk
        // so split those 2 statements to be able to log everything
        if (!(t->saddr_h || t->saddr_l)) {
            log_debug("ERR(read_conn_tuple.v6): src addr not set: type=%d, saddr_l=%d, saddr_h=%d\n",
                      type, t->saddr_l, t->saddr_h);
            return 0;
        }

        if (!(t->daddr_h || t->daddr_l)) {
            log_debug("ERR(read_conn_tuple.v6): dst addr not set: type=%d, daddr_l=%d, daddr_h=%d\n",
                      type, t->daddr_l, t->daddr_h);
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

    // Retrieve ports
    if (t->sport == 0) {
        t->sport = read_sport(skp);
    }
    if (t->dport == 0) {
        bpf_probe_read_kernel_with_telemetry(&t->dport, sizeof(t->dport), ((char*)skp) + offset_dport());
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
    bpf_memset(t, 0, sizeof(conn_tuple_t));
    return read_conn_tuple_partial(t, skp, pid_tgid, type);
}

#endif // __CONN_TUPLE_H
