#ifndef __NETNS_H
#define __NETNS_H

#if defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)

#include "bpf_core_read.h"
#include "bpf_telemetry.h"

#ifdef COMPILE_RUNTIME
#include <net/net_namespace.h>
#include <net/sock.h>
#endif

static __always_inline u32 get_netns_from_ct_net(struct net* ct_net) {
    u32 net_ns_inum = 0;
#ifdef CONFIG_NET_NS
#ifdef _LINUX_NS_COMMON_H
    BPF_CORE_READ_INTO(&net_ns_inum, ct_net, ns.inum);
#else
    BPF_CORE_READ_INTO(&net_ns_inum, ct_net, proc_inum);
#endif
#endif
    return net_ns_inum;
}

static __maybe_unused __always_inline u32 get_netns_from_sock(struct sock *sk) {
    struct net *ct_net = NULL;
#ifdef CONFIG_NET_NS
    BPF_CORE_READ_INTO(&ct_net, sk, sk_net);
#endif
    return get_netns_from_ct_net(ct_net);
}

// depending on the kernel version p_net may be a struct net** or possible_net_t*
static __always_inline u32 get_netns(void *p_net) {
    struct net *ct_net = NULL;
#ifdef CONFIG_NET_NS
    bpf_probe_read_with_telemetry(&ct_net, sizeof(ct_net), p_net);
#endif
    return get_netns_from_ct_net(ct_net);
}

#endif // defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)

#endif
