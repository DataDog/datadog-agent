#ifndef __TCP_H__
#define __TCP_H__

#include "bpf_helpers.h"
#include "bpf_telemetry.h"
#include "tracer-events.h"
#include "tracer-stats.h"
#include "tracer-maps.h"
#include "sock.h"
#include "sockfd.h"
#include "port.h"
#include "skb.h"

SEC("kprobe/tcp_recvmsg")
int kprobe__tcp_recvmsg(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
#if defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE < KERNEL_VERSION(4, 1, 0)
    struct sock *skp = (void*)PT_REGS_PARM2(ctx);
    int flags = (int)PT_REGS_PARM6(ctx);
#else
    struct sock *skp = (void*)PT_REGS_PARM1(ctx);
    int flags = (int)PT_REGS_PARM5(ctx);
#endif
    if (flags & MSG_PEEK) {
        return 0;
    }

    bpf_map_update_with_telemetry(tcp_recvmsg_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

#if defined(COMPILE_CORE) || defined(COMPILE_PREBUILT)

SEC("kprobe/tcp_sendmsg/pre_4_1_0")
int kprobe__tcp_sendmsg__pre_4_1_0(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_sendmsg: pid_tgid: %d\n", pid_tgid);
    struct sock *skp = (struct sock *)PT_REGS_PARM2(ctx);
    bpf_map_update_with_telemetry(tcp_sendmsg_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

SEC("kprobe/tcp_recvmsg/pre_4_1_0")
int kprobe__tcp_recvmsg__pre_4_1_0(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_recvmsg: pid_tgid: %d\n", pid_tgid);
    int flags = (int)PT_REGS_PARM6(ctx);
    if (flags & MSG_PEEK) {
        return 0;
    }

    struct sock *skp = (void*)PT_REGS_PARM2(ctx);
    bpf_map_update_with_telemetry(tcp_recvmsg_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

#endif // COMPILE_CORE || COMPILE_PREBUILT

SEC("kretprobe/tcp_recvmsg")
int kretprobe__tcp_recvmsg(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock **skpp = (struct sock**) bpf_map_lookup_elem(&tcp_recvmsg_args, &pid_tgid);
    if (!skpp) {
        return 0;
    }

    struct sock *skp = *skpp;
    bpf_map_delete_elem(&tcp_recvmsg_args, &pid_tgid);
    if (!skp) {
        return 0;
    }

    int recv = PT_REGS_RC(ctx);
    if (recv < 0) {
        return 0;
    }

    return handle_tcp_recv(pid_tgid, skp, recv);
}

SEC("kprobe/tcp_read_sock")
int kprobe__tcp_read_sock(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    void *parm1 = (void*)PT_REGS_PARM1(ctx);
    struct sock* skp = parm1;
    // we reuse tcp_recvmsg_args here since there is no overlap
    // between the tcp_recvmsg and tcp_read_sock paths
    bpf_map_update_with_telemetry(tcp_recvmsg_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

SEC("kretprobe/tcp_read_sock")
int kretprobe__tcp_read_sock(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    // we reuse tcp_recvmsg_args here since there is no overlap
    // between the tcp_recvmsg and tcp_read_sock paths
    struct sock **skpp = (struct sock**) bpf_map_lookup_elem(&tcp_recvmsg_args, &pid_tgid);
    if (!skpp) {
        return 0;
    }

    struct sock *skp = *skpp;
    bpf_map_delete_elem(&tcp_recvmsg_args, &pid_tgid);
    if (!skp) {
        return 0;
    }

    int recv = PT_REGS_RC(ctx);
    if (recv < 0) {
        return 0;
    }

    return handle_tcp_recv(pid_tgid, skp, recv);
}

SEC("kprobe/tcp_sendmsg")
int kprobe__tcp_sendmsg(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
#if defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE < KERNEL_VERSION(4, 1, 0)
    struct sock *skp = (struct sock *)PT_REGS_PARM2(ctx);
#else
    struct sock *skp = (struct sock *)PT_REGS_PARM1(ctx);
#endif
    log_debug("kprobe/tcp_sendmsg: pid_tgid: %d, sock: %llx\n", pid_tgid, skp);
    bpf_map_update_with_telemetry(tcp_sendmsg_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

SEC("kretprobe/tcp_sendmsg")
int kretprobe__tcp_sendmsg(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock **skpp = (struct sock **)bpf_map_lookup_elem(&tcp_sendmsg_args, &pid_tgid);
    if (!skpp) {
        log_debug("kretprobe/tcp_sendmsg: sock not found\n");
        return 0;
    }

    struct sock *skp = *skpp;
    bpf_map_delete_elem(&tcp_sendmsg_args, &pid_tgid);

    int sent = PT_REGS_RC(ctx);
    if (sent < 0) {
        return 0;
    }

    if (!skp) {
        return 0;
    }

    log_debug("kretprobe/tcp_sendmsg: pid_tgid: %d, sent: %d, sock: %llx\n", pid_tgid, sent, skp);
    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, skp, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, skp, 0);

    __u32 packets_in = 0;
    __u32 packets_out = 0;
    get_tcp_segment_counts(skp, &packets_in, &packets_out);

    return handle_message(&t, sent, 0, CONN_DIRECTION_UNKNOWN, packets_out, packets_in, PACKET_COUNT_ABSOLUTE, skp);
}

SEC("kprobe/tcp_close")
int kprobe__tcp_close(struct pt_regs *ctx) {
    struct sock *sk;
    conn_tuple_t t = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();
    sk = (struct sock *)PT_REGS_PARM1(ctx);

    // Should actually delete something only if the connection never got established
    bpf_map_delete_elem(&tcp_ongoing_connect_pid, &sk);

    clear_sockfd_maps(sk);

    // Get network namespace id
    log_debug("kprobe/tcp_close: tgid: %u, pid: %u\n", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }
    log_debug("kprobe/tcp_close: netns: %u, sport: %u, dport: %u\n", t.netns, t.sport, t.dport);

    cleanup_conn(&t, sk);
    return 0;
}

SEC("kretprobe/tcp_close")
int kretprobe__tcp_close(struct pt_regs *ctx) {
    flush_conn_close_if_full(ctx);
    return 0;
}

#if defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)

SEC("kprobe/tcp_retransmit_skb")
int kprobe__tcp_retransmit_skb(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    u64 tid = bpf_get_current_pid_tgid();
    tcp_retransmit_skb_args_t args = {};
    args.sk = sk;
    args.segs = 0;
    BPF_CORE_READ_INTO(&args.retrans_out_pre, tcp_sk(sk), retrans_out);
    bpf_map_update_with_telemetry(pending_tcp_retransmit_skb, &tid, &args, BPF_ANY);
    return 0;
}

SEC("kretprobe/tcp_retransmit_skb")
int kretprobe__tcp_retransmit_skb(struct pt_regs *ctx) {
    log_debug("kretprobe/tcp_retransmit\n");
    u64 tid = bpf_get_current_pid_tgid();
    if (PT_REGS_RC(ctx) < 0) {
        bpf_map_delete_elem(&pending_tcp_retransmit_skb, &tid);
        return 0;
    }
    tcp_retransmit_skb_args_t *args = bpf_map_lookup_elem(&pending_tcp_retransmit_skb, &tid);
    if (args == NULL) {
        return 0;
    }
    struct sock* sk = args->sk;
    u32 retrans_out_pre = args->retrans_out_pre;
    bpf_map_delete_elem(&pending_tcp_retransmit_skb, &tid);
    u32 retrans_out = 0;
    BPF_CORE_READ_INTO(&retrans_out, tcp_sk(sk), retrans_out);
    return handle_retransmit(sk, retrans_out-retrans_out_pre);
}

#endif // COMPILE_CORE || COMPILE_RUNTIME

SEC("kprobe/tcp_set_state")
int kprobe__tcp_set_state(struct pt_regs *ctx) {
    u8 state = (u8)PT_REGS_PARM2(ctx);

    // For now we're tracking only TCP_ESTABLISHED
    if (state != TCP_ESTABLISHED) {
        return 0;
    }

    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    tcp_stats_t stats = { .state_transitions = (1 << state) };
    update_tcp_stats(&t, stats);

    return 0;
}

SEC("kprobe/tcp_connect")
int kprobe__tcp_connect(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_connect: tgid: %u, pid: %u\n", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);
    struct sock *skp = (struct sock *)PT_REGS_PARM1(ctx);

    bpf_map_update_with_telemetry(tcp_ongoing_connect_pid, &skp, &pid_tgid, BPF_ANY);

    return 0;
}

SEC("kprobe/tcp_finish_connect")
int kprobe__tcp_finish_connect(struct pt_regs *ctx) {
    struct sock *skp = (struct sock *)PT_REGS_PARM1(ctx);
    u64 *pid_tgid_p = bpf_map_lookup_elem(&tcp_ongoing_connect_pid, &skp);
    if (!pid_tgid_p) {
        return 0;
    }

    u64 pid_tgid = *pid_tgid_p;
    bpf_map_delete_elem(&tcp_ongoing_connect_pid, &skp);
    log_debug("kprobe/tcp_finish_connect: tgid: %u, pid: %u\n", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, skp, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, skp, TCP_ESTABLISHED);
    handle_message(&t, 0, 0, CONN_DIRECTION_OUTGOING, 0, 0, PACKET_COUNT_NONE, skp);

    log_debug("kprobe/tcp_connect: netns: %u, sport: %u, dport: %u\n", t.netns, t.sport, t.dport);

    return 0;
}

SEC("kretprobe/inet_csk_accept")
int kretprobe__inet_csk_accept(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_RC(ctx);
    if (!sk) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kretprobe/inet_csk_accept: tgid: %u, pid: %u\n", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }
    handle_tcp_stats(&t, sk, TCP_ESTABLISHED);
    handle_message(&t, 0, 0, CONN_DIRECTION_INCOMING, 0, 0, PACKET_COUNT_NONE, sk);

    port_binding_t pb = {};
    pb.netns = t.netns;
    pb.port = t.sport;
    add_port_bind(&pb, port_bindings);

    log_debug("kretprobe/inet_csk_accept: netns: %u, sport: %u, dport: %u\n", t.netns, t.sport, t.dport);
    return 0;
}

SEC("kprobe/inet_csk_listen_stop")
int kprobe__inet_csk_listen_stop(struct pt_regs *ctx) {
    struct sock *skp = (struct sock *)PT_REGS_PARM1(ctx);
    __u16 lport = read_sport(skp);
    if (lport == 0) {
        log_debug("ERR(inet_csk_listen_stop): lport is 0 \n");
        return 0;
    }

    port_binding_t pb = { .netns = 0, .port = 0 };
    pb.netns = get_netns_from_sock(skp);
    pb.port = lport;
    remove_port_bind(&pb, &port_bindings);

    log_debug("kprobe/inet_csk_listen_stop: net ns: %u, lport: %u\n", pb.netns, pb.port);
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

SEC("kprobe/tcp_sendpage")
int kprobe__tcp_sendpage(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_sendpage: pid_tgid: %d\n", pid_tgid);
    struct sock *skp = (struct sock *)PT_REGS_PARM1(ctx);
    bpf_map_update_with_telemetry(tcp_sendpage_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

SEC("kretprobe/tcp_sendpage")
int kretprobe__tcp_sendpage(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock **skpp = (struct sock **)bpf_map_lookup_elem(&tcp_sendpage_args, &pid_tgid);
    if (!skpp) {
        log_debug("kretprobe/tcp_sendpage: sock not found\n");
        return 0;
    }

    struct sock *skp = *skpp;
    bpf_map_delete_elem(&tcp_sendpage_args, &pid_tgid);

    int sent = PT_REGS_RC(ctx);
    if (sent < 0) {
        return 0;
    }

    if (!skp) {
        return 0;
    }

    log_debug("kretprobe/tcp_sendpage: pid_tgid: %d, sent: %d, sock: %x\n", pid_tgid, sent, skp);
    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, skp, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, skp, 0);

    __u32 packets_in = 0;
    __u32 packets_out = 0;
    get_tcp_segment_counts(skp, &packets_in, &packets_out);

    return handle_message(&t, sent, 0, CONN_DIRECTION_UNKNOWN, packets_out, packets_in, PACKET_COUNT_ABSOLUTE, skp);
}

// Represents the parameters being passed to the tracepoint net/net_dev_queue
struct net_dev_queue_ctx {
    u64 unused;
    struct sk_buff* skb;
};

static __always_inline struct sock* sk_buff_sk(struct sk_buff *skb) {
    struct sock * sk = NULL;
#ifdef COMPILE_PREBUILT
    bpf_probe_read(&sk, sizeof(struct sock*), (char*)skb + offset_sk_buff_sock());
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&sk, skb, sk);
#endif

    return sk;
}

SEC("tracepoint/net/net_dev_queue")
int tracepoint__net__net_dev_queue(struct net_dev_queue_ctx* ctx) {
    struct sk_buff* skb = ctx->skb;
    if (!skb) {
        return 0;
    }
    struct sock* sk = sk_buff_sk(skb);
    if (!sk) {
        return 0;
    }

    conn_tuple_t skb_tup;
    bpf_memset(&skb_tup, 0, sizeof(conn_tuple_t));
    if (sk_buff_to_tuple(skb, &skb_tup) <= 0) {
        return 0;
    }

    if (!(skb_tup.metadata&CONN_TYPE_TCP)) {
        return 0;
    }

    conn_tuple_t sock_tup;
    bpf_memset(&sock_tup, 0, sizeof(conn_tuple_t));
    if (!read_conn_tuple(&sock_tup, sk, 0, CONN_TYPE_TCP)) {
        return 0;
    }
    sock_tup.netns = 0;
    sock_tup.pid = 0;

    if (!is_equal(&skb_tup, &sock_tup)) {
        bpf_map_update_with_telemetry(conn_tuple_to_socket_skb_conn_tuple, &sock_tup, &skb_tup, BPF_NOEXIST);
    }

    return 0;
}

#endif // __TCP_H__
