#ifndef _HOOKS_NETWORK_FLOW_H_
#define _HOOKS_NETWORK_FLOW_H_

#include "constants/offsets/network.h"
#include "constants/offsets/netns.h"
#include "helpers/network/pid_resolver.h"
#include "helpers/network/utils.h"

HOOK_ENTRY("security_sk_classify_flow")
int hook_security_sk_classify_flow(ctx_t *ctx) {
    struct sock *sk = (struct sock *)CTX_PARM1(ctx);
    struct flowi *fl = (struct flowi *)CTX_PARM2(ctx);
    struct pid_route_t key = {};
    struct pid_route_entry_t value = {};
    union flowi_uli uli;

    bpf_get_current_comm(&value.comm, sizeof(value.comm));

    u16 family = get_family_from_sock_common((void *)sk);
    if (family == AF_INET6) {
        bpf_probe_read(&key.addr, sizeof(u64) * 2, (void *)fl + get_flowi6_saddr_offset());
        // TODO: fill l4_protocol, but wait for implementation on security_socket_bind to be ready first
//        bpf_probe_read(&key.l4_protocol, 1, (void *)fl + get_flowi6_proto_offset());
        bpf_probe_read(&value.family, 1, (void *)fl + get_flowi6_proto_offset());
        bpf_probe_read(&uli, sizeof(uli), (void *)fl + get_flowi6_uli_offset());
        bpf_probe_read(&key.port, sizeof(key.port), &uli.ports.sport);
        bpf_probe_read(&value.dport, sizeof(value.dport), &uli.ports.dport);
    } else if (family == AF_INET) {
        bpf_probe_read(&key.addr, sizeof(u32), (void *)fl + get_flowi4_saddr_offset());
        // TODO: fill l4_protocol, but wait for implementation on security_socket_bind to be ready first
//        bpf_probe_read(&key.l4_protocol, 1, (void *)fl + get_flowi4_proto_offset());
        bpf_probe_read(&value.family, 1, (void *)fl + get_flowi4_proto_offset());
        bpf_probe_read(&uli, sizeof(uli), (void *)fl + get_flowi4_uli_offset());
        bpf_probe_read(&key.port, sizeof(key.port), &uli.ports.sport);
        bpf_probe_read(&value.dport, sizeof(value.dport), &uli.ports.dport);
    } else {
        return 0;
    }

    // Register service PID
    if (key.port != 0) {
        u64 id = bpf_get_current_pid_tgid();
        u32 tid = (u32)id;
        value.pid = id >> 32;
        value.type = FLOW_CLASSIFICATION_ENTRY;

        // add netns information
        key.netns = get_netns_from_sock(sk);
        if (key.netns != 0) {
            bpf_map_update_elem(&netns_cache, &tid, &key.netns, BPF_ANY);
        }

        if (value.pid == 392383) {
            bpf_printk("FLOW ! pid:%d netns:%u state:%d\n", value.pid, key.netns);
            bpf_printk("         p:%d a:%d a:%d", key.port, key.addr[0], key.addr[1]);
        }

        // check if the socket already has an active flow
        struct pid_route_t *existing_route = bpf_sk_storage_get(&sock_active_pid_route, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
        if (existing_route != NULL) {
            if (existing_route->port != 0 || existing_route->addr[0] != 0 || existing_route->addr[1] != 0) {
                bpf_printk("flushing previous entry p:%d a:%d a:%d ...\n", existing_route->port, existing_route->addr[0], existing_route->addr[1]);

                // delete existing entry
                bpf_map_delete_elem(&flow_pid, existing_route);
                existing_route->addr[0] = 0;
                existing_route->addr[1] = 0;
                bpf_map_delete_elem(&flow_pid, existing_route);
            }

            // register the new one in the sock_active_pid_route map
            *existing_route = key;
        }

        bpf_map_update_elem(&flow_pid, &key, &value, BPF_ANY);

#if defined(DEBUG_NETWORK_FLOW)
        bpf_printk("# registered (flow) pid:%d netns:%u", value.pid, key.netns);
        bpf_printk("# p:%d a:%d a:%d", key.port, key.addr[0], key.addr[1]);
#endif
    }
    return 0;
}

__attribute__((always_inline)) int trace_nat_manip_pkt(struct nf_conn *ct) {
    u32 netns = get_netns_from_nf_conn(ct);

    struct nf_conntrack_tuple_hash tuplehash[IP_CT_DIR_MAX];
    bpf_probe_read(&tuplehash, sizeof(tuplehash), &ct->tuplehash);

    struct nf_conntrack_tuple *orig_tuple = &tuplehash[IP_CT_DIR_ORIGINAL].tuple;
    struct nf_conntrack_tuple *reply_tuple = &tuplehash[IP_CT_DIR_REPLY].tuple;

    // parse nat flows
    struct namespaced_flow_t orig = {
        .netns = netns,
    };
    struct namespaced_flow_t reply = {
        .netns = netns,
    };
    parse_tuple(orig_tuple, &orig.flow);
    parse_tuple(reply_tuple, &reply.flow);

    // save nat translation:
    //   - flip(reply) should be mapped to orig
    //   - reply should be mapped to flip(orig)
    flip(&reply.flow);
    bpf_map_update_elem(&conntrack, &reply, &orig, BPF_ANY);
    flip(&reply.flow);
    flip(&orig.flow);
    bpf_map_update_elem(&conntrack, &reply, &orig, BPF_ANY);
    return 0;
}

HOOK_ENTRY("nf_nat_manip_pkt")
int hook_nf_nat_manip_pkt(ctx_t *ctx) {
    struct nf_conn *ct = (struct nf_conn *)CTX_PARM2(ctx);
    return trace_nat_manip_pkt(ct);
}

HOOK_ENTRY("nf_nat_packet")
int hook_nf_nat_packet(ctx_t *ctx) {
    struct nf_conn *ct = (struct nf_conn *)CTX_PARM1(ctx);
    return trace_nat_manip_pkt(ct);
}

__attribute__((always_inline)) void fill_pid_route_from_sflow(struct pid_route_t *route, struct namespaced_flow_t *ns_flow) {
    route->addr[0] = ns_flow->flow.saddr[0];
    route->addr[1] = ns_flow->flow.saddr[1];
    route->port = ns_flow->flow.sport;
    route->netns = ns_flow->netns;
}

HOOK_ENTRY("nf_ct_delete")
int hook_nf_ct_delete(ctx_t *ctx) {
    struct nf_conn *ct = (struct nf_conn *)CTX_PARM1(ctx);
    u32 netns = get_netns_from_nf_conn(ct);
    u32 pid = bpf_get_current_pid_tgid() >> 32;

    struct nf_conntrack_tuple_hash tuplehash[IP_CT_DIR_MAX];
    bpf_probe_read(&tuplehash, sizeof(tuplehash), &ct->tuplehash);
    struct nf_conntrack_tuple *orig_tuple = &tuplehash[IP_CT_DIR_ORIGINAL].tuple;
    struct nf_conntrack_tuple *reply_tuple = &tuplehash[IP_CT_DIR_REPLY].tuple;

    // parse nat flows
    struct namespaced_flow_t orig = {
        .netns = netns,
    };
    struct namespaced_flow_t reply = {
        .netns = netns,
    };
    parse_tuple(orig_tuple, &orig.flow);
    parse_tuple(reply_tuple, &reply.flow);

    if (pid == 392383) {
        bpf_printk("CALLED\n");
        bpf_printk(" - src p:%d a:%d a:%d", orig.flow.sport, orig.flow.saddr[0], orig.flow.saddr[1]);
        bpf_printk(" - dst p:%d a:%d a:%d", orig.flow.dport, orig.flow.daddr[0], orig.flow.daddr[1]);
    }

    // clean up entries in the conntrack map
    bpf_map_delete_elem(&conntrack, &reply);
    flip(&reply.flow);
    bpf_map_delete_elem(&conntrack, &reply);

    // Between NAT operations and network direction, both `orig` and `reply` could hold entries
    // in `flow_pid`, clean up all matching non-"BIND_ENTRY" entries.
    struct pid_route_t route = {};

    // start with orig
    fill_pid_route_from_sflow(&route, &orig);
    struct pid_route_entry_t *value = bpf_map_lookup_elem(&flow_pid, &route);
    if (value != NULL) {
        if (value->type == FLOW_CLASSIFICATION_ENTRY) {
            bpf_map_delete_elem(&flow_pid, &route);
        }
    } else {
        // try with no IP
        route.addr[0] = 0;
        route.addr[1] = 0;
        value = bpf_map_lookup_elem(&flow_pid, &route);
        if (value != NULL) {
            if (value->type == FLOW_CLASSIFICATION_ENTRY) {
                bpf_map_delete_elem(&flow_pid, &route);
            }
        }
    }

    // flip orig and try again
    flip(&orig.flow);
    fill_pid_route_from_sflow(&route, &orig);
    value = bpf_map_lookup_elem(&flow_pid, &route);
    if (value != NULL) {
        if (value->type == FLOW_CLASSIFICATION_ENTRY) {
            bpf_map_delete_elem(&flow_pid, &route);
        }
    } else {
         // try with no IP
         route.addr[0] = 0;
         route.addr[1] = 0;
         value = bpf_map_lookup_elem(&flow_pid, &route);
         if (value != NULL) {
             if (value->type == FLOW_CLASSIFICATION_ENTRY) {
                 bpf_map_delete_elem(&flow_pid, &route);
             }
         }
     }

    // reply
    fill_pid_route_from_sflow(&route, &reply);
    value = bpf_map_lookup_elem(&flow_pid, &route);
    if (value != NULL) {
        if (value->type == FLOW_CLASSIFICATION_ENTRY) {
            bpf_map_delete_elem(&flow_pid, &route);
        }
    } else {
         // try with no IP
         route.addr[0] = 0;
         route.addr[1] = 0;
         value = bpf_map_lookup_elem(&flow_pid, &route);
         if (value != NULL) {
             if (value->type == FLOW_CLASSIFICATION_ENTRY) {
                 bpf_map_delete_elem(&flow_pid, &route);
             }
         }
     }

    // flip reply and try again
    flip(&reply.flow);
    fill_pid_route_from_sflow(&route, &reply);
    value = bpf_map_lookup_elem(&flow_pid, &route);
    if (value != NULL) {
        if (value->type == FLOW_CLASSIFICATION_ENTRY) {
            bpf_map_delete_elem(&flow_pid, &route);
        }
    } else {
         // try with no IP
         route.addr[0] = 0;
         route.addr[1] = 0;
         value = bpf_map_lookup_elem(&flow_pid, &route);
         if (value != NULL) {
             if (value->type == FLOW_CLASSIFICATION_ENTRY) {
                 bpf_map_delete_elem(&flow_pid, &route);
             }
         }
     }

    return 0;
}

__attribute__((always_inline)) int handle_sk_release(struct sock *sk) {
    struct pid_route_t route = {};

    // copy netns
    route.netns = get_netns_from_sock(sk);
    if (route.netns == 0) {
        return 0;
    }

    // copy ipv4 / ipv6
    u16 family = 0;
    bpf_probe_read(&family, sizeof(family), &sk->__sk_common.skc_family);
    if (family == AF_INET) {
        bpf_probe_read(&route.addr, sizeof(sk->__sk_common.skc_rcv_saddr), &sk->__sk_common.skc_rcv_saddr);
    } else if (family == AF_INET6) {
        bpf_probe_read(&route.addr, sizeof(u64) * 2, &sk->__sk_common.skc_v6_rcv_saddr);
    } else {
        return 0;
    }

    // copy port
    bpf_probe_read(&route.port, sizeof(route.port), &sk->__sk_common.skc_num);
    route.port = htons(route.port);

    // clean up flow_pid entry
    bpf_map_delete_elem(&flow_pid, &route);
    // also clean up empty entry if it exists
    route.addr[0] = 0;
    route.addr[1] = 0;
    bpf_map_delete_elem(&flow_pid, &route);
    return 0;
}

// for kernel-initiated socket cleanup (timeout or error)
HOOK_ENTRY("sk_common_release")
int hook_sk_common_release(ctx_t *ctx) {
    struct sock *sk = (struct sock *)CTX_PARM1(ctx);
    if (sk == NULL) {
        return 0;
    }
    return handle_sk_release(sk);
}

// UDP specific termination
HOOK_ENTRY("udp_destroy_sock")
int hook_udp_destroy_sock(ctx_t *ctx) {
    struct sock *sk = (struct sock *)CTX_PARM1(ctx);
    if (sk == NULL) {
        return 0;
    }
    return handle_sk_release(sk);
}

// UDP6 specific termination
HOOK_ENTRY("udpv6_destroy_sock")
int hook_udpv6_destroy_sock(ctx_t *ctx) {
    struct sock *sk = (struct sock *)CTX_PARM1(ctx);
    if (sk == NULL) {
        return 0;
    }
    return handle_sk_release(sk);
}

// TCP specific termination
HOOK_ENTRY("tcp_v4_destroy_sock")
int hook_tcp_v4_destroy_sock(ctx_t *ctx) {
    struct sock *sk = (struct sock *)CTX_PARM1(ctx);
    if (sk == NULL) {
        return 0;
    }
    return handle_sk_release(sk);
}

// TCP specific termination
HOOK_ENTRY("tcp_shutdown")
int hook_tcp_shutdown(ctx_t *ctx) {
    struct sock *sk = (struct sock *)CTX_PARM1(ctx);
    if (sk == NULL) {
        return 0;
    }
    return handle_sk_release(sk);
}

// TCP specific termination
HOOK_ENTRY("tcp_close")
int hook_tcp_close(ctx_t *ctx) {
    struct sock *sk = (struct sock *)CTX_PARM1(ctx);
    if (sk == NULL) {
        return 0;
    }
    return handle_sk_release(sk);
}

// for user-space initiated socket shutdown
HOOK_ENTRY("inet_shutdown")
int hook_inet_shutdown(ctx_t *ctx) {
    struct socket *sock = (struct socket *)CTX_PARM1(ctx);
    struct sock *sk;
    bpf_probe_read(&sk, sizeof(sk), &sock->sk);
    if (sk == NULL) {
        return 0;
    }

    return handle_sk_release(sk);
}

// for user space initiated socket termination
HOOK_ENTRY("inet_release")
int hook_inet_release(ctx_t *ctx) {
    struct socket *sock = (struct socket *)CTX_PARM1(ctx);
    struct sock *sk;
    bpf_probe_read(&sk, sizeof(sk), &sock->sk);
    if (sk == NULL) {
        return 0;
    }

    return handle_sk_release(sk);
}

SEC("fexit/inet_bind")
int BPF_PROG(hook_inet_bind, struct socket *sock, struct sockaddr *uaddr, int addr_len, int ret) {
    if (ret < 0) {
        // we only care about successful bind operations
        return 0;
    }

    struct sock *sk;
    bpf_probe_read(&sk, sizeof(sk), &sock->sk);
    if (sk == NULL) {
        return 0;
    }
    struct pid_route_t route = {};
    struct pid_route_entry_t value = {};
    bpf_get_current_comm(&value.comm, sizeof(value.comm));
    value.type = 3;
    u64 id = bpf_get_current_pid_tgid();
    u32 tid = (u32)id;

    // add netns information
    route.netns = get_netns_from_sock(sk);
    if (route.netns != 0) {
        bpf_map_update_elem(&netns_cache, &tid, &route.netns, BPF_ANY);
    }

    // copy ipv4 / ipv6
    u16 family = 0;
    bpf_probe_read(&family, sizeof(family), &sk->__sk_common.skc_family);
    if (family == AF_INET) {
        bpf_probe_read(&route.addr, sizeof(sk->__sk_common.skc_rcv_saddr), &sk->__sk_common.skc_rcv_saddr);
    } else if (family == AF_INET6) {
        bpf_probe_read(&route.addr, sizeof(u64) * 2, &sk->__sk_common.skc_v6_rcv_saddr);
    } else {
        // we don't care about non IPv4 / IPV6 flows
        return 0;
    }

    // copy port
    bpf_probe_read(&route.port, sizeof(route.port), &sk->__sk_common.skc_num);
    route.port = htons(route.port);

    // Register service PID
    if (route.port > 0) {
        value.pid = id >> 32;
        bpf_map_update_elem(&flow_pid, &route, &value, BPF_ANY);
    }
    return 0;
}

#endif
