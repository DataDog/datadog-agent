#ifndef __SOCKFD_PROBES_H
#define __SOCKFD_PROBES_H

#include "ktypes.h"
#include "bpf_builtins.h"
#include "map-defs.h"
#include "bpf_bypass.h"

#ifndef COMPILE_CORE
#include <linux/ptrace.h>
#include <linux/net.h>
#endif

#include "sock.h"
#include "sockfd.h"
#include "pid_tgid.h"
#include "protocols/tls/go-tls-maps.h"

SEC("kprobe/tcp_close")
int BPF_KPROBE(kprobe__tcp_close, struct sock *sk) {
    if (sk == NULL) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    conn_tuple_t t;
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    pid_fd_t *pid_fd = bpf_map_lookup_elem(&pid_fd_by_tuple, &t);
    if (pid_fd != NULL) {
        // Copy map value to stack so we can use it as a map key (needed for older kernels)
        pid_fd_t pid_fd_copy = *pid_fd;
        bpf_map_delete_elem(&tuple_by_pid_fd, &pid_fd_copy);
        bpf_map_delete_elem(&pid_fd_by_tuple, &t);
    }
    
    void **ssl_ctx_ptr = bpf_map_lookup_elem(&ssl_ctx_by_tuple, &t);
    if (ssl_ctx_ptr) {
        void *ssl_ctx = *ssl_ctx_ptr;
        bpf_map_delete_elem(&ssl_ctx_by_tuple, &t);
        if (ssl_ctx) {
            bpf_map_delete_elem(&ssl_sock_by_ctx, &ssl_ctx);
        }
    }
    
    
    // Cleanup Go TLS connections map
    // Look up the Go TLS connection pointer using the reverse mapping
    void **go_tls_conn_ptr = bpf_map_lookup_elem(&go_tls_conn_by_tuple, &t);
    if (go_tls_conn_ptr) {
        void *go_tls_conn = *go_tls_conn_ptr;
        // Remove both the forward and reverse mappings
        bpf_map_delete_elem(&conn_tup_by_go_tls_conn, &go_tls_conn);
        bpf_map_delete_elem(&go_tls_conn_by_tuple, &t);
    }
    // The cleanup of the map happens either during TCP termination or during the TLS shutdown event.
    // TCP termination is managed by the socket filter, thus it cannot clean TLS entries,
    // as it does not have access to the PID and NETNS.
    // Therefore, we use tls_finish to clean the connection. While this approach is not ideal, it is the best option available to us for now.
    tls_finish(ctx, &t, true);
    return 0;
}

SEC("kprobe/sockfd_lookup_light")
int BPF_KPROBE(kprobe__sockfd_lookup_light, int sockfd) {
    u64 pid_tgid = bpf_get_current_pid_tgid();

    // Check if have already a map entry for this pid_fd_t
    // TODO: This lookup eliminates *4* map operations for existing entries
    // but can reduce the accuracy of programs relying on socket FDs for
    // processes with a lot of FD churn
    pid_fd_t key = {
        .pid = GET_USER_MODE_PID(pid_tgid),
        .fd = sockfd,
    };
    conn_tuple_t *t = bpf_map_lookup_elem(&tuple_by_pid_fd, &key);
    if (t != NULL) {
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
int BPF_KRETPROBE(kretprobe__sockfd_lookup_light, struct socket *socket) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    int *sockfd = bpf_map_lookup_elem(&sockfd_lookup_args, &pid_tgid);
    if (sockfd == NULL) {
        return 0;
    }

    // NOTE: the code below should be executed only once for a given socket
    // For now let's only store information for TCP sockets
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

    conn_tuple_t t;
    if (!read_conn_tuple(&t, sock, pid_tgid, CONN_TYPE_TCP)) {
        goto cleanup;
    }

    pid_fd_t pid_fd = {
        .pid = GET_USER_MODE_PID(pid_tgid),
        .fd = (*sockfd),
    };

    // These entries are cleaned up by tcp_close
    bpf_map_update_with_telemetry(pid_fd_by_tuple, &t, &pid_fd, BPF_ANY);
    bpf_map_update_with_telemetry(tuple_by_pid_fd, &pid_fd, &t, BPF_ANY);
cleanup:
    bpf_map_delete_elem(&sockfd_lookup_args, &pid_tgid);
    return 0;
}

#endif // __SOCKFD_PROBES_H
