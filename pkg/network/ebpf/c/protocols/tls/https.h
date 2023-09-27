#ifndef __HTTPS_H
#define __HTTPS_H

#ifdef COMPILE_CORE
#include "ktypes.h"
#define MINORBITS  20
#define MINORMASK  ((1U << MINORBITS) - 1)
#define MAJOR(dev) ((unsigned int)((dev) >> MINORBITS))
#define MINOR(dev) ((unsigned int)((dev)&MINORMASK))
#else
#include <linux/dcache.h>
#include <linux/fs.h>
#include <linux/mm_types.h>
#include <linux/sched.h>
#endif

#include "bpf_builtins.h"
#include "port_range.h"
#include "sock.h"

#include "protocols/classification/dispatcher-helpers.h"
#include "protocols/classification/dispatcher-maps.h"
#include "protocols/http/buffer.h"
#include "protocols/http/http.h"
#include "protocols/http2/decoding.h"
#include "protocols/tls/go-tls-maps.h"
#include "protocols/tls/go-tls-types.h"
#include "protocols/tls/native-tls-maps.h"
#include "protocols/tls/tags-types.h"
#include "protocols/tls/tls-maps.h"

#define HTTPS_PORT 443

static __always_inline void http_process(http_transaction_t *http_stack, skb_info_t *skb_info, __u64 tags);

/* this function is called by all TLS hookpoints (OpenSSL, GnuTLS and GoTLS, JavaTLS) and */
/* it's used for classify the subset of protocols that is supported by `classify_protocol_for_dispatcher` */
static __always_inline void classify_decrypted_payload(protocol_stack_t *stack, conn_tuple_t *t, void *buffer, size_t len) {
    // we're in the context of TLS hookpoints, thus the protocol is TLS.
    set_protocol(stack, PROTOCOL_TLS);

    if (is_protocol_layer_known(stack, LAYER_APPLICATION)) {
        // No classification is needed.
        return;
    }

    protocol_t proto = PROTOCOL_UNKNOWN;
    classify_protocol_for_dispatcher(&proto, t, buffer, len);
    if (proto == PROTOCOL_UNKNOWN) {
        return;
    }

    set_protocol(stack, proto);
}

static __always_inline void tls_process(struct pt_regs *ctx, conn_tuple_t *t, void *buffer_ptr, size_t len, __u64 tags) {
    protocol_stack_t *stack = get_protocol_stack(t);
    if (!stack) {
        return;
    }
    log_debug("tls process entry");
    log_debug("[tls_process] conn tuple: saddr_h=%lu saddr_l=%lu",
        t->saddr_h,
        t->saddr_l);
    log_debug("[tls_process] conn tuple: daddr_h=%lu daddr_l=%lu",
        t->daddr_h,
        t->daddr_l);
    log_debug("[tls_process] conn tuple: sport=%u dport=%u",
        t->sport,
        t->dport);

    const __u32 zero = 0;
    protocol_t protocol = get_protocol_from_stack(stack, LAYER_APPLICATION);
    if (protocol == PROTOCOL_UNKNOWN) {
        char *request_fragment = bpf_map_lookup_elem(&tls_classification_heap, &zero);
        if (request_fragment == NULL) {
            return;
        }
        read_into_user_buffer_classification(request_fragment, buffer_ptr);

        classify_decrypted_payload(stack, t, request_fragment, len);
        protocol = get_protocol_from_stack(stack, LAYER_APPLICATION);
    }
    tls_prog_t prog;
    switch (protocol) {
    case PROTOCOL_HTTP:
        prog = TLS_HTTP_PROCESS;
        break;
    case PROTOCOL_HTTP2:
        prog = TLS_HTTP2_PROCESS;
        break;
    default:
        return;
    }

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        log_debug("dispatcher failed to save arguments for tls tail call\n");
        return;
    }

    *args = (tls_dispatcher_arguments_t){
        .tup = *t,
        .buf = buffer_ptr,
        .tags = tags,
        .len = len,
        .off = 0,
    };

    bpf_tail_call_compat(ctx, &tls_process_progs, prog);
}

static __always_inline void tls_finish(struct pt_regs *ctx, conn_tuple_t *t) {
    protocol_stack_t *stack = get_protocol_stack(t);
    if (!stack) {
        return;
    }

    tls_prog_t prog;
    protocol_t protocol = get_protocol_from_stack(stack, LAYER_APPLICATION);
    switch (protocol) {
    case PROTOCOL_HTTP:
        prog = TLS_HTTP_TERMINATION;
        break;
    /* case PROTOCOL_HTTP2: */
    /*     prog = TLS_HTTP2_PROCESS; */
    /*     break; */
    default:
        return;
    }

    const __u32 zero = 0;
    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        log_debug("dispatcher failed to save arguments for tls tail call\n");
        return;
    }
    bpf_memset(args, 0, sizeof(tls_dispatcher_arguments_t));
    bpf_memcpy(&args->tup, t, sizeof(conn_tuple_t));
    bpf_tail_call_compat(ctx, &tls_process_progs, prog);
}

static __always_inline conn_tuple_t *tup_from_ssl_ctx(void *ssl_ctx, u64 pid_tgid) {
    ssl_sock_t *ssl_sock = bpf_map_lookup_elem(&ssl_sock_by_ctx, &ssl_ctx);
    if (ssl_sock == NULL) {
        // Best-effort fallback mechanism to guess the socket address without
        // intercepting the SSL socket initialization. This improves the the quality
        // of data for TLS connections started *prior* to system-probe
        // initialization. Here we simply store the pid_tgid along with its
        // corresponding ssl_ctx pointer. In another probe (tcp_sendmsg), we
        // query again this map and if there is a match we assume that the *sock
        // object is the the TCP socket being used by this SSL connection. The
        // whole thing works based on the assumption that SSL_read/SSL_write is
        // then followed by the execution of tcp_sendmsg within the same CPU
        // context. This is not necessarily true for all cases (such as when
        // using the async SSL API) but seems to work on most-cases.
        bpf_map_update_with_telemetry(ssl_ctx_by_pid_tgid, &pid_tgid, &ssl_ctx, BPF_ANY);
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
    if (sock == NULL) {
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

    bpf_memcpy(&ssl_sock->tup, &t, sizeof(conn_tuple_t));

    if (!is_ephemeral_port(ssl_sock->tup.sport)) {
        flip_tuple(&ssl_sock->tup);
    }

    return &ssl_sock->tup;
}

static __always_inline void init_ssl_sock(void *ssl_ctx, u32 socket_fd) {
    ssl_sock_t ssl_sock = { 0 };
    ssl_sock.fd = socket_fd;
    bpf_map_update_with_telemetry(ssl_sock_by_ctx, &ssl_ctx, &ssl_sock, BPF_ANY);
}

static __always_inline void map_ssl_ctx_to_sock(struct sock *skp) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    void **ssl_ctx_map_val = bpf_map_lookup_elem(&ssl_ctx_by_pid_tgid, &pid_tgid);
    if (ssl_ctx_map_val == NULL) {
        return;
    }
    bpf_map_delete_elem(&ssl_ctx_by_pid_tgid, &pid_tgid);

    ssl_sock_t ssl_sock = {};
    if (!read_conn_tuple(&ssl_sock.tup, skp, pid_tgid, CONN_TYPE_TCP)) {
        return;
    }
    ssl_sock.tup.netns = 0;
    ssl_sock.tup.pid = 0;
    normalize_tuple(&ssl_sock.tup);

    // copy map value to stack. required for older kernels
    void *ssl_ctx = *ssl_ctx_map_val;
    bpf_map_update_with_telemetry(ssl_sock_by_ctx, &ssl_ctx, &ssl_sock, BPF_ANY);
}

/**
 * get_offsets_data retrieves the result of binary analysis for the
 * current task binary's inode number.
 */
static __always_inline tls_offsets_data_t *get_offsets_data() {
    struct task_struct *t = (struct task_struct *)bpf_get_current_task();
    struct inode *inode;
    go_tls_offsets_data_key_t key;
    dev_t dev_id;

    inode = BPF_CORE_READ(t, mm, exe_file, f_inode);
    if (!inode) {
        log_debug("get_offsets_data: could not read f_inode field\n");
        return NULL;
    }

    int err;
    err = BPF_CORE_READ_INTO(&key.ino, inode, i_ino);
    if (err) {
        log_debug("get_offsets_data: could not read i_ino field\n");
        return NULL;
    }

    err = BPF_CORE_READ_INTO(&dev_id, inode, i_sb, s_dev);
    if (err) {
        log_debug("get_offsets_data: could not read s_dev field\n");
        return NULL;
    }

    key.device_id_major = MAJOR(dev_id);
    key.device_id_minor = MINOR(dev_id);

    log_debug("get_offsets_data: task binary inode number: %ld; device ID %x:%x\n", key.ino, key.device_id_major, key.device_id_minor);

    return bpf_map_lookup_elem(&offsets_data, &key);
}

#endif
