#ifndef __HTTPS_H
#define __HTTPS_H

#ifdef COMPILE_CORE
#include "ktypes.h"
#define MINORBITS	20
#define MINORMASK	((1U << MINORBITS) - 1)
#define MAJOR(dev)	((unsigned int) ((dev) >> MINORBITS))
#define MINOR(dev)	((unsigned int) ((dev) & MINORMASK))
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
#include "protocols/http/types.h"
#include "protocols/http/maps.h"
#include "protocols/http/http.h"
#include "protocols/tls/go-tls-maps.h"
#include "protocols/tls/go-tls-types.h"
#include "protocols/tls/native-tls-maps.h"
#include "protocols/tls/tags-types.h"
#include "protocols/tls/tls-maps.h"

#define HTTPS_PORT 443

static __always_inline void http_process(http_event_t *event, skb_info_t *skb_info, __u64 tags);

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
    conn_tuple_t final_tuple = {0};
    conn_tuple_t normalized_tuple = *t;
    normalize_tuple(&normalized_tuple);
    normalized_tuple.pid = 0;
    normalized_tuple.netns = 0;

    protocol_stack_t *stack = get_protocol_stack(&normalized_tuple);
    if (!stack) {
        return;
    }

    const __u32 zero = 0;
    protocol_t protocol = get_protocol_from_stack(stack, LAYER_APPLICATION);
    if (protocol == PROTOCOL_UNKNOWN) {
        char *request_fragment = bpf_map_lookup_elem(&tls_classification_heap, &zero);
        if (request_fragment == NULL) {
            return;
        }
        read_into_user_buffer_classification(request_fragment, buffer_ptr);

        classify_decrypted_payload(stack, &normalized_tuple, request_fragment, len);
        protocol = get_protocol_from_stack(stack, LAYER_APPLICATION);
    }
    tls_prog_t prog;
    switch (protocol) {
    case PROTOCOL_HTTP:
        prog = TLS_HTTP_PROCESS;
        final_tuple = normalized_tuple;
        break;
    case PROTOCOL_HTTP2:
        prog = TLS_HTTP2_FIRST_FRAME;
        final_tuple = *t;
        break;
    default:
        return;
    }

    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        log_debug("dispatcher failed to save arguments for tls tail call");
        return;
    }
    *args = (tls_dispatcher_arguments_t){
        .tup = final_tuple,
        .tags = tags,
        .buffer_ptr = buffer_ptr,
        .data_end = len,
        .data_off = 0,
    };
    bpf_tail_call_compat(ctx, &tls_process_progs, prog);
}

static __always_inline void tls_finish(struct pt_regs *ctx, conn_tuple_t *t, bool skip_http) {
    conn_tuple_t final_tuple = {0};
    conn_tuple_t normalized_tuple = *t;
    normalize_tuple(&normalized_tuple);
    normalized_tuple.pid = 0;
    normalized_tuple.netns = 0;

    protocol_stack_t *stack = get_protocol_stack(&normalized_tuple);
    if (!stack) {
        return;
    }

    tls_prog_t prog;
    protocol_t protocol = get_protocol_from_stack(stack, LAYER_APPLICATION);
    switch (protocol) {
    case PROTOCOL_HTTP:
        // HTTP is a special case. As of today, regardless of TLS or plaintext traffic, we ignore the PID and NETNS while processing it.
        // The termination, both for TLS and plaintext, for HTTP traffic is taken care of in the socket filter.
        // Until we split the TLS and plaintext management for HTTP traffic, there are flows (such as those being called from tcp_close)
        // in which we don't want to terminate HTTP traffic, but instead leave it to the socket filter.
        if (skip_http) {return;}
        prog = TLS_HTTP_TERMINATION;
        final_tuple = normalized_tuple;
        break;
    case PROTOCOL_HTTP2:
        prog = TLS_HTTP2_TERMINATION;
        final_tuple = *t;
        break;
    default:
        return;
    }

    const __u32 zero = 0;
    tls_dispatcher_arguments_t *args = bpf_map_lookup_elem(&tls_dispatcher_arguments, &zero);
    if (args == NULL) {
        log_debug("dispatcher failed to save arguments for tls tail call");
        return;
    }
    bpf_memset(args, 0, sizeof(tls_dispatcher_arguments_t));
    bpf_memcpy(&args->tup, &final_tuple, sizeof(conn_tuple_t));
    bpf_tail_call_compat(ctx, &tls_process_progs, prog);
}

static __always_inline conn_tuple_t* tup_from_ssl_ctx(void *ssl_ctx, u64 pid_tgid) {
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
    if (sock == NULL)  {
        return NULL;
    }

    conn_tuple_t t;
    if (!read_conn_tuple(&t, *sock, pid_tgid, CONN_TYPE_TCP)) {
        return NULL;
    }

    bpf_memcpy(&ssl_sock->tup, &t, sizeof(conn_tuple_t));
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

    // copy map value to stack. required for older kernels
    void *ssl_ctx = *ssl_ctx_map_val;
    bpf_map_update_with_telemetry(ssl_sock_by_ctx, &ssl_ctx, &ssl_sock, BPF_ANY);
}

/**
 * get_offsets_data retrieves the result of binary analysis for the
 * current task binary's inode number.
 */
static __always_inline tls_offsets_data_t* get_offsets_data() {
    struct task_struct *t = (struct task_struct *) bpf_get_current_task();
    struct inode *inode;
    go_tls_offsets_data_key_t key;
    dev_t dev_id;

    inode = BPF_CORE_READ(t, mm, exe_file, f_inode);
    if (!inode) {
        log_debug("get_offsets_data: could not read f_inode field");
        return NULL;
    }

    int err;
    err = BPF_CORE_READ_INTO(&key.ino, inode, i_ino);
    if (err) {
        log_debug("get_offsets_data: could not read i_ino field");
        return NULL;
    }

    err = BPF_CORE_READ_INTO(&dev_id, inode, i_sb, s_dev);
    if (err) {
        log_debug("get_offsets_data: could not read s_dev field");
        return NULL;
    }

    key.device_id_major = MAJOR(dev_id);
    key.device_id_minor = MINOR(dev_id);

    log_debug("get_offsets_data: task binary inode number: %ld; device ID %x:%x", key.ino, key.device_id_major, key.device_id_minor);

    return bpf_map_lookup_elem(&offsets_data, &key);
}

#endif
