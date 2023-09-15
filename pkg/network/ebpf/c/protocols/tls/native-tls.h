#ifndef __NATIVE_TLS_H
#define __NATIVE_TLS_H

#include "ktypes.h"
#include "bpf_builtins.h"

#include "protocols/tls/native-tls-maps.h"

SEC("uprobe/SSL_do_handshake")
int uprobe__SSL_do_handshake(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    void *ssl_ctx = (void *)PT_REGS_PARM1(ctx);
    log_debug("uprobe/SSL_do_handshake: pid_tgid=%llx ssl_ctx=%llx\n", pid_tgid, ssl_ctx);
    bpf_map_update_with_telemetry(ssl_ctx_by_pid_tgid, &pid_tgid, &ssl_ctx, BPF_ANY);
    return 0;
}

SEC("uretprobe/SSL_do_handshake")
int uretprobe__SSL_do_handshake(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uretprobe/SSL_do_handshake: pid_tgid=%llx\n", pid_tgid);
    bpf_map_delete_elem(&ssl_ctx_by_pid_tgid, &pid_tgid);
    return 0;
}

SEC("uprobe/SSL_connect")
int uprobe__SSL_connect(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    void *ssl_ctx = (void *)PT_REGS_PARM1(ctx);
    log_debug("uprobe/SSL_connect: pid_tgid=%llx ssl_ctx=%llx\n", pid_tgid, ssl_ctx);
    bpf_map_update_with_telemetry(ssl_ctx_by_pid_tgid, &pid_tgid, &ssl_ctx, BPF_ANY);
    return 0;
}

SEC("uretprobe/SSL_connect")
int uretprobe__SSL_connect(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uretprobe/SSL_connect: pid_tgid=%llx\n", pid_tgid);
    bpf_map_delete_elem(&ssl_ctx_by_pid_tgid, &pid_tgid);
    return 0;
}

// this uprobe is essentially creating an index mapping a SSL context to a conn_tuple_t
SEC("uprobe/SSL_set_fd")
int uprobe__SSL_set_fd(struct pt_regs *ctx) {
    void *ssl_ctx = (void *)PT_REGS_PARM1(ctx);
    u32 socket_fd = (u32)PT_REGS_PARM2(ctx);
    log_debug("uprobe/SSL_set_fd: ctx=%llx fd=%d\n", ssl_ctx, socket_fd);
    init_ssl_sock(ssl_ctx, socket_fd);
    return 0;
}

SEC("uprobe/BIO_new_socket")
int uprobe__BIO_new_socket(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 socket_fd = (u32)PT_REGS_PARM1(ctx);
    log_debug("uprobe/BIO_new_socket: pid_tgid=%llx fd=%d\n", pid_tgid, socket_fd);
    bpf_map_update_with_telemetry(bio_new_socket_args, &pid_tgid, &socket_fd, BPF_ANY);
    return 0;
}

SEC("uretprobe/BIO_new_socket")
int uretprobe__BIO_new_socket(struct pt_regs *ctx) {
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
int uprobe__SSL_set_bio(struct pt_regs *ctx) {
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
int uprobe__SSL_read(struct pt_regs *ctx) {
    ssl_read_args_t args = { 0 };
    args.ctx = (void *)PT_REGS_PARM1(ctx);
    args.buf = (void *)PT_REGS_PARM2(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("uprobe/SSL_read: pid_tgid=%llx ctx=%llx\n", pid_tgid, args.ctx);
    bpf_map_update_with_telemetry(ssl_read_args, &pid_tgid, &args, BPF_ANY);
    return 0;
}

SEC("uretprobe/SSL_read")
int uretprobe__SSL_read(struct pt_regs *ctx) {
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
        log_debug("uretprobe/SSL_write_ex: no args pid_tgid=%llx\n", pid_tgid);
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
        log_debug("uretprobe/SSL_write_ex: pid_tgid=%llx: no conn tuple\n", pid_tgid);
        goto cleanup;
    }

    https_process(conn_tuple, args->buf, bytes_count, LIBSSL);
    http_batch_flush(ctx);
cleanup:
    bpf_map_delete_elem(&ssl_write_ex_args, &pid_tgid);
    return 0;
}

SEC("uprobe/SSL_shutdown")
int uprobe__SSL_shutdown(struct pt_regs *ctx) {
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

#endif
