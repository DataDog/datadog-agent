#ifndef __SK_PORT_H
#define __SK_PORT_H

#include "ktypes.h"
#include "bpf_helpers.h"

#include "defs.h"
#include "tracer/port.h"
#include "netns.h"
#include "ip.h"
#include "ipv6.h"
#include "port_range.h"

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
    if (!is_protocol_family_enabled(sk)) {
        return 0;
    }

    if (sk->sk_protocol == IPPROTO_TCP || sk->sk_protocol == IPPROTO_MPTCP) {
        if (sk->__sk_common.skc_state == TCP_LISTEN) {
            port_binding_t pb = {};
            pb.netns = get_netns_from_sock(sk);
            pb.port = read_sport(sk);
            add_port_bind(&pb, port_bindings);
        }
    } else if (sk->sk_protocol == IPPROTO_UDP) {
        if (sk->__sk_common.skc_state == TCP_CLOSE) {
            u16 sport = read_sport(sk);
            if (is_ephemeral_port(sport)) {
                return 0;
            }

            port_binding_t pb = {};
            pb.netns = get_netns_from_sock(sk);
            pb.port = sport;
            add_port_bind(&pb, udp_port_bindings);
        }
    }
    return 0;
}

#endif
