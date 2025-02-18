#ifndef _HOOKS_NETWORK_FLOW_H_
#define _HOOKS_NETWORK_FLOW_H_

#include "constants/offsets/network.h"
#include "constants/offsets/netns.h"
#include "helpers/network/pid_resolver.h"
#include "helpers/network/utils.h"
#include "helpers/network/flow.h"

HOOK_ENTRY("security_sk_classify_flow")
int hook_security_sk_classify_flow(ctx_t *ctx) {
    struct sock *sk = (struct sock *)CTX_PARM1(ctx);
    struct flowi *fl = (struct flowi *)CTX_PARM2(ctx);
    struct pid_route_t key = {};
    struct pid_route_entry_t value = {};
    union flowi_uli uli;

//#if defined(DEBUG_NETWORK_FLOW)
    char state = 0;
    bpf_probe_read(&state, sizeof(state), (void *)&sk->sk_state);
    bpf_printk("security_sk_classify_flow state:%u p:0x%p", state, sk);
//#endif


    // There can be a missmatch between the family of the socket and the family of the flow.
    // The socket can be of AF_INET6, and yet the flow could be AF_INET.
    // See https://man7.org/linux/man-pages/man7/ipv6.7.html for more.

    // In our case, this means that we need to "guess" if the flow is AF_INET or AF_INET6 when the socket is AF_INET6.
    u16 flow_family = get_family_from_sock_common((void *)sk);
    if (flow_family != AF_INET && flow_family != AF_INET6) {
        // ignore these flows for now
        return 0;
    }

    u64 id = bpf_get_current_pid_tgid();
    if (id == 0) {
        // we only care about packet sent from an actual task
        return 0;
    }

    u16 sk_port = get_skc_num_from_sock_common((void *)sk);
    // add netns information
    key.netns = get_netns_from_sock(sk);
    if (flow_family == AF_INET6) {
        // check if the source port of the flow matches with the bound port of the socket
        bpf_probe_read(&uli, sizeof(uli), (void *)fl + get_flowi6_uli_offset());
        bpf_probe_read(&key.port, sizeof(key.port), &uli.ports.sport);

        // if they don't match, then this is likely an AF_INET socket
        if (sk_port != key.port) {
            flow_family = AF_INET;
        } else {
            // this is an AF_INET6 flow
            bpf_probe_read(&key.addr, sizeof(u64) * 2, (void *)fl + get_flowi6_saddr_offset());
            // TODO: fill l4_protocol, but wait for implementation on security_socket_bind to be ready first
            // bpf_probe_read(&key.l4_protocol, 1, (void *)fl + get_flowi6_proto_offset());
        }
    }
    if (flow_family == AF_INET) {
        // make sure the ports match
        bpf_probe_read(&uli, sizeof(uli), (void *)fl + get_flowi4_uli_offset());
        bpf_probe_read(&key.port, sizeof(key.port), &uli.ports.sport);

        // if they don't match, return now, we don't know how to handle this flow
        if (sk_port != key.port) {
            char state = 0;
            bpf_probe_read(&state, sizeof(state), (void *)&sk->sk_state);
            bpf_printk("|    flow_with_no_matching_port state:%u p:0x%p", state, sk);
            print_route(&key);
            bpf_printk("|--> uli.port:%d sk_port:%d", key.port, sk_port);
            return 0;
        } else {
            // This is an AF_INET flow
            bpf_probe_read(&key.addr, sizeof(u32), (void *)fl + get_flowi4_saddr_offset());
            // TODO: fill l4_protocol, but wait for implementation on security_socket_bind to be ready first
            // bpf_probe_read(&key.l4_protocol, 1, (void *)fl + get_flowi4_proto_offset());
        }
    }

//#if defined(DEBUG_NETWORK_FLOW)
    print_route(&key);
//#endif

    // check if the socket already has an active flow
    struct sock_meta_t *meta = get_sock_meta(sk);
    if (meta != NULL) {
        if (meta->existing_route.port != 0 || meta->existing_route.addr[0] != 0 || meta->existing_route.addr[1] != 0) {
            if (can_delete_route(&meta->existing_route, meta)) {
                bpf_printk("|    flushing previous route:");
                print_route(&meta->existing_route);
                bpf_map_delete_elem(&flow_pid, &meta->existing_route);
            }

            // check with an empty IP address
            meta->existing_route.addr[0] = 0;
            meta->existing_route.addr[1] = 0;

            if (can_delete_route(&meta->existing_route, meta)) {
                bpf_printk("|    flushing previous empty route:");
                print_route(&meta->existing_route);
                bpf_map_delete_elem(&flow_pid, &meta->existing_route);
            }
        }

        // register the new one in the sock_active_pid_route map
        meta->existing_route = key;

        bpf_printk("|    socket_closing = %d", meta->socket_closing);
        bpf_printk("|    accept_created_socket = %d", meta->accept_created_socket);

        // is this socket closing ?
        if (meta->socket_closing) {
            bpf_printk("|    should leave early due to socket_closing !");
            // Exit now, we don't want to register a new route for a closing socket.
            // We arrive here when a socket sends a final TCP FIN packet when the socket is closed.
//            return 0;
        }
    } else {
        bpf_printk("|    no sock_meta entry !");
    }

    // Register service PID
    if (key.port != 0) {
        u32 tid = (u32)id;
        value.pid = id >> 32;
        value.type = FLOW_CLASSIFICATION_ENTRY;
        if (meta != NULL) {
            value.added_by_accept_created_socket = meta->accept_created_socket;
        }

        if (key.netns != 0) {
            bpf_map_update_elem(&netns_cache, &tid, &key.netns, BPF_ANY);
        }

        bpf_map_update_elem(&flow_pid, &key, &value, BPF_ANY);

//#if defined(DEBUG_NETWORK_FLOW)
        print_route_entry(&value);
        bpf_printk("|--> new flow registered !", value.pid, key.netns);
//#endif
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

__attribute__((always_inline)) void flush_flow_pid_by_route(struct pid_route_t *route) {
    struct pid_route_entry_t *value = bpf_map_lookup_elem(&flow_pid, route);
    if (value != NULL) {
        if (value->type == FLOW_CLASSIFICATION_ENTRY) {
            bpf_map_delete_elem(&flow_pid, route);
        }
    } else {
        // try with no IP
        route->addr[0] = 0;
        route->addr[1] = 0;
        value = bpf_map_lookup_elem(&flow_pid, route);
        if (value != NULL) {
            if (value->type == FLOW_CLASSIFICATION_ENTRY) {
                bpf_map_delete_elem(&flow_pid, route);
            }
        }
    }
}

HOOK_ENTRY("nf_ct_delete")
int hook_nf_ct_delete(ctx_t *ctx) {
    struct nf_conn *ct = (struct nf_conn *)CTX_PARM1(ctx);
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

#if defined(DEBUG_NETWORK_FLOW)
    bpf_printk("nf_ct_delete");
    bpf_printk(" - src p:%d a:%lu a:%lu", orig.flow.sport, orig.flow.saddr[0], orig.flow.saddr[1]);
    bpf_printk(" - dst p:%d a:%lu a:%lu", orig.flow.dport, orig.flow.daddr[0], orig.flow.daddr[1]);
#endif

    // clean up entries in the conntrack map
    bpf_map_delete_elem(&conntrack, &reply);
    flip(&reply.flow);
    bpf_map_delete_elem(&conntrack, &reply);

    // Between NAT operations and network direction, both `orig` and `reply` could hold entries
    // in `flow_pid`, clean up all matching non-"BIND_ENTRY" entries.
    struct pid_route_t route = {};

    // start with orig
    fill_pid_route_from_sflow(&route, &orig);
    flush_flow_pid_by_route(&route);

    // flip orig and try again
    flip(&orig.flow);
    fill_pid_route_from_sflow(&route, &orig);
    flush_flow_pid_by_route(&route);

    // reply
    fill_pid_route_from_sflow(&route, &reply);
    flush_flow_pid_by_route(&route);

    // flip reply and try again
    flip(&reply.flow);
    fill_pid_route_from_sflow(&route, &reply);
    flush_flow_pid_by_route(&route);

    return 0;
}

__attribute__((always_inline)) int handle_sk_release(struct sock *sk) {
    struct pid_route_t route = {};

    // register that this socket is closing
    struct sock_meta_t *meta = peek_sock_meta(sk);
    if (meta != NULL) {
        meta->socket_closing = 1;

//        // no clean up required if this socket was created by accepting a connection
//        if (meta->accept_created_socket) {
//            bpf_printk("    leaving sk_release due to accept_created_socket: p:0x%p", sk);
//            return 0;
//        }
        print_meta(meta);
    }

    // extract netns
    route.netns = get_netns_from_sock(sk);
    if (route.netns == 0) {
        return 0;
    }

    // extract port
    route.port = get_skc_num_from_sock_common((void *)sk);

    // extract ipv4 / ipv6
    u16 family = get_family_from_sock_common((void *)sk);
    char state = 0;
    bpf_probe_read(&state, sizeof(state), (void *)&sk->sk_state);
    if (family == AF_INET6) {
        bpf_probe_read(&route.addr, sizeof(u64) * 2, &sk->__sk_common.skc_v6_rcv_saddr);

//#if defined(DEBUG_NETWORK_FLOW)
        bpf_printk("|    sk_release_v6: state:%u @:0x%p", state, sk);
        print_route(&route);
//#endif

        if (can_delete_route(&route, meta)) {
            // clean up flow_pid entry
            bpf_printk("|    deleted entry !");
            bpf_map_delete_elem(&flow_pid, &route);
        } else {
            bpf_printk("|    couldn't delete entry !");
        }

        // also clean up empty entry if it exists
        route.addr[0] = 0;
        route.addr[1] = 0;

        if (can_delete_route(&route, meta)) {
            // clean up flow_pid entry
            bpf_printk("|    deleted entry with 0-0 !");
            bpf_map_delete_elem(&flow_pid, &route);
        } else {
            bpf_printk("|    couldn't delete entry with 0-0 !");
        }

        // We might be dealing with an AF_INET traffic over an AF_INET6 socket.
        // To be sure, clean AF_INET entries as well.
        family = AF_INET;
    }
    if (family == AF_INET) {
        bpf_probe_read(&route.addr, sizeof(sk->__sk_common.skc_rcv_saddr), &sk->__sk_common.skc_rcv_saddr);

//#if defined(DEBUG_NETWORK_FLOW)
        bpf_printk("|    sk_release_v4: state:%u @:0x%p", state, sk);
        print_route(&route);
//#endif

        if (can_delete_route(&route, meta)) {
            // clean up flow_pid entry
            bpf_printk("|    deleted entry !");
            bpf_map_delete_elem(&flow_pid, &route);
        } else {
            bpf_printk("|    couldn't delete entry !");
        }

        // also clean up empty entry if it exists
        route.addr[0] = 0;
        route.addr[1] = 0;

        if (can_delete_route(&route, meta)) {
            // clean up flow_pid entry
            bpf_printk("|    deleted entry with 0-0 !");
            bpf_map_delete_elem(&flow_pid, &route);
        } else {
            bpf_printk("|    couldn't delete entry with 0-0 !");
        }
    }

    // Make sure we also cleanup the entry stored in the socket attached metadata.
    if (meta != NULL) {
        if (can_delete_route(&meta->existing_route, meta)) {
            // clean up flow_pid entry
            bpf_printk("|    deleted sock_meta entry !");
            bpf_map_delete_elem(&flow_pid, &meta->existing_route);
        } else {
            bpf_printk("|    couldn't delete sock_meta entry !");
        }

        // also clean up empty entry if it exists
        meta->existing_route.addr[0] = 0;
        meta->existing_route.addr[1] = 0;

        if (can_delete_route(&meta->existing_route, meta)) {
            bpf_printk("|    deleted sock_meta entry with 0-0 !");
            bpf_map_delete_elem(&flow_pid, &meta->existing_route);
        } else {
            bpf_printk("|    couldn't delete sock_meta entry with 0-0 !");
        }
    }

    return 0;
}

// for kernel-initiated socket cleanup (timeout or error)
HOOK_ENTRY("sk_common_release")
int hook_sk_common_release(ctx_t *ctx) {
    struct sock *sk = (struct sock *)CTX_PARM1(ctx);
    bpf_printk("sk_common_release: %p", sk);
    if (sk == NULL) {
        return 0;
    }
    handle_sk_release(sk);
    return 0;
}

// HERE

// Simplify the shit out of this. I don't need to check every step of the TCP stack. What you want is:
// - map all packets to a pid : you don't need to check that we have added an entry, but that we were able to resolve the pid when we captured the packet
// - prevent leaking entries : you don't nee to check exactly when the entry is deleted, just that it is eventually deleted.

// The big issue here is that you've added hook points for the test to work -> sk_destruct and inet_put_port should be more than enough to clean up entries

// SO:
// 0) start by removing your last change - it can't work: if you reset on one side, you can likely expect both sides to close almost immediately
// 1) simplify the test, only check for entry creation first
// 2) check normal deletion - look for leaks
// 2-bis) check for leaks in invalid attempts or socket reuse
// 3) make sure entries are deleted when sockets are deleted

// YOU PROBABLY DON'T NEED TO CARE ABOUT RESET OR AT LEAST DON'T TRY TO SYNCHRONISE THE PRESENCE OF AN ENTRY IN THE SAME TEST.


// for externally-initiate socket cleanup (TCP RST for example)
HOOK_ENTRY("inet_csk_destroy_sock")
int hook_inet_csk_destroy_sock(ctx_t *ctx) {
    struct sock *sk = (struct sock *)CTX_PARM1(ctx);
    bpf_printk("inet_csk_destroy_sock: 0x%p", sk);
    if (sk == NULL) {
        return 0;
    }
    handle_sk_release(sk);
    return 0;
}

// for user-space initiated socket shutdown
HOOK_ENTRY("inet_shutdown")
int hook_inet_shutdown(ctx_t *ctx) {
    struct socket *socket = (struct socket *)CTX_PARM1(ctx);
    struct sock *sk = get_sock_from_socket(socket);
    bpf_printk("inet_shutdown: %p", sk);
    if (sk == NULL) {
        return 0;
    }

    handle_sk_release(sk);
    return 0;
}

// for user space initiated socket termination
HOOK_ENTRY("inet_release")
int hook_inet_release(ctx_t *ctx) {
    struct socket *socket = (struct socket *)CTX_PARM1(ctx);
    struct sock *sk = get_sock_from_socket(socket);
    bpf_printk("inet_release: %p", sk);
    if (sk == NULL) {
        return 0;
    }

    handle_sk_release(sk);
    return 0;
}

// make sure we delete entries before the relevant port is removed from the socket
HOOK_ENTRY("inet_put_port")
int hook_inet_put_port(ctx_t *ctx) {
    struct sock *sk = (struct sock *)CTX_PARM1(ctx);
    bpf_printk("inet_put_port %p", sk);
    if (sk == NULL) {
        return 0;
    }
    handle_sk_release(sk);
    return 0;
}

// In case we don't have access to SK_STORAGE maps, we need to cleanup our internal socket metadata storage on socket
// deletion.
HOOK_ENTRY("sk_destruct")
int hook_sk_destruct(ctx_t *ctx) {
    struct sock *sk = (struct sock *)CTX_PARM1(ctx);
    bpf_printk("__sk_destruct %p", sk);
    if (sk == NULL) {
        return 0;
    }
//    handle_sk_release(sk);

    // delete internal storage
    delete_sock_meta(sk);
    return 0;
}

__attribute__((always_inline)) int handle_inet_bind(struct socket *sock) {
    struct inet_bind_args_t args = {};
    args.sock = sock;
    u64 pid = bpf_get_current_pid_tgid();
    bpf_map_update_elem(&inet_bind_args, &pid, &args, BPF_ANY);
    return 0;
}

HOOK_ENTRY("inet_bind")
int hook_inet_bind(ctx_t *ctx) {
    struct socket *sock = (struct socket *)CTX_PARM1(ctx);
    return handle_inet_bind(sock);
}

HOOK_ENTRY("inet6_bind")
int hook_inet6_bind(ctx_t *ctx) {
    struct socket *sock = (struct socket *)CTX_PARM1(ctx);
    return handle_inet_bind(sock);
}

__attribute__((always_inline)) int handle_inet_bind_ret(int ret) {
    // fetch inet_bind arguments
    u64 id = bpf_get_current_pid_tgid();
    u32 tid = (u32)id;
    struct inet_bind_args_t *args = bpf_map_lookup_elem(&inet_bind_args, &id);
    if (args == NULL) {
        // should never happen, ignore
        return 0;
    }

    // delete the entry in inet_bind_args to make sure we always cleanup inet_bind_args and we don't leak entries
    bpf_map_delete_elem(&inet_bind_args, &id);

    if (ret < 0) {
        // we only care about successful bind operations
        return 0;
    }

    struct socket *socket = args->sock;
    if (socket == NULL) {
        // should never happen, ignore
        return 0;
    }

    struct sock *sk = get_sock_from_socket(socket);
    if (sk == NULL) {
        return 0;
    }
    struct pid_route_t route = {};
    struct pid_route_entry_t value = {};
    value.type = BIND_ENTRY;

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
        bpf_printk("inet_bind: %p", sk);
        print_route(&route);
        print_route_entry(&value);

        // check if the socket already has an active flow
        struct sock_meta_t *meta = get_sock_meta(sk);
        if (meta != NULL) {
            // register the new one in the sock_active_pid_route map
            meta->existing_route = route;
            print_meta(meta);
        }
        bpf_printk("|--> new BIND_ENTRY added !");
    }
    return 0;
}

HOOK_EXIT("inet_bind")
int rethook_inet_bind(ctx_t *ctx) {
    int ret = CTX_PARMRET(ctx);
    return handle_inet_bind_ret(ret);
}

HOOK_EXIT("inet6_bind")
int rethook_inet6_bind(ctx_t *ctx) {
    int ret = CTX_PARMRET(ctx);
    return handle_inet_bind_ret(ret);
}

// This hook point is used called when an internal newly created "sock" is linked to a user-space "socket" after a connection
// has been accepted. It is usually leveraged by LSM to inherit LSM attributes from a parent socket to a child.
// We use it to identify kernel created socket by the accept socket, so we decide what (not) to clean up upon socket closure.
HOOK_ENTRY("security_sock_graft")
int hook_security_sock_graft(ctx_t *ctx) {
    struct sock *sk = (struct sock *)CTX_PARM1(ctx);
    bpf_printk("security_sock_graft 0x%p", sk);

    // register that this socket is closing
    struct sock_meta_t *meta = get_sock_meta(sk);
    if (meta != NULL) {
        meta->accept_created_socket = 1;
        print_meta(meta);
    }
    return 0;
}

#endif
