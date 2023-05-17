#include "kconfig.h"

#include "bpf_helpers.h"
#include "bpf_builtins.h"

#include "offsets.h"
#include "ip.h"

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
    if (tup.sport != 53 && (!dns_stats_enabled() || tup.dport != 53)) {
        return 0;
    }

    return -1;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
