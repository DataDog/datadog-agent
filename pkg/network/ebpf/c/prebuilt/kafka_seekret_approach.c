//#include <bpf/bpf_core_read.h>
//#include <bpf/bpf_helpers.h>
//#include <bpf/bpf_tracing.h>
//#include <bpf/bpf_endian.h>

#include "kconfig.h"
#include "tracer.h"
#include "bpf_telemetry.h"
#include "ip.h"
#include "ipv6.h"
#include "http.h"
#include "https.h"
#include "http-buffer.h"
#include "sockfd.h"
#include "tags-types.h"
#include "sock.h"
#include "port_range.h"

#include "kafka/seekret-approach/defs.h"
#include "kafka/seekret-approach/helpers.h"
#include "kafka/seekret-approach/maps.h"
//#include "structs.h"

// Need to use tcp_sendmsg to create the conn_tuple struct so the tracepoint can use it later

//SEC("kprobe/tcp_sendmsg")
//int kprobe__tcp_sendmsg(struct pt_regs *ctx) {
//    u64 pid_tgid = bpf_get_current_pid_tgid();
//    log_debug("kprobe/tcp_sendmsg: pid_tgid: %d\n", pid_tgid);
//    struct sock *parm1 = (struct sock *)PT_REGS_PARM1(ctx);
//    struct sock *skp = parm1;
//    bpf_map_update_with_telemetry(tcp_sendmsg_args_for_kafka, &pid_tgid, &skp, BPF_ANY);
//    return 0;
//}
//
//EC("kretprobe/tcp_sendmsg")
//int kretprobe__tcp_sendmsg(struct pt_regs *ctx) {
//    u64 pid_tgid = bpf_get_current_pid_tgid();
//    struct sock **skpp = (struct sock **)bpf_map_lookup_elem(&tcp_sendmsg_args_for_kafka, &pid_tgid);
//    if (!skpp) {
//        log_debug("kretprobe/tcp_sendmsg: sock not found\n");
//        return 0;
//    }
//
//    struct sock *skp = *skpp;
//    bpf_map_delete_elem(&tcp_sendmsg_args_for_kafka, &pid_tgid);
//
//    int sent = PT_REGS_RC(ctx);
//    if (sent < 0) {
//        log_debug("kretprobe/tcp_sendmsg: tcp_sendmsg err=%d\n", sent);
//        return 0;
//    }
//
//    if (!skp) {
//        return 0;
//    }
//
//    log_debug("kretprobe/tcp_sendmsg: pid_tgid: %d, sent: %d, sock: %x\n", pid_tgid, sent, skp);
//    conn_tuple_t t = {};
//    if (!read_conn_tuple(&t, skp, pid_tgid, CONN_TYPE_TCP)) {
//        return 0;
//    }
//
//
//    handle_tcp_stats(&t, skp, 0);
//
//    // I assume here that t is a valid conn_tuple
//    bpf_map_update_elem(&current_conn_tuple, &pid_tgid, &t, BPF_ANY);
//    return 0;
//
////    __u32 packets_in = 0;
////    __u32 packets_out = 0;
////    get_tcp_segment_counts(skp, &packets_in, &packets_out);
////
////    return handle_message(&t, sent, 0, CONN_DIRECTION_UNKNOWN, packets_out, packets_in, PACKET_COUNT_ABSOLUTE, skp);
//}

SEC("tracepoint/sys_enter_connect")
int tracepoint__sys_enter_connect(struct trace_event_raw_sys_enter *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();
    struct connect_args_t connect_args = {};

    connect_args.fd = (int)ctx->args[0];
    connect_args.addr = (const struct sockaddr*)ctx->args[1];
    bpf_map_update_elem(&active_connect_args_map, &id, &connect_args, BPF_ANY);

    return 0;
}

SEC("tracepoint/sys_exit_connect")
int tracepoint__sys_exit_connect(struct trace_event_raw_sys_exit *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();

    const struct connect_args_t* connect_args = bpf_map_lookup_elem(&active_connect_args_map, &id);
    if (connect_args != NULL) {
        process_syscall_connect(ctx, id, ctx->ret, connect_args);
        bpf_map_delete_elem(&active_connect_args_map, &id);
    }

    return 0;
}

SEC("tracepoint/sys_enter_accept")
int tracepoint__sys_enter_accept(struct trace_event_raw_sys_enter *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();

    struct accept_args_t accept_args = {};
    accept_args.addr = (struct sockaddr*)ctx->args[1];
    bpf_map_update_elem(&active_accept_args_map, &id, &accept_args, BPF_ANY);

    return 0;
}

SEC("tracepoint/sys_exit_accept")
int tracepoint__sys_exit_accept(struct trace_event_raw_sys_exit *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();

    struct accept_args_t* accept_args = bpf_map_lookup_elem(&active_accept_args_map, &id);
    if (accept_args != NULL) {
        process_syscall_accept(ctx, id, ctx->ret, accept_args);
        bpf_map_delete_elem(&active_accept_args_map, &id);
    }

    return 0;
}

SEC("tracepoint/sys_enter_accept4")
int tracepoint__sys_enter_accept4(struct trace_event_raw_sys_enter *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();

    struct accept_args_t accept_args = {};
    accept_args.addr = (struct sockaddr*)ctx->args[1];
    bpf_map_update_elem(&active_accept_args_map, &id, &accept_args, BPF_ANY);

    return 0;
}

SEC("tracepoint/sys_exit_accept4")
int tracepoint__sys_exit_accept4(struct trace_event_raw_sys_exit *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();

    struct accept_args_t* accept_args = bpf_map_lookup_elem(&active_accept_args_map, &id);
    if (accept_args != NULL) {
       process_syscall_accept(ctx, id, ctx->ret, accept_args);
       bpf_map_delete_elem(&active_accept_args_map, &id);
    }

    return 0;
}

SEC("tracepoint/sys_enter_write")
int tracepoint__sys_enter_write(struct trace_event_raw_sys_enter *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();

    struct data_args_t write_args = {};
    write_args.fd = (int)ctx->args[0];
    write_args.buf = (char*)ctx->args[1];
    bpf_map_update_elem(&active_write_args_map, &id, &write_args, BPF_ANY);

    return 0;
}

SEC("tracepoint/sys_exit_write")
int tracepoint__sys_exit_write(struct trace_event_raw_sys_exit *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();

    struct data_args_t* write_args = bpf_map_lookup_elem(&active_write_args_map, &id);
    if (write_args != NULL) {
        ssize_t bytes_count = ctx->ret;
        process_plaintext_data(ctx, id, kEgress, write_args, bytes_count);
        bpf_map_delete_elem(&active_write_args_map, &id);
    }

    return 0;
}

SEC("tracepoint/sys_enter_writev")
int tracepoint__sys_enter_writev(struct trace_event_raw_sys_enter *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();

    struct data_args_t write_args = {};
    write_args.fd = (int)ctx->args[0];
    write_args.iov = (const struct iovec*)ctx->args[1];
    write_args.iovlen = (int)ctx->args[2];
    bpf_map_update_elem(&active_write_args_map, &id, &write_args, BPF_ANY);

    return 0;
}

SEC("tracepoint/sys_exit_writev")
int tracepoint__sys_exit_writev(struct trace_event_raw_sys_exit *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();

    struct data_args_t* write_args = bpf_map_lookup_elem(&active_write_args_map, &id);
    if (write_args != NULL) {
        ssize_t bytes_count = ctx->ret;
        process_syscall_data_vecs(ctx, id, kEgress, write_args, bytes_count);
        bpf_map_delete_elem(&active_write_args_map, &id);
    }

    return 0;
}

SEC("tracepoint/sys_enter_sendto")
int tracepoint__sys_enter_sendto(struct trace_event_raw_sys_enter *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();
    int sockfd = (int)ctx->args[0];
    char* buf = (char*)ctx->args[1];
    //size_t len =  (size_t)ctx->args[2];
    const struct sockaddr* dest_addr = (const struct sockaddr*)ctx->args[4];
    if (dest_addr != NULL) {
        struct connect_args_t connect_args = {};
        connect_args.fd = sockfd;
        connect_args.addr = dest_addr;
        bpf_map_update_elem(&active_connect_args_map, &id, &connect_args, BPF_ANY);
    }

    struct data_args_t write_args = {};
    write_args.fd = sockfd;
    write_args.buf = buf;
    bpf_map_update_elem(&active_write_args_map, &id, &write_args, BPF_ANY);

    return 0;
}

SEC("tracepoint/sys_exit_sendto")
int tracepoint__sys_exit_sendto(struct trace_event_raw_sys_exit *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();
    ssize_t bytes_count = ctx->ret;

    const struct connect_args_t* connect_args = bpf_map_lookup_elem(&active_connect_args_map, &id);
    if (connect_args != NULL && bytes_count > 0) {
        process_implicit_conn(ctx, id, connect_args);
        bpf_map_delete_elem(&active_connect_args_map, &id);
    }

    struct data_args_t* write_args = bpf_map_lookup_elem(&active_write_args_map, &id);
    if (write_args != NULL) {
        process_plaintext_data(ctx, id, kEgress, write_args, bytes_count);
        bpf_map_delete_elem(&active_write_args_map, &id);
    }

    return 0;
}

SEC("tracepoint/sys_enter_sendmsg")
int tracepoint__sys_enter_sendmsg(struct trace_event_raw_sys_enter *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();
    int sockfd = (int)ctx->args[0];
    const struct user_msghdr* msghdr = (const struct user_msghdr*)ctx->args[1];
    if (msghdr != NULL) {

        //void *msg_name = BPF_CORE_READ_USER(msghdr, msg_name);
        void *msg_name;
        bpf_probe_read_user(&return_value, sizeof(return_value), (void*)&(sock->__sk_common.skc_v6_daddr));
        return return_value;

        if (msg_name != NULL) {
            struct connect_args_t connect_args = {};
            connect_args.fd = sockfd;
            connect_args.addr = msg_name;
            bpf_map_update_elem(&active_connect_args_map, &id, &connect_args, BPF_ANY);
        }

        struct data_args_t write_args = {};
        write_args.fd = sockfd;
        write_args.iov = BPF_CORE_READ_USER(msghdr, msg_iov);
        write_args.iovlen = BPF_CORE_READ_USER(msghdr, msg_iovlen);
        bpf_map_update_elem(&active_write_args_map, &id, &write_args, BPF_ANY);
    }

    return 0;
}

SEC("tracepoint/sys_exit_sendmsg")
int tracepoint__sys_exit_sendmsg(struct trace_event_raw_sys_exit *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();
    ssize_t bytes_count = ctx->ret;

    const struct connect_args_t* connect_args = bpf_map_lookup_elem(&active_connect_args_map, &id);
    if (connect_args != NULL && bytes_count > 0) {
        process_implicit_conn(ctx, id, connect_args);
        bpf_map_delete_elem(&active_connect_args_map, &id);
    }

    const struct data_args_t* write_args = bpf_map_lookup_elem(&active_write_args_map, &id);
    if (write_args != NULL) {
        process_syscall_data_vecs(ctx, id, kEgress, write_args, bytes_count);
        bpf_map_delete_elem(&active_write_args_map, &id);
    }

    return 0;
}

SEC("tracepoint/sys_enter_sendmmsg")
int tracepoint__sys_enter_sendmmsg(struct trace_event_raw_sys_enter *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();
    int sockfd = (int)ctx->args[0];
    struct mmsghdr* msgvec = (struct mmsghdr*)ctx->args[1];
    unsigned int vlen = (unsigned int)ctx->args[2];

    if (msgvec != NULL && vlen >= 1) {
        void *msg_name = BPF_CORE_READ_USER(msgvec, msg_hdr.msg_name);
        if (msg_name != NULL) {
            struct connect_args_t connect_args = {};
            connect_args.fd = sockfd;
            connect_args.addr = msg_name;
            bpf_map_update_elem(&active_connect_args_map, &id, &connect_args, BPF_ANY);
        }

        struct data_args_t write_args = {};
        write_args.fd = sockfd;
        write_args.iov = BPF_CORE_READ_USER(msgvec, msg_hdr.msg_iov);
        write_args.iovlen = BPF_CORE_READ_USER(msgvec, msg_hdr.msg_iovlen);
        write_args.msg_len = BPF_CORE_READ_USER(msgvec, msg_len);
        bpf_map_update_elem(&active_write_args_map, &id, &write_args, BPF_ANY);
    }

    return 0;
}

SEC("tracepoint/sys_exit_sendmmsg")
int tracepoint__sys_exit_sendmmsg(struct trace_event_raw_sys_exit *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();
    int num_msgs = ctx->ret;

    const struct connect_args_t* connect_args = bpf_map_lookup_elem(&active_connect_args_map, &id);
    if (connect_args != NULL && num_msgs > 0) {
        process_implicit_conn(ctx, id, connect_args);
        bpf_map_delete_elem(&active_connect_args_map, &id);
    }

    struct data_args_t* write_args = bpf_map_lookup_elem(&active_write_args_map, &id);
    if (write_args != NULL && num_msgs > 0) {
        process_syscall_data_vecs(ctx, id, kEgress, write_args, write_args->msg_len);
        bpf_map_delete_elem(&active_write_args_map, &id);
    }

    return 0;
}

SEC("tracepoint/sys_enter_read")
int tracepoint__sys_enter_read(struct trace_event_raw_sys_enter *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();

    struct data_args_t read_args = {};
    read_args.fd = (int)ctx->args[0];
    read_args.buf = (char*)ctx->args[1];
    bpf_map_update_elem(&active_read_args_map, &id, &read_args, BPF_ANY);

    return 0;
}

SEC("tracepoint/sys_exit_read")
int tracepoint__sys_exit_read(struct trace_event_raw_sys_exit *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();
    ssize_t bytes_count = ctx->ret;

    struct data_args_t* read_args = bpf_map_lookup_elem(&active_read_args_map, &id);
    if (read_args != NULL) {
        process_plaintext_data(ctx, id, kIngress, read_args, ctx->ret);
        bpf_map_delete_elem(&active_read_args_map, &id);
    }

    return 0;
}

SEC("tracepoint/sys_enter_readv")
int tracepoint__sys_enter_readv(struct trace_event_raw_sys_enter *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();

    struct data_args_t read_args = {};
    read_args.fd = (int)ctx->args[0];
    read_args.iov = (struct iovec*)ctx->args[1];
    read_args.iovlen = (int)ctx->args[2];
    bpf_map_update_elem(&active_read_args_map, &id, &read_args, BPF_ANY);

    return 0;
}

SEC("tracepoint/sys_exit_readv")
int tracepoint__sys_exit_readv(struct trace_event_raw_sys_exit *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();
    ssize_t bytes_count = ctx->ret;

    struct data_args_t* read_args = bpf_map_lookup_elem(&active_read_args_map, &id);
    if (read_args != NULL) {
        process_syscall_data_vecs(ctx, id, kIngress, read_args, bytes_count);
        bpf_map_delete_elem(&active_read_args_map, &id);
    }

    return 0;
}

SEC("tracepoint/sys_enter_recvfrom")
int tracepoint__sys_enter_recvfrom(struct trace_event_raw_sys_enter *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();

    int sockfd = (int)ctx->args[0];
    struct sockaddr* src_addr = (struct sockaddr*)ctx->args[4];
    if (src_addr != NULL) {
        struct connect_args_t connect_args = {};
        connect_args.fd = sockfd;
        connect_args.addr = src_addr;
        bpf_map_update_elem(&active_connect_args_map, &id, &connect_args, BPF_ANY);
    }

    struct data_args_t read_args = {};
    read_args.fd = sockfd;
    read_args.buf = (char *)ctx->args[1];
    bpf_map_update_elem(&active_read_args_map, &id, &read_args, BPF_ANY);

    return 0;
}

SEC("tracepoint/sys_exit_recvfrom")
int tracepoint__sys_exit_recvfrom(struct trace_event_raw_sys_exit *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();
    ssize_t bytes_count = ctx->ret;

    const struct connect_args_t* connect_args = bpf_map_lookup_elem(&active_connect_args_map, &id);
    if (connect_args != NULL && bytes_count > 0) {
        process_implicit_conn(ctx, id, connect_args);
        bpf_map_delete_elem(&active_connect_args_map, &id);
    }

    struct data_args_t* read_args = bpf_map_lookup_elem(&active_read_args_map, &id);
    if (read_args != NULL) {
        process_plaintext_data(ctx, id, kIngress, read_args, bytes_count);
        bpf_map_delete_elem(&active_read_args_map, &id);
    }

    return 0;
}

SEC("tracepoint/sys_enter_recvmsg")
int tracepoint__sys_enter_recvmsg(struct trace_event_raw_sys_enter *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();
    struct user_msghdr* msghdr = (struct user_msghdr*)ctx->args[1];
    if (msghdr == NULL) {
        return 0;
    }

    int sockfd = (int)ctx->args[0];
    void *msg_name = BPF_CORE_READ_USER(msghdr, msg_name);
    if (msg_name != NULL) {
        struct connect_args_t connect_args = {};
        connect_args.fd = sockfd;
        connect_args.addr = msg_name;
        bpf_map_update_elem(&active_connect_args_map, &id, &connect_args, BPF_ANY);
    }

    struct data_args_t read_args = {};
    read_args.fd = sockfd;
    read_args.iov = BPF_CORE_READ_USER(msghdr, msg_iov);
    read_args.iovlen = BPF_CORE_READ_USER(msghdr, msg_iovlen);
    bpf_map_update_elem(&active_read_args_map, &id, &read_args, BPF_ANY);

    return 0;
}

SEC("tracepoint/sys_exit_recvmsg")
int tracepoint__sys_exit_recvmsg(struct trace_event_raw_sys_exit *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();
    ssize_t bytes_count = ctx->ret;

    const struct connect_args_t* connect_args = bpf_map_lookup_elem(&active_connect_args_map, &id);
    if (connect_args != NULL && bytes_count > 0) {
        process_implicit_conn(ctx, id, connect_args);
        bpf_map_delete_elem(&active_connect_args_map, &id);
    }

    struct data_args_t* read_args = bpf_map_lookup_elem(&active_read_args_map, &id);
    if (read_args != NULL) {
        process_syscall_data_vecs(ctx, id, kIngress, read_args, bytes_count);
        bpf_map_delete_elem(&active_read_args_map, &id);
    }

    return 0;
}

SEC("tracepoint/sys_enter_recvmmsg")
int tracepoint__sys_enter_recvmmsg(struct trace_event_raw_sys_enter *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();
    struct mmsghdr* msgvec = (struct mmsghdr*)ctx->args[1];
    unsigned int vlen = (unsigned int)ctx->args[2];

    if (msgvec == NULL || vlen < 1) {
        return 0;
    }

    int sockfd = (int)ctx->args[0];
    void *msg_name = BPF_CORE_READ_USER(msgvec, msg_hdr.msg_name);
    if (msg_name != NULL) {
        struct connect_args_t connect_args = {0};
        connect_args.fd = sockfd;
        connect_args.addr = msg_name;
        bpf_map_update_elem(&active_connect_args_map, &id, &connect_args, BPF_ANY);
    }

    struct data_args_t read_args = {};
    read_args.fd = sockfd;
    read_args.iov = BPF_CORE_READ_USER(msgvec, msg_hdr.msg_iov);
    read_args.iovlen = BPF_CORE_READ_USER(msgvec, msg_hdr.msg_iovlen);
    read_args.msg_len = BPF_CORE_READ_USER(msgvec, msg_len);
    bpf_map_update_elem(&active_read_args_map, &id, &read_args, BPF_ANY);

    return 0;
}

SEC("tracepoint/sys_exit_recvmmsg")
int tracepoint__sys_exit_recvmmsg(struct trace_event_raw_sys_exit *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();
    int num_msgs = ctx->ret;

    const struct connect_args_t* connect_args = bpf_map_lookup_elem(&active_connect_args_map, &id);
    if (connect_args != NULL && num_msgs > 0) {
        process_implicit_conn(ctx, id, connect_args);
        bpf_map_delete_elem(&active_connect_args_map, &id);
    }

    struct data_args_t* read_args = bpf_map_lookup_elem(&active_read_args_map, &id);
    if (read_args != NULL && num_msgs > 0) {
        process_syscall_data_vecs(ctx, id, kIngress, read_args, read_args->msg_len);
        bpf_map_delete_elem(&active_read_args_map, &id);
    }

    return 0;
}

SEC("tracepoint/sys_enter_close")
int tracepoint__sys_enter_close(struct trace_event_raw_sys_enter *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();

    struct close_args_t close_args = {0};
    close_args.fd = (int)ctx->args[0];
    bpf_map_update_elem(&active_close_args_map, &id, &close_args, BPF_ANY);

    return 0;
}

SEC("tracepoint/sys_exit_close")
int tracepoint__sys_exit_close(struct trace_event_raw_sys_exit *ctx)
{
    uint64_t id = bpf_get_current_pid_tgid();

    const struct close_args_t* close_args = bpf_map_lookup_elem(&active_close_args_map, &id);
    if (close_args != NULL) {
        process_syscall_close(ctx, id, ctx->ret, close_args);
        bpf_map_delete_elem(&active_close_args_map, &id);
    }

    return 0;
}

SEC("kprobe/security_socket_accept")
int BPF_KPROBE(kprobe_security_socket_accept, struct socket *sock, struct socket *newsock)
{
    uint64_t id = bpf_get_current_pid_tgid();

    // Only trace sock_alloc() called by accept()/accept4().
    struct accept_args_t* accept_args = bpf_map_lookup_elem(&active_accept_args_map, &id);
    if (accept_args != NULL) {
        accept_args->sock_alloc_socket = newsock;
    }
    return 0;
}

SEC("kprobe/security_socket_connect")
int BPF_KPROBE(kprobe_security_socket_connect, struct socket* socket, struct sockaddr *address, int addrlen)
{
    uint64_t id = bpf_get_current_pid_tgid();
    struct connect_args_t* connect_args = bpf_map_lookup_elem(&active_connect_args_map, &id);
    // Only trace invocations preceded called by connect().
    if (connect_args != NULL) {
        connect_args->sock_lookup_socket = socket;
    }
    return 0;
}

SEC("uprobe/ssl_read_ex")
int uprobe__ssl_read_ex(struct pt_regs *ctx) {
    uint64_t id = bpf_get_current_pid_tgid();
    struct tls_data_args_t read_args = {};
    read_args.buf = (char*)PT_REGS_PARM2(ctx);
    read_args.tls_output_size = (size_t*)PT_REGS_PARM4(ctx);

    int *fd = get_tls_fd_from_context((uint64_t)PT_REGS_PARM1(ctx), id);
    if (fd != NULL) {
        read_args.fd = *fd;
    }
    bpf_map_update_elem(&tls_read_args_map, &id, &read_args, BPF_ANY);
    return 0;
}

SEC("uretprobe/ssl_read_ex")
int uretprobe__ssl_read_ex(struct pt_regs *ctx) {
    uint64_t id = bpf_get_current_pid_tgid();
    struct tls_data_args_t* read_args = bpf_map_lookup_elem(&tls_read_args_map, &id);
    if (read_args != NULL) {
        ssize_t ret = PT_REGS_RC(ctx);
        if (ret == 1) {
            size_t bytes_count = 0;
            bpf_probe_read_user(&bytes_count, sizeof(bytes_count), read_args->tls_output_size);
            struct data_args_t read_args_local = {};
            read_args_local.fd = read_args->fd;
            read_args_local.buf = read_args->buf;
            process_tls_data(ctx, id, kIngress, &read_args_local, bytes_count);
        }
        bpf_map_delete_elem(&tls_read_args_map, &id);
    }

    return 0;
}

SEC("uprobe/ssl_write_ex")
int uprobe__ssl_write_ex(struct pt_regs *ctx) {
    uint64_t id = bpf_get_current_pid_tgid();
    struct tls_data_args_t write_args = {};
    write_args.buf = (const char *) PT_REGS_PARM2(ctx);
    write_args.tls_output_size = (size_t*)PT_REGS_PARM4(ctx);

    int *fd = get_tls_fd_from_context((uint64_t)PT_REGS_PARM1(ctx), id);
    if (fd != NULL) {
        write_args.fd = *fd;
    }
    bpf_map_update_elem(&tls_write_args_map, &id, &write_args, BPF_ANY);
    return 0;
}

SEC("uretprobe/ssl_write_ex")
int uretprobe__ssl_write_ex(struct pt_regs *ctx) {
    uint64_t id = bpf_get_current_pid_tgid();

    struct tls_data_args_t* write_args = bpf_map_lookup_elem(&tls_write_args_map, &id);
    if (write_args != NULL) {
        ssize_t ret = PT_REGS_RC(ctx);
        if (ret == 1) {
            size_t bytes_count = 0;
            bpf_probe_read_user(&bytes_count, sizeof(bytes_count), write_args->tls_output_size);
            struct data_args_t write_args_local = {};
            write_args_local.fd = write_args->fd;
            write_args_local.buf = write_args->buf;
            process_tls_data(ctx, id, kEgress, &write_args_local, bytes_count);
        }
        bpf_map_delete_elem(&tls_write_args_map, &id);
    }

    return 0;
}

SEC("uprobe/ssl_read")
int uprobe__ssl_read(struct pt_regs *ctx) {
    uint64_t id = bpf_get_current_pid_tgid();
    struct tls_data_args_t read_args = {};
    read_args.buf = (char*)PT_REGS_PARM2(ctx);

    int *fd = get_tls_fd_from_context((uint64_t)PT_REGS_PARM1(ctx), id);
    if (fd != NULL) {
        read_args.fd = *fd;
    }

    bpf_map_update_elem(&tls_read_args_map, &id, &read_args, BPF_ANY);
    return 0;
}

SEC("uretprobe/ssl_read")
int uretprobe__ssl_read(struct pt_regs *ctx) {
    uint64_t id = bpf_get_current_pid_tgid();
    struct tls_data_args_t* read_args = bpf_map_lookup_elem(&tls_read_args_map, &id);
    if (read_args != NULL) {
        struct data_args_t read_args_local = {};
        read_args_local.fd = read_args->fd;
        read_args_local.buf = read_args->buf;
        process_tls_data(ctx, id, kIngress, &read_args_local, PT_REGS_RC(ctx));
        bpf_map_delete_elem(&tls_read_args_map, &id);
    }

    return 0;
}

SEC("uprobe/ssl_write")
int uprobe__ssl_write(struct pt_regs *ctx) {
    uint64_t id = bpf_get_current_pid_tgid();
    struct tls_data_args_t write_args = {};
    write_args.buf = (const char *) PT_REGS_PARM2(ctx);

    int *fd = get_tls_fd_from_context((uint64_t)PT_REGS_PARM1(ctx), id);
    if (fd != NULL) {
        write_args.fd = *fd;
    }

    bpf_map_update_elem(&tls_write_args_map, &id, &write_args, BPF_ANY);
    return 0;
}

SEC("uretprobe/ssl_write")
int uretprobe__ssl_write(struct pt_regs *ctx) {
    uint64_t id = bpf_get_current_pid_tgid();

    struct tls_data_args_t* write_args = bpf_map_lookup_elem(&tls_write_args_map, &id);
    if (write_args != NULL) {
        struct data_args_t write_args_local = {};
        write_args_local.fd = write_args->fd;
        write_args_local.buf = write_args->buf;
        process_tls_data(ctx, id, kEgress, &write_args_local, PT_REGS_RC(ctx));
        bpf_map_delete_elem(&tls_write_args_map, &id);
    }

    return 0;
}

SEC("uprobe/ssl_set_fd")
int uprobe__ssl_set_fd(struct pt_regs *ctx) {
    uint64_t id = bpf_get_current_pid_tgid();
    struct tls_set_fd_args_t args = {};
    args.tls_context = (void*) PT_REGS_PARM1(ctx);
    args.fd = (int) PT_REGS_PARM2(ctx);
    bpf_map_update_elem(&tls_set_fd_args_map, &id, &args, BPF_ANY);

    return 0;
}

SEC("uretprobe/ssl_set_fd")
int uretprobe__ssl_set_fd(struct pt_regs *ctx) {
    uint64_t id = bpf_get_current_pid_tgid();

    struct tls_set_fd_args_t *args = bpf_map_lookup_elem(&tls_set_fd_args_map, &id);
    if (args != NULL) {
        mark_connection_as_tls(id, args->fd);

        struct tls_ctx_to_fd_key_t tls_ctx_to_fd_key = {};
        tls_ctx_to_fd_key.id = id;
        tls_ctx_to_fd_key.tls_context_as_number = (uint64_t)args->tls_context;
        bpf_map_update_elem(&tls_ctx_to_fd_map, &tls_ctx_to_fd_key, &args->fd, BPF_ANY);

        bpf_map_delete_elem(&tls_set_fd_args_map, &id);
    }

    return 0;
}


char LICENSE[] SEC("license") = "GPL";
