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
#include <net/sock.h>
static __always_inline __u32 get_netns_from_sock(struct sock* sk) {
    void* skc_net = NULL;
    __u32 net_ns_inum = 0;
    bpf_probe_read_kernel_with_telemetry(&skc_net, sizeof(void*), ((char*)sk) + offset_netns());
    bpf_probe_read_kernel_with_telemetry(&net_ns_inum, sizeof(net_ns_inum), ((char*)skc_net) + offset_ino());
    return net_ns_inum;
}

#endif // COMPILE_PREBUILT

#ifdef COMPILE_CORE

struct nf_conn___old {
    struct net *ct_net;
};

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
    struct net *ns = NULL;
    if (bpf_core_field_exists(sk->sk_net.net) ||
        bpf_core_field_exists(((struct sock___old*)sk)->sk_net->ns)) {
        BPF_CORE_READ_INTO(&ns, sk, sk_net);
        BPF_CORE_READ_INTO(&net_ns_inum, ns, ns.inum);
    } else if (bpf_core_field_exists(((struct net___old*)ns)->proc_inum)) {
        BPF_CORE_READ_INTO(&ns, (struct sock___old*)sk, sk_net);
        BPF_CORE_READ_INTO(&net_ns_inum, (struct net___old*)ns, proc_inum);
    }
    return net_ns_inum;
}

// get_netns_from_sock for a BTF-typed sock pointer (a fentry/fexit/tp_btf argument, or a pointer loaded
// directly from one): the verifier turns the loads into BPF_PROBE_MEM loads. The kernels that can run those
// programs all have possible_net_t; any other layout takes the probe-read version above.
static __maybe_unused __always_inline __u32 get_netns_from_sock_btf(struct sock* sk) {
    if (bpf_core_field_exists(sk->sk_net.net)) {
        struct net *ns = sk->sk_net.net;
        return ns->ns.inum;
    }
    return get_netns_from_sock(sk);
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
    struct net *ns = NULL;
    BPF_PROBE_READ_INTO(&ns, sk, sk_net);
#ifdef _LINUX_NS_COMMON_H
    BPF_PROBE_READ_INTO(&net_ns_inum, ns, ns.inum);
#else
    BPF_PROBE_READ_INTO(&net_ns_inum, ns, proc_inum);
#endif // LINUX_NS_COMMON_H
#endif // CONFIG_NET_NS

    return net_ns_inum;
}

#endif // COMPILE_RUNTIME

#endif
