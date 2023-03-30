#ifndef __NETNS_H
#define __NETNS_H

#include "bpf_core_read.h"
#include "bpf_telemetry.h"

#ifdef COMPILE_RUNTIME
#include <net/net_namespace.h>
#include <net/sock.h>
#endif

#ifdef COMPILE_CORE
#define sk_net __sk_common.skc_net
#endif

#ifdef COMPILE_PREBUILT

static __always_inline __u32 get_netns_from_sock(struct sock* sk) {
    void* skc_net = NULL;
    __u32 net_ns_inum = 0;
    bpf_probe_read_kernel_with_telemetry(&skc_net, sizeof(void*), ((char*)sk) + offset_netns());
    bpf_probe_read_kernel_with_telemetry(&net_ns_inum, sizeof(net_ns_inum), ((char*)skc_net) + offset_ino());
    return net_ns_inum;
}

#endif // COMPILE_PREBUILT

#ifdef COMPILE_CORE

struct net___old {
    unsigned int proc_inum;
};

struct sock_common___old {
    struct net *skc_net;
};

struct sock___old {
    struct sock_common___old __sk_common;
};

static __always_inline __u32 get_netns_from_sock(struct sock* sk) {
    u32 net_ns_inum = 0;
    if (bpf_core_field_exists(sk->sk_net.net)) {
        BPF_CORE_READ_INTO(&net_ns_inum, sk, sk_net.net, ns.inum);
    } else {
        struct sock___old *oldsk = (struct sock___old*)sk;
        if (bpf_core_field_exists(oldsk->sk_net->ns)) {
            BPF_CORE_READ_INTO(&net_ns_inum, oldsk, sk_net, ns.inum);
        } else {
            struct net___old *ns = NULL;
            BPF_CORE_READ_INTO(&ns, oldsk, sk_net);
            BPF_CORE_READ_INTO(&net_ns_inum, ns, proc_inum);
        }
    }
    return net_ns_inum;
}

#endif // COMPILE_CORE

#ifdef COMPILE_RUNTIME

static __maybe_unused __always_inline u32 get_netns_from_sock(struct sock *sk) {
    // Retrieve network namespace id
    //
    // `possible_net_t skc_net`
    // replaced
    // `struct net *skc_net`
    // https://github.com/torvalds/linux/commit/0c5c9fb55106333e773de8c9dd321fa8240caeb3
    u32 net_ns_inum = 0;
#ifdef CONFIG_NET_NS
#ifdef _LINUX_NS_COMMON_H
    BPF_PROBE_READ_INTO(&net_ns_inum, sk, sk_net.net, ns.inum);
#else
    BPF_PROBE_READ_INTO(&net_ns_inum, sk, sk_net, proc_inum);
#endif // LINUX_NS_COMMON_H
#endif // CONFIG_NET_NS

    return net_ns_inum;
}

#endif // COMPILE_RUNTIME

#endif
