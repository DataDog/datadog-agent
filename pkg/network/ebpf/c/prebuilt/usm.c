#include "kconfig.h"
#include "bpf_tracing.h"
#include "bpf_telemetry.h"
#include "bpf_builtins.h"

#include "offsets.h"

#include "protocols/classification/dispatcher-helpers.h"
#include "protocols/http/http.h"
#include "protocols/http/buffer.h"
#include "protocols/http2/decoding.h"
#include "protocols/tls/https.h"
#include "protocols/tls/tags-types.h"
#include "protocols/tls/java-tls-erpc.h"
#include "protocols/kafka/kafka-parsing.h"

#define SO_SUFFIX_SIZE 3

SEC("socket/protocol_dispatcher")
int socket__protocol_dispatcher(struct __sk_buff *skb) {
    protocol_dispatcher_entrypoint(skb);
    return 0;
}

// This entry point is needed to bypass a memory limit on socket filters
// See: https://datadoghq.atlassian.net/wiki/spaces/NET/pages/2326855913/HTTP#Known-issues
SEC("socket/protocol_dispatcher_kafka")
int socket__protocol_dispatcher_kafka(struct __sk_buff *skb) {
    dispatch_kafka(skb);
    return 0;
}

SEC("kprobe/tcp_sendmsg")
int kprobe__tcp_sendmsg(struct pt_regs* ctx) {
    log_debug("kprobe/tcp_sendmsg: sk=%llx\n", PT_REGS_PARM1(ctx));
    // map connection tuple during SSL_do_handshake(ctx)
    map_ssl_ctx_to_sock((struct sock*)PT_REGS_PARM1(ctx));
    return 0;
}

SEC("tracepoint/net/netif_receive_skb")
int tracepoint__net__netif_receive_skb(struct pt_regs* ctx) {
    log_debug("tracepoint/net/netif_receive_skb\n");
    // flush batch to userspace
    // because perf events can't be sent from socket filter programs
    http_batch_flush(ctx);
    http2_batch_flush(ctx);
    kafka_batch_flush(ctx);
    return 0;
}


SEC("uprobe/SSL_do_handshake")
int uprobe__SSL_do_handshake(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    void *ssl_ctx = (void *)PT_REGS_PARM1(ctx);
    log_debug("uprobe/SSL_do_handshake: pid_tgid=%llx ssl_ctx=%llx\n", pid_tgid, ssl_ctx);
    bpf_map_update_with_telemetry(ssl_ctx_by_pid_tgid, &pid_tgid, &ssl_ctx, BPF_ANY);
    return 0;
}

SEC("uretprobe/SSL_do_handshake")
int uretprobe__SSL_do_handshake(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uretprobe/SSL_do_handshake: pid_tgid=%llx\n", pid_tgid);
    bpf_map_delete_elem(&ssl_ctx_by_pid_tgid, &pid_tgid);
    return 0;
}

SEC("uprobe/SSL_connect")
int uprobe__SSL_connect(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    void *ssl_ctx = (void *)PT_REGS_PARM1(ctx);
    log_debug("uprobe/SSL_connect: pid_tgid=%llx ssl_ctx=%llx\n", pid_tgid, ssl_ctx);
    bpf_map_update_with_telemetry(ssl_ctx_by_pid_tgid, &pid_tgid, &ssl_ctx, BPF_ANY);
    return 0;
}

SEC("uretprobe/SSL_connect")
int uretprobe__SSL_connect(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uretprobe/SSL_connect: pid_tgid=%llx\n", pid_tgid);
    bpf_map_delete_elem(&ssl_ctx_by_pid_tgid, &pid_tgid);
    return 0;
}

// this uprobe is essentially creating an index mapping a SSL context to a conn_tuple_t
SEC("uprobe/SSL_set_fd")
int uprobe__SSL_set_fd(struct pt_regs* ctx) {
    void *ssl_ctx = (void *)PT_REGS_PARM1(ctx);
    u32 socket_fd = (u32)PT_REGS_PARM2(ctx);
    log_debug("uprobe/SSL_set_fd: ctx=%llx fd=%d\n", ssl_ctx, socket_fd);
    init_ssl_sock(ssl_ctx, socket_fd);
    return 0;
}

SEC("uprobe/BIO_new_socket")
int uprobe__BIO_new_socket(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 socket_fd = (u32)PT_REGS_PARM1(ctx);
    log_debug("uprobe/BIO_new_socket: pid_tgid=%llx fd=%d\n", pid_tgid, socket_fd);
    bpf_map_update_with_telemetry(bio_new_socket_args, &pid_tgid, &socket_fd, BPF_ANY);
    return 0;
}

SEC("uretprobe/BIO_new_socket")
int uretprobe__BIO_new_socket(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uretprobe/BIO_new_socket: pid_tgid=%llx\n", pid_tgid);
    u32 *socket_fd = bpf_map_lookup_elem(&bio_new_socket_args, &pid_tgid);
    if (socket_fd == NULL) {
        return 0;
    }

    void *bio = (void *)PT_REGS_RC(ctx);
    if (bio == NULL) {
        goto cleanup;
    }
    u32 fd = *socket_fd; // copy map value into stack (required by older Kernels)
    bpf_map_update_with_telemetry(fd_by_ssl_bio, &bio, &fd, BPF_ANY);
cleanup:
    bpf_map_delete_elem(&bio_new_socket_args, &pid_tgid);
    return 0;
}

SEC("uprobe/SSL_set_bio")
int uprobe__SSL_set_bio(struct pt_regs* ctx) {
    void *ssl_ctx = (void *)PT_REGS_PARM1(ctx);
    void *bio = (void *)PT_REGS_PARM2(ctx);
    log_debug("uprobe/SSL_set_bio: ctx=%llx bio=%llx\n", ssl_ctx, bio);
    u32 *socket_fd = bpf_map_lookup_elem(&fd_by_ssl_bio, &bio);
    if (socket_fd == NULL) {
        return 0;
    }
    init_ssl_sock(ssl_ctx, *socket_fd);
    bpf_map_delete_elem(&fd_by_ssl_bio, &bio);
    return 0;
}

SEC("uprobe/SSL_read")
int uprobe__SSL_read(struct pt_regs* ctx) {
    ssl_read_args_t args = {0};
    args.ctx = (void *)PT_REGS_PARM1(ctx);
    args.buf = (void *)PT_REGS_PARM2(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uprobe/SSL_read: pid_tgid=%llx ctx=%llx\n", pid_tgid, args.ctx);
    bpf_map_update_with_telemetry(ssl_read_args, &pid_tgid, &args, BPF_ANY);
    return 0;
}

SEC("uretprobe/SSL_read")
int uretprobe__SSL_read(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    int len = (int)PT_REGS_RC(ctx);
    if (len <= 0) {
        log_debug("uretprobe/SSL_read: pid_tgid=%llx ret=%d\n", pid_tgid, len);
        goto cleanup;
    }

    log_debug("uretprobe/SSL_read: pid_tgid=%llx\n", pid_tgid);
    ssl_read_args_t *args = bpf_map_lookup_elem(&ssl_read_args, &pid_tgid);
    if (args == NULL) {
        return 0;
    }

    void *ssl_ctx = args->ctx;
    conn_tuple_t *t = tup_from_ssl_ctx(ssl_ctx, pid_tgid);
    if (t == NULL) {
        log_debug("uretprobe/SSL_read: pid_tgid=%llx ctx=%llx: no conn tuple\n", pid_tgid, ssl_ctx);
        goto cleanup;
    }

    https_process(t, args->buf, len, LIBSSL);
    http_batch_flush(ctx);
cleanup:
    bpf_map_delete_elem(&ssl_read_args, &pid_tgid);
    return 0;
}

SEC("uprobe/SSL_write")
int uprobe__SSL_write(struct pt_regs* ctx) {
    ssl_write_args_t args = {0};
    args.ctx = (void *)PT_REGS_PARM1(ctx);
    args.buf = (void *)PT_REGS_PARM2(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uprobe/SSL_write: pid_tgid=%llx ctx=%llx\n", pid_tgid, args.ctx);
    bpf_map_update_with_telemetry(ssl_write_args, &pid_tgid, &args, BPF_ANY);
    return 0;
}

SEC("uretprobe/SSL_write")
int uretprobe__SSL_write(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    int write_len = (int)PT_REGS_RC(ctx);
    log_debug("uretprobe/SSL_write: pid_tgid=%llx len=%d\n", pid_tgid, write_len);
    if (write_len <= 0) {
        goto cleanup;
    }

    ssl_write_args_t *args = bpf_map_lookup_elem(&ssl_write_args, &pid_tgid);
    if (args == NULL) {
        return 0;
    }

    conn_tuple_t *t = tup_from_ssl_ctx(args->ctx, pid_tgid);
    if (t == NULL) {
        goto cleanup;
    }

    https_process(t, args->buf, write_len, LIBSSL);
    http_batch_flush(ctx);
cleanup:
    bpf_map_delete_elem(&ssl_write_args, &pid_tgid);
    return 0;
}

SEC("uprobe/SSL_read_ex")
int uprobe__SSL_read_ex(struct pt_regs* ctx) {
    ssl_read_ex_args_t args = {0};
    args.ctx = (void *)PT_REGS_PARM1(ctx);
    args.buf = (void *)PT_REGS_PARM2(ctx);
    args.size_out_param = (size_t *)PT_REGS_PARM4(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uprobe/SSL_read_ex: pid_tgid=%llx ctx=%llx\n", pid_tgid, args.ctx);
    bpf_map_update_elem(&ssl_read_ex_args, &pid_tgid, &args, BPF_ANY);
    return 0;
}

SEC("uretprobe/SSL_read_ex")
int uretprobe__SSL_read_ex(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    const int return_code = (int)PT_REGS_RC(ctx);
    if (return_code != 1) {
        log_debug("uretprobe/SSL_read_ex: failed pid_tgid=%llx ret=%d\n", pid_tgid, return_code);
        goto cleanup;
    }

    ssl_read_ex_args_t *args = bpf_map_lookup_elem(&ssl_read_ex_args, &pid_tgid);
    if (args == NULL) {
        log_debug("uretprobe/SSL_read_ex: no args pid_tgid=%llx\n", pid_tgid);
        return 0;
    }

    if (args->size_out_param == NULL) {
        log_debug("uretprobe/SSL_read_ex: pid_tgid=%llx buffer size out param is null\n", pid_tgid);
        goto cleanup;
    }

    size_t bytes_count = 0;
    bpf_probe_read_user(&bytes_count, sizeof(bytes_count), args->size_out_param);
    if ( bytes_count <= 0) {
        log_debug("uretprobe/SSL_read_ex: read non positive number of bytes (pid_tgid=%llx len=%d)\n", pid_tgid, bytes_count);
        goto cleanup;
    }

    void *ssl_ctx = args->ctx;
    conn_tuple_t *conn_tuple = tup_from_ssl_ctx(ssl_ctx, pid_tgid);
    if (conn_tuple == NULL) {
        log_debug("uretprobe/SSL_read_ex: pid_tgid=%llx ctx=%llx: no conn tuple\n", pid_tgid, ssl_ctx);
        goto cleanup;
    }

    https_process(conn_tuple, args->buf, bytes_count, LIBSSL);
    http_batch_flush(ctx);
cleanup:
    bpf_map_delete_elem(&ssl_read_ex_args, &pid_tgid);
    return 0;
}

SEC("uprobe/SSL_write_ex")
int uprobe__SSL_write_ex(struct pt_regs* ctx) {
    ssl_write_ex_args_t args = {0};
    args.ctx = (void *)PT_REGS_PARM1(ctx);
    args.buf = (void *)PT_REGS_PARM2(ctx);
    args.size_out_param = (size_t *)PT_REGS_PARM4(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uprobe/SSL_write_ex: pid_tgid=%llx ctx=%llx\n", pid_tgid, args.ctx);
    bpf_map_update_elem(&ssl_write_ex_args, &pid_tgid, &args, BPF_ANY);
    return 0;
}

SEC("uretprobe/SSL_write_ex")
int uretprobe__SSL_write_ex(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    const int return_code = (int)PT_REGS_RC(ctx);
    if (return_code != 1) {
        log_debug("uretprobe/SSL_write_ex: failed pid_tgid=%llx len=%d\n", pid_tgid, return_code);
        goto cleanup;
    }

    ssl_write_ex_args_t *args = bpf_map_lookup_elem(&ssl_write_ex_args, &pid_tgid);
    if (args == NULL) {
        return 0;
    }

    if (args->size_out_param == NULL) {
        log_debug("uretprobe/SSL_write_ex: pid_tgid=%llx buffer size out param is null\n", pid_tgid);
        goto cleanup;
    }

    size_t bytes_count = 0;
    bpf_probe_read_user(&bytes_count, sizeof(bytes_count), args->size_out_param);
    if ( bytes_count <= 0) {
        log_debug("uretprobe/SSL_write_ex: wrote non positive number of bytes (pid_tgid=%llx len=%d)\n", pid_tgid, bytes_count);
        goto cleanup;
    }

    conn_tuple_t *conn_tuple = tup_from_ssl_ctx(args->ctx, pid_tgid);
    if (conn_tuple == NULL) {
        goto cleanup;
    }

    https_process(conn_tuple, args->buf, bytes_count, LIBSSL);
    http_batch_flush(ctx);
cleanup:
    bpf_map_delete_elem(&ssl_write_ex_args, &pid_tgid);
    return 0;
}

SEC("uprobe/SSL_shutdown")
int uprobe__SSL_shutdown(struct pt_regs* ctx) {
    void *ssl_ctx = (void *)PT_REGS_PARM1(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uprobe/SSL_shutdown: pid_tgid=%llx ctx=%llx\n", pid_tgid, ssl_ctx);
    conn_tuple_t *t = tup_from_ssl_ctx(ssl_ctx, pid_tgid);
    if (t == NULL) {
        return 0;
    }

    https_finish(t);
    http_batch_flush(ctx);

    bpf_map_delete_elem(&ssl_sock_by_ctx, &ssl_ctx);
    return 0;
}

SEC("uprobe/gnutls_handshake")
int uprobe__gnutls_handshake(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    void *ssl_ctx = (void *)PT_REGS_PARM1(ctx);
    bpf_map_update_with_telemetry(ssl_ctx_by_pid_tgid, &pid_tgid, &ssl_ctx, BPF_ANY);
    return 0;
}

SEC("uretprobe/gnutls_handshake")
int uretprobe__gnutls_handshake(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    bpf_map_delete_elem(&ssl_ctx_by_pid_tgid, &pid_tgid);
    return 0;
}

// void gnutls_transport_set_int (gnutls_session_t session, int fd)
// Note: this function is implemented as a macro in gnutls
// that calls gnutls_transport_set_int2, so no uprobe is needed

// void gnutls_transport_set_int2 (gnutls_session_t session, int recv_fd, int send_fd)
SEC("uprobe/gnutls_transport_set_int2")
int uprobe__gnutls_transport_set_int2(struct pt_regs *ctx) {
    void *ssl_session = (void *)PT_REGS_PARM1(ctx);
    // Use the recv_fd and ignore the send_fd;
    // in most real-world scenarios, they are the same.
    int recv_fd = (int)PT_REGS_PARM2(ctx);
    log_debug("gnutls_transport_set_int2: ctx=%llx fd=%d\n", ssl_session, recv_fd);

    init_ssl_sock(ssl_session, (u32)recv_fd);
    return 0;
}

// void gnutls_transport_set_ptr (gnutls_session_t session, gnutls_transport_ptr_t ptr)
// "In berkeley style sockets this function will set the connection descriptor."
SEC("uprobe/gnutls_transport_set_ptr")
int uprobe__gnutls_transport_set_ptr(struct pt_regs *ctx) {
    void *ssl_session = (void *)PT_REGS_PARM1(ctx);
    // This is a void*, but it might contain the socket fd cast as a pointer.
    int fd = (int)PT_REGS_PARM2(ctx);
    log_debug("gnutls_transport_set_ptr: ctx=%llx fd=%d\n", ssl_session, fd);

    init_ssl_sock(ssl_session, (u32)fd);
    return 0;
}

// void gnutls_transport_set_ptr2 (gnutls_session_t session, gnutls_transport_ptr_t recv_ptr, gnutls_transport_ptr_t send_ptr)
// "In berkeley style sockets this function will set the connection descriptor."
SEC("uprobe/gnutls_transport_set_ptr2")
int uprobe__gnutls_transport_set_ptr2(struct pt_regs *ctx) {
    void *ssl_session = (void *)PT_REGS_PARM1(ctx);
    // Use the recv_ptr and ignore the send_ptr;
    // in most real-world scenarios, they are the same.
    // This is a void*, but it might contain the socket fd cast as a pointer.
    int recv_fd = (int)PT_REGS_PARM2(ctx);
    log_debug("gnutls_transport_set_ptr2: ctx=%llx fd=%d\n", ssl_session, recv_fd);

    init_ssl_sock(ssl_session, (u32)recv_fd);
    return 0;
}

// ssize_t gnutls_record_recv (gnutls_session_t session, void * data, size_t data_size)
SEC("uprobe/gnutls_record_recv")
int uprobe__gnutls_record_recv(struct pt_regs *ctx) {
    void *ssl_session = (void *)PT_REGS_PARM1(ctx);
    void *data = (void *)PT_REGS_PARM2(ctx);

    // Re-use the map for SSL_read
    ssl_read_args_t args = {
        .ctx = ssl_session,
        .buf = data,
    };
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("gnutls_record_recv: pid=%llu ctx=%llx\n", pid_tgid, ssl_session);
    bpf_map_update_with_telemetry(ssl_read_args, &pid_tgid, &args, BPF_ANY);
    return 0;
}

// ssize_t gnutls_record_recv (gnutls_session_t session, void * data, size_t data_size)
SEC("uretprobe/gnutls_record_recv")
int uretprobe__gnutls_record_recv(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    ssize_t read_len = (ssize_t)PT_REGS_RC(ctx);
    if (read_len <= 0) {
        goto cleanup;
    }

    // Re-use the map for SSL_read
    ssl_read_args_t *args = bpf_map_lookup_elem(&ssl_read_args, &pid_tgid);
    if (args == NULL) {
        return 0;
    }

    void *ssl_session = args->ctx;
    log_debug("uret/gnutls_record_recv: pid=%llu ctx=%llx\n", pid_tgid, ssl_session);
    conn_tuple_t *t = tup_from_ssl_ctx(ssl_session, pid_tgid);
    if (t == NULL) {
        goto cleanup;
    }

    https_process(t, args->buf, read_len, LIBGNUTLS);
    http_batch_flush(ctx);
cleanup:
    bpf_map_delete_elem(&ssl_read_args, &pid_tgid);
    return 0;
}

// ssize_t gnutls_record_send (gnutls_session_t session, const void * data, size_t data_size)
SEC("uprobe/gnutls_record_send")
int uprobe__gnutls_record_send(struct pt_regs *ctx) {
    ssl_write_args_t args = {0};
    args.ctx = (void *)PT_REGS_PARM1(ctx);
    args.buf = (void *)PT_REGS_PARM2(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uprobe/gnutls_record_send: pid=%llu ctx=%llx\n", pid_tgid, args.ctx);
    bpf_map_update_with_telemetry(ssl_write_args, &pid_tgid, &args, BPF_ANY);
    return 0;
}

SEC("uretprobe/gnutls_record_send")
int uretprobe__gnutls_record_send(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    ssize_t write_len = (ssize_t)PT_REGS_RC(ctx);
    log_debug("uretprobe/gnutls_record_send: pid=%llu len=%d\n", pid_tgid, write_len);
    if (write_len <= 0) {
        goto cleanup;
    }

    ssl_write_args_t *args = bpf_map_lookup_elem(&ssl_write_args, &pid_tgid);
    if (args == NULL) {
        return 0;
    }

    conn_tuple_t *t = tup_from_ssl_ctx(args->ctx, pid_tgid);
    if (t == NULL) {
        goto cleanup;
    }

    https_process(t, args->buf, write_len, LIBGNUTLS);
    http_batch_flush(ctx);
cleanup:
    bpf_map_delete_elem(&ssl_write_args, &pid_tgid);
    return 0;
}

static __always_inline void gnutls_goodbye(void *ssl_session) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("gnutls_goodbye: pid=%llu ctx=%llx\n", pid_tgid, ssl_session);
    conn_tuple_t *t = tup_from_ssl_ctx(ssl_session, pid_tgid);
    if (t == NULL) {
        return;
    }

    https_finish(t);
    bpf_map_delete_elem(&ssl_sock_by_ctx, &ssl_session);
}

// int gnutls_bye (gnutls_session_t session, gnutls_close_request_t how)
SEC("uprobe/gnutls_bye")
int uprobe__gnutls_bye(struct pt_regs *ctx) {
    void *ssl_session = (void *)PT_REGS_PARM1(ctx);
    gnutls_goodbye(ssl_session);
    http_batch_flush(ctx);
    return 0;
}

// void gnutls_deinit (gnutls_session_t session)
SEC("uprobe/gnutls_deinit")
int uprobe__gnutls_deinit(struct pt_regs *ctx) {
    void *ssl_session = (void *)PT_REGS_PARM1(ctx);
    gnutls_goodbye(ssl_session);
    http_batch_flush(ctx);
    return 0;
}

static __always_inline int fill_path_safe(lib_path_t *path, char *path_argument) {
#pragma unroll
    for (int i = 0; i < LIB_PATH_MAX_SIZE; i++) {
        bpf_probe_read_user(&path->buf[i], 1, &path_argument[i]);
        if (path->buf[i] == 0) {
            path->len = i;
            break;
        }
    }
    return 0;
}

static __always_inline int do_sys_open_helper_enter(struct pt_regs* ctx) {
    char *path_argument = (char *)PT_REGS_PARM2(ctx);
    lib_path_t path = {0};
    if (bpf_probe_read_user_with_telemetry(path.buf, sizeof(path.buf), path_argument) >= 0) {
// Find the null character and clean up the garbage following it
#pragma unroll
        for (int i = 0; i < LIB_PATH_MAX_SIZE; i++) {
            if (path.len) {
                path.buf[i] = 0;
            } else if (path.buf[i] == 0) {
                path.len = i;
            }
        }
    } else {
        fill_path_safe(&path, path_argument);
    }

    // Bail out if the path size is larger than our buffer
    if (!path.len) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    path.pid = pid_tgid >> 32;
    bpf_map_update_with_telemetry(open_at_args, &pid_tgid, &path, BPF_ANY);
    return 0;
}

SEC("kprobe/do_sys_open")
int kprobe__do_sys_open(struct pt_regs* ctx) {
    return do_sys_open_helper_enter(ctx);
}

SEC("kprobe/do_sys_openat2")
int kprobe__do_sys_openat2(struct pt_regs* ctx) {
    return do_sys_open_helper_enter(ctx);
}

static __always_inline int do_sys_open_helper_exit(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();

    // If file couldn't be opened, bail out
    if ((long)PT_REGS_RC(ctx) < 0) {
        goto cleanup;
    }

    lib_path_t *path = bpf_map_lookup_elem(&open_at_args, &pid_tgid);
    if (path == NULL) {
        return 0;
    }

    // Detect whether the file being opened is a shared library
    bool is_shared_library = false;
#pragma unroll
    for (int i = 0; i < LIB_PATH_MAX_SIZE - SO_SUFFIX_SIZE; i++) {
        if (path->buf[i] == '.' && path->buf[i+1] == 's' && path->buf[i+2] == 'o') {
            is_shared_library = true;
            break;
        }
    }

    if (!is_shared_library) {
        goto cleanup;
    }

    // Copy map value into eBPF stack
    lib_path_t lib_path;
    bpf_memcpy(&lib_path, path, sizeof(lib_path));

    u32 cpu = bpf_get_smp_processor_id();
    bpf_perf_event_output_with_telemetry(ctx, &shared_libraries, cpu, &lib_path, sizeof(lib_path));
cleanup:
    bpf_map_delete_elem(&open_at_args, &pid_tgid);
    return 0;
}

SEC("kretprobe/do_sys_open")
int kretprobe__do_sys_open(struct pt_regs* ctx) {
    return do_sys_open_helper_exit(ctx);
}

SEC("kretprobe/do_sys_openat2")
int kretprobe__do_sys_openat2(struct pt_regs* ctx) {
    return do_sys_open_helper_exit(ctx);
}

SEC("kprobe/do_vfs_ioctl")
int kprobe__do_vfs_ioctl(struct pt_regs *ctx) {
    if (is_usm_erpc_request(ctx)) {
        handle_erpc_request(ctx);
    }

    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
