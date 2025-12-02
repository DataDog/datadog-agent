#include "kconfig.h"
#include "bpf_metadata.h"

#include "bpf_helpers.h"
#include "bpf_builtins.h"

#include "offsets.h"
#include "ip.h"

// This function is meant to be used as a BPF_PROG_TYPE_SOCKET_FILTER.
// When attached to a RAW_SOCKET, this code filters out everything but DNS traffic.
// All structs referenced here are kernel independent as they simply map protocol headers (Ethernet, IP and UDP).
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 32);
    __type(key, __u16);
    __type(value, __u8);
} dns_ports SEC(".maps");

SEC("socket/dns_filter")
int socket__dns_filter(struct __sk_buff* skb) {
    skb_info_t skb_info;
    conn_tuple_t tup;
    bpf_memset(&tup, 0, sizeof(conn_tuple_t));
    if (!read_conn_tuple_skb(skb, &skb_info, &tup)) {
        return 0;
    }

    __u16 sport = tup.sport;
    __u16 dport = tup.dport;

    if (bpf_map_lookup_elem(&dns_ports, &sport) != NULL) {
        return -1;
    }

    if (dns_stats_enabled() && bpf_map_lookup_elem(&dns_ports, &dport) != NULL) {
        return -1;
    }

    return 0;
}

char _license[] SEC("license") = "GPL";
