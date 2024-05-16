#ifndef _CONSTANTS_OFFSETS_NETNS_H_
#define _CONSTANTS_OFFSETS_NETNS_H_

#include "constants/macros.h"

__attribute__((always_inline)) u32 get_ifindex_from_net_device(struct net_device *device) {
    u64 net_device_ifindex_offset;
    LOAD_CONSTANT("net_device_ifindex_offset", net_device_ifindex_offset);

    u32 ifindex;
    bpf_probe_read(&ifindex, sizeof(ifindex), (void*)device + net_device_ifindex_offset);
    return ifindex;
}

__attribute__((always_inline)) char* get_net_device_name(struct net_device *device) {
    u64 net_device_name_offset;
    LOAD_CONSTANT("net_device_name_offset", net_device_name_offset);

    return (char *)((void *)device + net_device_name_offset);
}

#define NET_STRUCT_HAS_PROC_INUM 0
#define NET_STRUCT_HAS_NS        1

__attribute__((always_inline)) u32 get_netns_from_net(struct net *net) {
    u64 net_struct_type;
    LOAD_CONSTANT("net_struct_type", net_struct_type);
    u64 net_proc_inum_offset;
    LOAD_CONSTANT("net_proc_inum_offset", net_proc_inum_offset);
    u64 net_ns_offset;
    LOAD_CONSTANT("net_ns_offset", net_ns_offset);

    if (net_struct_type == NET_STRUCT_HAS_PROC_INUM) {
        u32 inum = 0;
        bpf_probe_read(&inum, sizeof(inum), (void*)net + net_proc_inum_offset);
        return inum;
    }

#ifndef DO_NOT_USE_TC
    struct ns_common ns;
    bpf_probe_read(&ns, sizeof(ns), (void*)net + net_ns_offset);
    return ns.inum;
#else
    return 0;
#endif
}

__attribute__((always_inline)) u32 get_netns_from_net_device(struct net_device *device) {
    u64 device_nd_net_net_offset;
    LOAD_CONSTANT("device_nd_net_net_offset", device_nd_net_net_offset);

    // no constant
    if (device_nd_net_net_offset == -1) {
        return 0;
    }

    struct net *net = NULL;
    bpf_probe_read(&net, sizeof(net), (void *)device + device_nd_net_net_offset);
    return get_netns_from_net(net);
}

__attribute__((always_inline)) u32 get_netns_from_sock(struct sock *sk) {
    u64 sock_common_skc_net_offset;
    LOAD_CONSTANT("sock_common_skc_net_offset", sock_common_skc_net_offset);

    struct sock_common *common = (void *)sk;
    struct net *net = NULL;
    bpf_probe_read(&net, sizeof(net), (void *)common + sock_common_skc_net_offset);
    return get_netns_from_net(net);
}

__attribute__((always_inline)) u32 get_netns_from_socket(struct socket *socket) {
    u64 socket_sock_offset;
    LOAD_CONSTANT("socket_sock_offset", socket_sock_offset);

    struct sock *sk = NULL;
    bpf_probe_read(&sk, sizeof(sk), (void *)socket + socket_sock_offset);
    return get_netns_from_sock(sk);
}

__attribute__((always_inline)) u32 get_netns_from_nf_conn(struct nf_conn *ct) {
    u64 nf_conn_ct_net_offset;
    LOAD_CONSTANT("nf_conn_ct_net_offset", nf_conn_ct_net_offset);

    struct net *net = NULL;
    bpf_probe_read(&net, sizeof(net), (void *)ct + nf_conn_ct_net_offset);
    return get_netns_from_net(net);
}

#endif
