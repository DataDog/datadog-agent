#ifndef __HTTPS_H
#define __HTTPS_H

#include "http-types.h"
#include "http-maps.h"
#include "http-maps.h"
#include "http.h"

// read_into_buffer copies data from an arbitrary memory address into a (statically sized) HTTP buffer.
// Ideally we would only copy min(data_size, HTTP_BUFFER_SIZE) bytes, but the code below is the only way
// we found to handle data sizes smaller than HTTP_BUFFER_SIZE in Kernel 4.4.
// In a nutshell, we read HTTP_BUFFER_SIZE bytes no matter what and then get rid of garbage data.
// Please note that even though the memset could be removed with no semantic change to the code,
// it is still necessary to make the eBPF verifier happy.
static __always_inline void read_into_buffer(char *buffer, char *data, size_t data_size) {
    __builtin_memset(buffer, 0, HTTP_BUFFER_SIZE);
    bpf_probe_read(buffer, HTTP_BUFFER_SIZE, data);
    if (data_size >= HTTP_BUFFER_SIZE) {
        return;
    }

    // clean up garbage
#pragma unroll
    for (int i = 0; i < HTTP_BUFFER_SIZE; i++) {
        if (i >= data_size) {
            buffer[i] = 0;
        }
    }
}

static __always_inline void https_process(conn_tuple_t *t, void *buffer, size_t len, u64 tags) {
    http_transaction_t http;
    __builtin_memset(&http, 0, sizeof(http));
    __builtin_memcpy(&http.tup, t, sizeof(conn_tuple_t));
    read_into_buffer((char *)http.request_fragment, buffer, len);
    http.owned_by_src_port = http.tup.sport;
    http.tags |= tags;
    http_process(&http, NULL);
}

static __always_inline void https_finish(conn_tuple_t *t) {
    http_transaction_t http;
    __builtin_memset(&http, 0, sizeof(http));
    __builtin_memcpy(&http.tup, t, sizeof(conn_tuple_t));
    http.owned_by_src_port = http.tup.sport;

    skb_info_t skb_info = {0};
    skb_info.tcp_flags |= TCPHDR_FIN;
    http_process(&http, &skb_info);
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
    bpf_map_update_elem(&ssl_sock_by_ctx, &ssl_ctx, &ssl_sock, BPF_ANY);
}

#endif
