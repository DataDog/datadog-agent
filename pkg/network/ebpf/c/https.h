#ifndef __HTTPS_H
#define __HTTPS_H

#include "http-buffer.h"
#include "http-types.h"
#include "http-maps.h"
#include "http-maps.h"
#include "http.h"
#include "port_range.h"
#include "sockfd.h"
#include "tags-types.h"

#define HTTPS_PORT 443

static __always_inline int read_conn_tuple(conn_tuple_t* t, struct sock* skp, u64 pid_tgid, metadata_mask_t type);
static __always_inline int http_process(http_transaction_t *http_stack, skb_info_t *skb_info, __u64 tags);

static __always_inline void https_process(conn_tuple_t *t, void *buffer, size_t len, __u64 tags) {
    http_transaction_t http;
    __builtin_memset(&http, 0, sizeof(http));
    __builtin_memcpy(&http.tup, t, sizeof(conn_tuple_t));
    read_into_buffer(http.request_fragment, buffer, len);
    http.owned_by_src_port = http.tup.sport;
    log_debug("https_process: htx=%llx sport=%d\n", &http, http.owned_by_src_port);
    http_process(&http, NULL, tags);
}

static __always_inline void https_finish(conn_tuple_t *t) {
    http_transaction_t http;
    __builtin_memset(&http, 0, sizeof(http));
    __builtin_memcpy(&http.tup, t, sizeof(conn_tuple_t));
    http.owned_by_src_port = http.tup.sport;

    skb_info_t skb_info = {0};
    skb_info.tcp_flags |= TCPHDR_FIN;
    http_process(&http, &skb_info, NO_TAGS);
}

static __always_inline conn_tuple_t* tup_from_ssl_ctx(void *ssl_ctx, u64 pid_tgid) {
    ssl_sock_t *ssl_sock = bpf_map_lookup_elem(&ssl_sock_by_ctx, &ssl_ctx);
    if (ssl_sock == NULL) {
        return NULL;
    }

    if (ssl_sock->tup.sport != 0 && ssl_sock->tup.dport != 0) {
        return &ssl_sock->tup;
    }

    // the code path below should be executed only once during the lifecycle of a SSL session
    pid_fd_t pid_fd = {
        .pid = pid_tgid >> 32,
        .fd = ssl_sock->fd,
    };

    struct sock **sock = bpf_map_lookup_elem(&sock_by_pid_fd, &pid_fd);
    if (sock == NULL)  {
        return NULL;
    }

    conn_tuple_t t;
    if (!read_conn_tuple(&t, *sock, pid_tgid, CONN_TYPE_TCP)) {
        return NULL;
    }

    // Set the `.netns` and `.pid` values to always be 0.
    // They can't be sourced from inside `read_conn_tuple_skb`,
    // which is used elsewhere to produce the same `conn_tuple_t` value from a `struct __sk_buff*` value,
    // so we ensure it is always 0 here so that both paths produce the same `conn_tuple_t` value.
    // `netns` is not used in the userspace program part that binds http information to `ConnectionStats`,
    // so this is isn't a problem.
    t.netns = 0;
    t.pid = 0;

    __builtin_memcpy(&ssl_sock->tup, &t, sizeof(conn_tuple_t));

    if (!is_ephemeral_port(ssl_sock->tup.sport)) {
        flip_tuple(&ssl_sock->tup);
    }

    return &ssl_sock->tup;
}

static __always_inline void init_ssl_sock(void *ssl_ctx, u32 socket_fd) {
    ssl_sock_t ssl_sock = { 0 };
    ssl_sock.fd = socket_fd;
    MAP_UPDATE(ssl_sock_by_ctx, &ssl_ctx, &ssl_sock, BPF_ANY);
}

static __always_inline void init_ssl_sock_from_do_handshake(struct sock *skp) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    void **ssl_ctx_map_val = bpf_map_lookup_elem(&ssl_ctx_by_pid_tgid, &pid_tgid);
    if (ssl_ctx_map_val == NULL) {
        return;
    }

    ssl_sock_t ssl_sock = {};
    if (!read_conn_tuple(&ssl_sock.tup, skp, pid_tgid, CONN_TYPE_TCP)) {
        return;
    }
    ssl_sock.tup.netns = 0;
    ssl_sock.tup.pid = 0;
    normalize_tuple(&ssl_sock.tup);

    // copy map value to stack. required for older kernels
    void *ssl_ctx = *ssl_ctx_map_val;
    MAP_UPDATE(ssl_sock_by_ctx, &ssl_ctx , &ssl_sock, BPF_ANY);
}

#endif
