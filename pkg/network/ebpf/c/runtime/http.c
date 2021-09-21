#include "tracer.h"
#include "bpf_helpers.h"
#include "ip.h"
#include "ipv6.h"
#include "http.h"
#include "sockfd.h"
#include "conn-tuple.h"

// TODO: Replace those by injected constants based on system configuration
// once we have port range detection merged into the codebase.
#define EPHEMERAL_RANGE_BEG 32768
#define EPHEMERAL_RANGE_END 60999
#define HTTPS_PORT 443
#define SO_SUFFIX_SIZE 3

static __always_inline int is_ephemeral_port(u16 port) {
    return port >= EPHEMERAL_RANGE_BEG && port <= EPHEMERAL_RANGE_END;
}

static __always_inline void read_skb_data(struct __sk_buff* skb, u32 offset, char *buffer) {
    if (skb->len - offset < HTTP_BUFFER_SIZE) {
        return;
    }

#pragma unroll
    for (int i = 0; i < HTTP_BUFFER_SIZE; i++) {
        buffer[i] = load_byte(skb, offset + i);
    }
}

SEC("socket/http_filter")
int socket__http_filter(struct __sk_buff* skb) {
    skb_info_t skb_info;

    if (!read_conn_tuple_skb(skb, &skb_info)) {
        return 0;
    }

    // don't bother to inspect packet contents when there is no chance we're dealing with plain HTTP
    if (!(skb_info.tup.metadata&CONN_TYPE_TCP) || skb_info.tup.sport == HTTPS_PORT || skb_info.tup.dport == HTTPS_PORT) {
        return 0;
    }


    // src_port represents the source port number *before* normalization
    // for more context please refer to http-types.h comment on `owned_by_src_port` field
    u16 src_port = skb_info.tup.sport;

    // we normalize the tuple to always be (client, server),
    // so if sport is not in ephemeral port range we flip it
    if (!is_ephemeral_port(skb_info.tup.sport)) {
        flip_tuple(&skb_info.tup);
    }

    char buffer[HTTP_BUFFER_SIZE];
    __builtin_memset(buffer, 0, sizeof(buffer));
    read_skb_data(skb, skb_info.data_off, buffer);
    http_process(buffer, &skb_info, src_port);
    return 0;
}

// This kprobe is used to send batch completion notification to userspace
// because perf events can't be sent from socket filter programs
SEC("kretprobe/tcp_sendmsg")
int kretprobe__tcp_sendmsg(struct pt_regs* ctx) {
    http_notify_batch(ctx);
    return 0;
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

// this uprobe is essentially creating an index mapping a SSL context to a conn_tuple_t
SEC("uprobe/SSL_set_fd")
int uprobe__SSL_set_fd(struct pt_regs* ctx) {
    void *ssl_ctx = (void *)PT_REGS_PARM1(ctx);
    u32 socket_fd = (u32)PT_REGS_PARM2(ctx);
    init_ssl_sock(ssl_ctx, socket_fd);
    return 0;
}

SEC("uprobe/BIO_new_socket")
int uprobe__BIO_new_socket(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 socket_fd = (u32)PT_REGS_PARM1(ctx);
    bpf_map_update_elem(&bio_new_socket_args, &pid_tgid, &socket_fd, BPF_ANY);
    return 0;
}

SEC("uretprobe/BIO_new_socket")
int uretprobe__BIO_new_socket(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 *socket_fd = bpf_map_lookup_elem(&bio_new_socket_args, &pid_tgid);
    if (socket_fd == NULL) {
        return 0;
    }

    void *bio = (void *)PT_REGS_RC(ctx);
    if (bio == NULL) {
        goto cleanup;
    }
    u32 fd = *socket_fd; // copy map value into stack (required by older Kernels)
    bpf_map_update_elem(&fd_by_ssl_bio, &bio, &fd, BPF_ANY);
 cleanup:
    bpf_map_delete_elem(&bio_new_socket_args, &pid_tgid);
    return 0;
}

SEC("uprobe/SSL_set_bio")
int uprobe__SSL_set_bio(struct pt_regs* ctx) {
    void *ssl_ctx = (void *)PT_REGS_PARM1(ctx);
    void *bio = (void *)PT_REGS_PARM2(ctx);
    u32 *socket_fd = bpf_map_lookup_elem(&fd_by_ssl_bio, &bio);
    if (socket_fd == NULL)  {
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
    bpf_map_update_elem(&ssl_read_args, &pid_tgid, &args, BPF_ANY);
    return 0;
}

SEC("uretprobe/SSL_read")
int uretprobe__SSL_read(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    ssl_read_args_t *args = bpf_map_lookup_elem(&ssl_read_args, &pid_tgid);
    if (args == NULL) {
        return 0;
    }

    void *ssl_ctx = args->ctx;
    conn_tuple_t *t = tup_from_ssl_ctx(ssl_ctx, pid_tgid);
    if (t == NULL) {
        goto cleanup;
    }

    u32 len = (u32)PT_REGS_RC(ctx);
    char buffer[HTTP_BUFFER_SIZE];
    __builtin_memset(buffer, 0, sizeof(buffer));
    if (len >= HTTP_BUFFER_SIZE) {
        bpf_probe_read(buffer, sizeof(buffer), args->buf);
    }

    skb_info_t skb_info = {0};
    __builtin_memcpy(&skb_info.tup, t, sizeof(conn_tuple_t));
    http_process(buffer, &skb_info, skb_info.tup.sport);
 cleanup:
    bpf_map_delete_elem(&ssl_read_args, &pid_tgid);
    return 0;
}

SEC("uprobe/SSL_write")
int uprobe__SSL_write(struct pt_regs* ctx) {
    void *ssl_ctx = (void *)PT_REGS_PARM1(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    conn_tuple_t *t = tup_from_ssl_ctx(ssl_ctx, pid_tgid);
    if (t == NULL) {
        return 0;
    }

    void *ssl_buffer = (void *)PT_REGS_PARM2(ctx);
    size_t len = (size_t)PT_REGS_PARM3(ctx);
    char buffer[HTTP_BUFFER_SIZE];
    __builtin_memset(buffer, 0, sizeof(buffer));
    if (len >= HTTP_BUFFER_SIZE) {
        bpf_probe_read(buffer, sizeof(buffer), ssl_buffer);
    }

    skb_info_t skb_info = {0};
    __builtin_memcpy(&skb_info.tup, t, sizeof(conn_tuple_t));
    http_process(buffer, &skb_info, skb_info.tup.sport);
    return 0;
}

SEC("uprobe/SSL_shutdown")
int uprobe__SSL_shutdown(struct pt_regs* ctx) {
    void *ssl_ctx = (void *)PT_REGS_PARM1(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    conn_tuple_t *t = tup_from_ssl_ctx(ssl_ctx, pid_tgid);
    if (t == NULL) {
        return 0;
    }

    char buffer[HTTP_BUFFER_SIZE];
    __builtin_memset(buffer, 0, sizeof(buffer));

    skb_info_t skb_info = {0};
    __builtin_memcpy(&skb_info.tup, t, sizeof(conn_tuple_t));

    // TODO: this is just a hack. Let's get rid of this skb_info argument altogether
    skb_info.tcp_flags |= TCPHDR_FIN;
    http_process(buffer, &skb_info, skb_info.tup.sport);
    bpf_map_delete_elem(&ssl_sock_by_ctx, &ssl_ctx);
    return 0;
}

SEC("kprobe/do_sys_open")
int kprobe__do_sys_open(struct pt_regs* ctx) {
    char *path_argument = (char *)PT_REGS_PARM2(ctx);
    lib_path_t path = {0};
    bpf_probe_read(path.buf, sizeof(path.buf), path_argument);

    // Find the null character and clean up the garbage following it
#pragma unroll
    for (int i = 0; i < LIB_PATH_MAX_SIZE; i++) {
        if (path.len) {
            path.buf[i] = 0;
        } else if (path.buf[i] == 0) {
            path.len = i;
        }
    }

    // Bail out if the path size is larger than our buffer
    if (!path.len) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    bpf_map_update_elem(&open_at_args, &pid_tgid, &path, BPF_ANY);
    return 0;
}

SEC("kretprobe/do_sys_open")
int kretprobe__do_sys_open(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();

    // If file couldn't be opened, bail out
    if (!(long)PT_REGS_RC(ctx)) {
        goto cleanup;
    }

    lib_path_t *path = bpf_map_lookup_elem(&open_at_args, &pid_tgid);
    if (path == NULL) {
        return 0;
    }

    // Detect whether the file being opened is a shared library
    int is_shared_library = 0;
#pragma unroll
    for (int i = 0; i < LIB_PATH_MAX_SIZE - SO_SUFFIX_SIZE; i++) {
        if (path->buf[i] == '.' && path->buf[i+1] == 's' && path->buf[i+2] == 'o') {
            is_shared_library = 1;
            break;
        }
    }

    if (!is_shared_library) {
        goto cleanup;
    }

    // Copy map value into eBPF stack
    lib_path_t lib_path;
    __builtin_memcpy(&lib_path, path, sizeof(lib_path));

    u32 cpu = bpf_get_smp_processor_id();
    bpf_perf_event_output(ctx, &shared_libraries, cpu, &lib_path, sizeof(lib_path));
 cleanup:
    bpf_map_delete_elem(&open_at_args, &pid_tgid);
    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
