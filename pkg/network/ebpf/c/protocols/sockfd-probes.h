#ifndef __SOCKFD_PROBES_H
#define __SOCKFD_PROBES_H

#include "ktypes.h"
#include "bpf_builtins.h"
#include "map-defs.h"

#ifndef COMPILE_CORE
#include <linux/ptrace.h>
#include <linux/net.h>
#endif

#include "sock.h"
#include "sockfd.h"
#include "http2/decoding-tls.h"

// handle_http2_termination is a helper function that is called when a TCP connection is closed.
static __always_inline void handle_http2_termination (struct pt_regs *ctx, struct sock *sk){
    conn_tuple_t t;
    u64 pid_tgid = bpf_get_current_pid_tgid();
    if (read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return;
    }

    conn_tuple_t normalized_tuple = t;
    normalize_tuple(&normalized_tuple);
    normalized_tuple.pid = 0;
    normalized_tuple.netns = 0;

    protocol_stack_t *stack = get_protocol_stack(&normalized_tuple);
    if (!stack) {
        return;
    }

    protocol_t protocol = get_protocol_from_stack(stack, LAYER_APPLICATION);
    if (protocol != PROTOCOL_HTTP2) {
        return;
    }

    const __u32 zero = 0;
    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        log_debug("dispatcher failed to save arguments for tls tail call");
        return;
    }
    bpf_memset(args, 0, sizeof(tls_dispatcher_arguments_t));
    bpf_memcpy(&args->tup, &t, sizeof(conn_tuple_t));
    tls_termination_maps_deletion(args);
    return;
}

SEC("kprobe/tcp_close")
int kprobe__tcp_close(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    if (sk == NULL) {
        return 0;
    }

    pid_fd_t* pid_fd = bpf_map_lookup_elem(&pid_fd_by_sock, &sk);
    if (pid_fd == NULL) {
        return 0;
    }

    // Copy map value to stack before re-using it (needed for older kernels)
    pid_fd_t pid_fd_copy = {};
    bpf_memcpy(&pid_fd_copy, pid_fd, sizeof(pid_fd_t));
    pid_fd = &pid_fd_copy;

    bpf_map_delete_elem(&sock_by_pid_fd, pid_fd);
    bpf_map_delete_elem(&pid_fd_by_sock, &sk);

    // The probe contains PID and netns information, which we utilize to attempt to clean up resources for HTTP/2 TLS.
    handle_http2_termination(ctx, sk);
    return 0;
}

SEC("kprobe/sockfd_lookup_light")
int kprobe__sockfd_lookup_light(struct pt_regs *ctx) {
    int sockfd = (int)PT_REGS_PARM1(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();

    // Check if have already a map entry for this pid_fd_t
    // TODO: This lookup eliminates *4* map operations for existing entries
    // but can reduce the accuracy of programs relying on socket FDs for
    // processes with a lot of FD churn
    pid_fd_t key = {
        .pid = pid_tgid >> 32,
        .fd = sockfd,
    };
    struct sock **sock = bpf_map_lookup_elem(&sock_by_pid_fd, &key);
    if (sock != NULL) {
        return 0;
    }

    bpf_map_update_with_telemetry(sockfd_lookup_args, &pid_tgid, &sockfd, BPF_ANY);
    return 0;
}

static __always_inline const struct proto_ops * socket_proto_ops(struct socket *sock) {
    const struct proto_ops *proto_ops = NULL;
#ifdef COMPILE_PREBUILT
    // (struct socket).ops is always directly after (struct socket).sk,
    // which is a pointer.
    u64 ops_offset = offset_socket_sk() + sizeof(void *);
    bpf_probe_read_kernel_with_telemetry(&proto_ops, sizeof(proto_ops), (char*)sock + ops_offset);
#elif defined(COMPILE_RUNTIME) || defined(COMPILE_CORE)
    BPF_CORE_READ_INTO(&proto_ops, sock, ops);
#endif

    return proto_ops;
}

// this kretprobe is essentially creating:
// * an index of pid_fd_t to a struct sock*;
// * an index of struct sock* to pid_fd_t;
SEC("kretprobe/sockfd_lookup_light")
int kretprobe__sockfd_lookup_light(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    int *sockfd = bpf_map_lookup_elem(&sockfd_lookup_args, &pid_tgid);
    if (sockfd == NULL) {
        return 0;
    }

    // For now let's only store information for TCP sockets
    struct socket *socket = (struct socket *)PT_REGS_RC(ctx);
    if (!socket)
        goto cleanup;

    enum sock_type sock_type = 0;
    bpf_probe_read_kernel_with_telemetry(&sock_type, sizeof(short), &socket->type);

    const struct proto_ops *proto_ops = socket_proto_ops(socket);
    if (!proto_ops) {
        goto cleanup;
    }

    int family = 0;
    bpf_probe_read_kernel_with_telemetry(&family, sizeof(family), &proto_ops->family);
    if (sock_type != SOCK_STREAM || !(family == AF_INET || family == AF_INET6)) {
        goto cleanup;
    }

    // Retrieve struct sock* pointer from struct socket*
    struct sock *sock = socket_sk(socket);
    if (!sock) {
        goto cleanup;
    }

    pid_fd_t pid_fd = {
        .pid = pid_tgid >> 32,
        .fd = (*sockfd),
    };

    // These entries are cleaned up by tcp_close
    bpf_map_update_with_telemetry(pid_fd_by_sock, &sock, &pid_fd, BPF_ANY);
    bpf_map_update_with_telemetry(sock_by_pid_fd, &pid_fd, &sock, BPF_ANY);
cleanup:
    bpf_map_delete_elem(&sockfd_lookup_args, &pid_tgid);
    return 0;
}

#endif // __SOCKFD_PROBES_H
