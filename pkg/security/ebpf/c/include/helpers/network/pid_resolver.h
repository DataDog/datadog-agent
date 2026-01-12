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

__attribute__((always_inline)) void resolve_pid(struct __sk_buff *skb, struct packet_t *pkt) {
    // pid from socket cookie
    u64 cookie = bpf_get_socket_cookie(skb);
    u32 *pid = bpf_map_lookup_elem(&sock_cookie_pid, &cookie);
    if (pid) {
        pkt->pid = *pid;
    }

    // pid from sched_cls
    if (pkt->pid == 0) {
        u64 sched_cls_has_current_pid_tgid_helper = 0;
        LOAD_CONSTANT("sched_cls_has_current_pid_tgid_helper", sched_cls_has_current_pid_tgid_helper);
        if (sched_cls_has_current_pid_tgid_helper) {
            u64 pid_tgid = bpf_get_current_pid_tgid();
            pkt->pid = pid_tgid >> 32;
        }
    }

    // pid from flow pid
    if (pkt->pid == 0) {
        resolve_pid_from_flow_pid(pkt);
    }
}

#endif
