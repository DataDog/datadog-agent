#include "kconfig.h"
#include "bpf_tracing.h"
#include "map-defs.h"

#include <net/net_namespace.h>
#include <net/sock.h>
#include <net/inet_sock.h>
#include <net/flow.h>
#include <linux/tcp.h>

typedef enum {
    OFFSET_SADDR = 0,
    OFFSET_DADDR,
    OFFSET_SPORT,
    OFFSET_DPORT,
    OFFSET_NETNS,
    OFFSET_INO,
    OFFSET_FAMILY,
    OFFSET_RTT,
    OFFSET_RTTVAR,
    OFFSET_DADDR_IPV6,
    OFFSET_SADDR_FL4,
    OFFSET_DADDR_FL4,
    OFFSET_SPORT_FL4,
    OFFSET_DPORT_FL4,
    OFFSET_SADDR_FL6,
    OFFSET_DADDR_FL6,
    OFFSET_SPORT_FL6,
    OFFSET_DPORT_FL6,
    OFFSET_SOCKET_SK,
    OFFSET_SK_BUFF_SOCK,
    OFFSET_SK_BUFF_TRANSPORT_HEADER,
    OFFSET_SK_BUFF_HEAD,
} offset_t;

BPF_HASH_MAP(offsets, offset_t, u64, 1024)

SEC("kprobe/tcp_getsockopt")
int kprobe__tcp_getsockopt(struct pt_regs* ctx) {
    u64 offset = 0;
    offset_t o = OFFSET_SADDR;
    offset = offsetof(struct sock, sk_rcv_saddr);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

    o = OFFSET_DADDR;
    offset = offsetof(struct sock, sk_daddr);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

    o = OFFSET_FAMILY;
    offset = offsetof(struct sock, sk_family);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

    o = OFFSET_SPORT;
    offset = offsetof(struct inet_sock, inet_sport);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

    o = OFFSET_DPORT;
    offset = offsetof(struct sock, sk_dport);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

#ifdef CONFIG_NET_NS
    o = OFFSET_NETNS;
    offset = offsetof(struct sock, sk_net);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);
    o = OFFSET_INO;
#if defined(_LINUX_NS_COMMON_H)
    offset = offsetof(struct net, ns);
    offset += offsetof(struct ns_common, inum);
#else
    offset = offsetof(struct net, proc_inum);
#endif
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);
#endif

    o = OFFSET_RTT;
    offset = offsetof(struct tcp_sock, srtt_us);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

    o = OFFSET_RTTVAR;
    offset = offsetof(struct tcp_sock, mdev_us);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

#ifdef FEATURE_IPV6_ENABLED
    o = OFFSET_DADDR_IPV6;
    offset = offsetof(struct sock, sk_v6_daddr);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);
#endif

    o = OFFSET_SADDR_FL4;
    offset = offsetof(struct flowi4, saddr);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

    o = OFFSET_DADDR_FL4;
    offset = offsetof(struct flowi4, daddr);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

    o = OFFSET_SPORT_FL4;
    offset = offsetof(struct flowi4, fl4_sport);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

    o = OFFSET_DPORT_FL4;
    offset = offsetof(struct flowi4, fl4_dport);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

#ifdef FEATURE_IPV6_ENABLED
    o = OFFSET_SADDR_FL6;
    offset = offsetof(struct flowi6, saddr);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

    o = OFFSET_DADDR_FL6;
    offset = offsetof(struct flowi6, daddr);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

    o = OFFSET_SPORT_FL6;
    offset = offsetof(struct flowi6, fl6_sport);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

    o = OFFSET_DPORT_FL6;
    offset = offsetof(struct flowi6, fl6_dport);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);
#endif

    o = OFFSET_SOCKET_SK;
    offset = offsetof(struct socket, sk);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

    o = OFFSET_SK_BUFF_SOCK;
    offset = offsetof(struct sk_buff, sk);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

    o = OFFSET_SK_BUFF_TRANSPORT_HEADER;
    offset = offsetof(struct sk_buff, network_header) - 2;
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

    o = OFFSET_SK_BUFF_HEAD;
    offset = offsetof(struct sk_buff, head);
    bpf_map_update_elem(&offsets, &o, &offset, BPF_ANY);

    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
