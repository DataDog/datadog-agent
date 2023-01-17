#ifndef __NETNS_H
#define __NETNS_H

#include <net/net_namespace.h>

// depending on the kernel version p_net may be a struct net** or possible_net_t*
static __always_inline u32 get_netns(void *p_net) {
    u32 net_ns_inum = 0;
#ifdef CONFIG_NET_NS
    struct net *ct_net = NULL;
    bpf_probe_read_kernel_with_telemetry(&ct_net, sizeof(ct_net), p_net);
    #ifdef _LINUX_NS_COMMON_H
        bpf_probe_read_kernel_with_telemetry(&net_ns_inum, sizeof(net_ns_inum), &ct_net->ns.inum);
    #else
        bpf_probe_read_kernel_with_telemetry(&net_ns_inum, sizeof(net_ns_inum), &ct_net->proc_inum);
    #endif
#endif
    return net_ns_inum;
}

#endif
