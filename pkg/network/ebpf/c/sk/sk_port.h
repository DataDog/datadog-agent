#ifndef __SK_PORT_H
#define __SK_PORT_H

#include "ktypes.h"
#include "bpf_helpers.h"

#include "defs.h"
#include "tracer/port.h"
#include "netns.h"
#include "ip.h"
#include "ipv6.h"

SEC("iter/task_file")
int bpf_iter__task_file_port_bindings(struct bpf_iter__task_file *ctx) {
    struct task_struct *task = ctx->task;
    struct file *file = ctx->file;
    if (!task || !file) {
        return 0;
    }
    struct socket *sock = bpf_sock_from_file(file);
    if (!sock) {
        return 0;
    }
    struct sock *sk = sock->sk;

    if (sk->sk_protocol == IPPROTO_TCP || sk->sk_protocol == IPPROTO_MPTCP) {
        switch (sk->sk_family) {
        case AF_INET6:
            if (!is_tcpv6_enabled()) return 0;
            break;
        case AF_INET:
            if (!is_tcpv4_enabled()) return 0;
            break;
        default:
            return 0;
        }

        if (sk->__sk_common.skc_state == TCP_LISTEN) {
            port_binding_t pb = {};
            pb.netns = get_netns_from_sock(sk);
            pb.port = read_sport(sk);
            add_port_bind(&pb, port_bindings);
        }
        return 0;
    } else if (sk->sk_protocol == IPPROTO_UDP) {
        switch (sk->sk_family) {
        case AF_INET6:
            if (!is_udpv6_enabled()) return 0;
            break;
        case AF_INET:
            if (!is_udpv4_enabled()) return 0;
            break;
        default:
            return 0;
        }

        // TODO skip if in ephemeral port range
        if (sk->__sk_common.skc_state == TCP_CLOSE) {
            port_binding_t pb = {};
            pb.netns = get_netns_from_sock(sk);
            pb.port = read_sport(sk);
            add_port_bind(&pb, udp_port_bindings);
        }
        return 0;
    }
    return 0;
}

#endif
