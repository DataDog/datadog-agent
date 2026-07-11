#include "kconfig.h"
#include "bpf_metadata.h"

#include "bpf_helpers.h"
#include "bpf_builtins.h"

#include "offsets.h"
#include "ip.h"

// DNS_PORTS_MAX is the maximum number of distinct DNS ports the filter can
// be configured to monitor. Must stay in sync with DNSPortsMax in
// pkg/network/config/config.go
// 8 covers every realistic configuration (53 + mDNS/LLMNR + the
// 1053/8053/9053/10053 unprivileged CoreDNS family + a spare slot).
#define DNS_PORTS_MAX 8

// is_dns_port returns true iff port matches one of the configured DNS ports.
// Configured ports are loaded at agent startup as a sorted ascending
// sequence of LOAD_CONSTANT values dns_port_0..dns_port_{DNS_PORTS_MAX-1},
// with a zero sentinel after the last real entry.
static __always_inline bool is_dns_port(__u16 port) {
#define DNS_PORT_SLOT(n)                              \
    do {                                              \
        __u64 p = 0;                                  \
        LOAD_CONSTANT("dns_port_" #n, p);             \
        if (p == 0 || (__u16)p > port) return false;  \
        if (port == (__u16)p) return true;            \
    } while (0)
    DNS_PORT_SLOT(0);
    DNS_PORT_SLOT(1);
    DNS_PORT_SLOT(2);
    DNS_PORT_SLOT(3);
    DNS_PORT_SLOT(4);
    DNS_PORT_SLOT(5);
    DNS_PORT_SLOT(6);
    DNS_PORT_SLOT(7);
#undef DNS_PORT_SLOT
    return false;
}

// This function is meant to be used as a BPF_PROG_TYPE_SOCKET_FILTER.
// When attached to a RAW_SOCKET, this code filters out everything but DNS traffic.
// All structs referenced here are kernel independent as they simply map protocol headers (Ethernet, IP and UDP).
SEC("socket/dns_filter")
int socket__dns_filter(struct __sk_buff* skb) {
    skb_info_t skb_info;
    conn_tuple_t tup;
    bpf_memset(&tup, 0, sizeof(conn_tuple_t));
    if (!read_conn_tuple_skb(skb, &skb_info, &tup)) {
        return 0;
    }

    if (is_dns_port(tup.sport)) {
        return -1;
    }

    if (dns_stats_enabled() && is_dns_port(tup.dport)) {
        return -1;
    }

    return 0;
}

char _license[] SEC("license") = "GPL";
