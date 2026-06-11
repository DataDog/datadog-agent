#ifndef _HELPERS_NETWORK_PID_RESOLVER_H_
#define _HELPERS_NETWORK_PID_RESOLVER_H_

#include "maps.h"

__attribute__((always_inline)) s64 get_flow_pid(struct pid_route_t *key) {
    struct pid_route_entry_t *value = bpf_map_lookup_elem(&flow_pid, key);
    if (!value) {
        // Try with IP set to 0.0.0.0
        key->addr[0] = 0;
        key->addr[1] = 0;
        value = bpf_map_lookup_elem(&flow_pid, key);
        if (!value) {
            return 0;
        }
    }

    return value->pid;
}

// resolve_pid_from_flow_pid resolves the pid from the flow_pid map, keyed by
// {address, port, netns, protocol}. That key does not uniquely identify a socket, so get_flow_pid
// can return the wrong pid on a key collision, in particular when the same address and port are
// reused by a socket living in another network namespace. It is kept only as a fallback on older
// kernels that lack bpf_sk_lookup, sk-local storage, or the cgroup socket hook; the sk_lookup path
// is preferred because it identifies the exact owning socket.
__attribute__((always_inline)) void resolve_pid_from_flow_pid(struct packet_t *pkt) {
    struct pid_route_t pid_route = {};

    // resolve pid
    switch (pkt->network_direction) {
    case EGRESS: {
        pid_route.addr[0] = pkt->translated_ns_flow.flow.saddr[0];
        pid_route.addr[1] = pkt->translated_ns_flow.flow.saddr[1];
        pid_route.port = pkt->translated_ns_flow.flow.tcp_udp.sport;
        pid_route.netns = pkt->translated_ns_flow.netns;
        break;
    }
    case INGRESS: {
        pid_route.addr[0] = pkt->translated_ns_flow.flow.daddr[0];
        pid_route.addr[1] = pkt->translated_ns_flow.flow.daddr[1];
        pid_route.port = pkt->translated_ns_flow.flow.tcp_udp.dport;
        pid_route.netns = pkt->translated_ns_flow.netns;
        break;
    }
    }

    pid_route.l4_protocol = pkt->translated_ns_flow.flow.l4_protocol;
    pkt->pid = get_flow_pid(&pid_route);

    #if defined(DEBUG_NETWORK_FLOW)
    bpf_printk("Lookup: ip: %lu %lu port: %d", pid_route.addr[0], pid_route.addr[1], htons(pid_route.port));
    bpf_printk("        netns: %lu, protocol: %d", pid_route.netns, pid_route.l4_protocol);
    bpf_printk("        pid: %lu", pkt->pid);
    #endif
}

// resolve_pid_from_sk_lookup resolves the pid of a packet by looking up the owning socket in the
// network namespace of the device the skb is on, and reading the pid recorded in sk-local storage
// by the socket lifecycle hooks. Because it resolves the exact socket (instead of matching a
// {address, port, netns} key like the flow_pid map), it cannot misattribute a packet to a different
// socket that happens to reuse the same address and port in another network namespace.
__attribute__((always_inline)) void resolve_pid_from_sk_lookup(struct __sk_buff *skb, struct packet_t *pkt) {
    struct namespaced_flow_t *nsf = &pkt->translated_ns_flow;
    u16 l4_protocol = nsf->flow.l4_protocol;
    if (l4_protocol != IPPROTO_TCP && l4_protocol != IPPROTO_UDP) {
        // a socket lookup only makes sense for TCP and UDP
        return;
    }

    // Build the lookup tuple so that the local socket side ends up in daddr/dport: the kernel
    // socket table is keyed on the local side. On EGRESS the local side is the packet source,
    // on INGRESS it is the packet destination. We use the conntrack-translated flow so the
    // tuple matches the address the local socket is actually bound/connected to.
    u64 *local_addr, *remote_addr;
    u16 local_port, remote_port;
    if (pkt->network_direction == EGRESS) {
        local_addr = nsf->flow.saddr;
        remote_addr = nsf->flow.daddr;
        local_port = nsf->flow.tcp_udp.sport;
        remote_port = nsf->flow.tcp_udp.dport;
    } else {
        local_addr = nsf->flow.daddr;
        remote_addr = nsf->flow.saddr;
        local_port = nsf->flow.tcp_udp.dport;
        remote_port = nsf->flow.tcp_udp.sport;
    }

    struct bpf_sock_tuple tuple = {};
    u32 tuple_size;
    if (nsf->flow.l3_protocol == ETH_P_IP) {
        tuple.ipv4.saddr = (u32)remote_addr[0];
        tuple.ipv4.daddr = (u32)local_addr[0];
        tuple.ipv4.sport = remote_port;
        tuple.ipv4.dport = local_port;
        tuple_size = sizeof(tuple.ipv4);
    } else if (nsf->flow.l3_protocol == ETH_P_IPV6) {
        __builtin_memcpy(&tuple.ipv6.saddr, remote_addr, sizeof(tuple.ipv6.saddr));
        __builtin_memcpy(&tuple.ipv6.daddr, local_addr, sizeof(tuple.ipv6.daddr));
        tuple.ipv6.sport = remote_port;
        tuple.ipv6.dport = local_port;
        tuple_size = sizeof(tuple.ipv6);
    } else {
        return;
    }

    // BPF_F_CURRENT_NETNS performs the lookup in the network namespace of the device the skb is on
    // (the namespace this TC program is attached to). Combined with the packet tuple, it resolves
    // the exact owning socket, which the flow_pid {address, port, netns} key could not do reliably.
    struct bpf_sock *sk = NULL;
    if (l4_protocol == IPPROTO_TCP) {
        sk = bpf_sk_lookup_tcp(skb, &tuple, tuple_size, BPF_F_CURRENT_NETNS, 0);
    } else {
        sk = bpf_sk_lookup_udp(skb, &tuple, tuple_size, BPF_F_CURRENT_NETNS, 0);
    }
    if (sk == NULL) {
        return;
    }

    // The pid owning the socket is recorded in sk-local storage by the socket lifecycle hooks.
    // Reading it from the looked-up socket gives a namespace-correct owner without relying on
    // the flow_pid map.
    u32 *pid = bpf_sk_storage_get(&sk_storage_pid, sk, NULL, 0);
    if (pid != NULL) {
        pkt->pid = *pid;
    }

    // the reference taken by bpf_sk_lookup_* must always be released
    bpf_sk_release(sk);

    #if defined(DEBUG_NETWORK_FLOW)
    bpf_printk("sk_lookup: l4:%d direction:%d pid:%d", l4_protocol, pkt->network_direction, pkt->pid);
    #endif
}

__attribute__((always_inline)) void resolve_pid(struct __sk_buff *skb, struct packet_t *pkt) {
    pkt->pid = 0;
    pkt->cgroup_id = 0;

    int dbg = pkt->l4.udp.dest == htons(5555);

    // pid from socket cookie: set when the skb has an associated socket (mostly on egress)
    u64 cookie = bpf_get_socket_cookie(skb);
    u32 *pid = bpf_map_lookup_elem(&sock_cookie_pid, &cookie);
    if (pid) {
        pkt->pid = *pid;
        if (dbg) {
            bpf_printk("cookie pid: %d", pkt->pid);
        }
    }

    if (pkt->pid == 0 && pkt->network_direction == EGRESS) {
        u64 sched_cls_has_current_pid_tgid_helper = 0;
        LOAD_CONSTANT("sched_cls_has_current_pid_tgid_helper", sched_cls_has_current_pid_tgid_helper);
        if (sched_cls_has_current_pid_tgid_helper) {
            u64 pid_tgid = bpf_get_current_pid_tgid();
            pkt->pid = pid_tgid >> 32;
            if (dbg) {
                bpf_printk("helper pid: %d", pkt->pid);
            }
        }
    }

    if (pkt->pid == 0) {
        if (is_sk_lookup_pid_supported()) {
            // pid from socket lookup: namespace-correct resolution, preferred when available
            resolve_pid_from_sk_lookup(skb, pkt);
            if (dbg) {
                bpf_printk("sk_lookup pid: %d", pkt->pid);
            }
        } else {
            // pid from flow pid: fallback used only when the socket-lookup path is unavailable (older
            // kernels without bpf_sk_lookup, sk-local storage, or the cgroup socket hook)
            resolve_pid_from_flow_pid(pkt);
            if (dbg) {
                bpf_printk("flow pid: %d", pkt->pid);
            }
        }
    }

    if (pkt->pid != 0) {
        // check if the pid is a kworker pid, if so we won't associate a process which is expected
        u32 pid_val = (u32)pkt->pid;
        if (IS_KERNEL_THREAD(pid_val)) {
            pkt->pid = 0;
            if (dbg) {
                bpf_printk("kthread pid: %d", pkt->pid);
            }
        }
    }
}

#endif
