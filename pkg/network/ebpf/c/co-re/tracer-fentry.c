#include "ktypes.h"
#include "bpf_telemetry.h"
#include "bpf_endian.h"
#include "bpf_tracing.h"

#include "tracer.h"
#include "tracer-events.h"
#include "tracer-maps.h"
#include "tracer-stats.h"
#include "tracer-telemetry.h"
#include "sockfd.h"
#include "ip.h"
#include "ipv6.h"
#include "port.h"
#include "sock.h"
#include "skb.h"

BPF_PERCPU_HASH_MAP(udp6_send_skb_args, u64, u64, 1024)
BPF_PERCPU_HASH_MAP(udp_send_skb_args, u64, conn_tuple_t, 1024)

static __always_inline int read_conn_tuple_partial_from_flowi4(conn_tuple_t *t, struct flowi4 *fl4, u64 pid_tgid, metadata_mask_t type) {
    t->pid = pid_tgid >> 32;
    t->metadata = type;

    if (t->saddr_l == 0) {
        t->saddr_l = BPF_CORE_READ(fl4, saddr);
    }
    if (t->daddr_l == 0) {
        t->daddr_l = BPF_CORE_READ(fl4, daddr);
    }

    if (t->saddr_l == 0 || t->daddr_l == 0) {
        log_debug("ERR(fl4): src/dst addr not set src:%d,dst:%d\n", t->saddr_l, t->daddr_l);
        return 0;
    }

    if (t->sport == 0) {
        t->sport = BPF_CORE_READ(fl4, fl4_sport);
        t->sport = bpf_ntohs(t->sport);
    }
    if (t->dport == 0) {
        t->dport = BPF_CORE_READ(fl4, fl4_dport);
        t->dport = bpf_ntohs(t->dport);
    }

    if (t->sport == 0 || t->dport == 0) {
        log_debug("ERR(fl4): src/dst port not set: src:%d, dst:%d\n", t->sport, t->dport);
        return 0;
    }

    return 1;
}

static __always_inline int read_conn_tuple_partial_from_flowi6(conn_tuple_t *t, struct flowi6 *fl6, u64 pid_tgid, metadata_mask_t type) {
    t->pid = pid_tgid >> 32;
    t->metadata = type;

    struct in6_addr addr = BPF_CORE_READ(fl6, saddr);
    if (t->saddr_l == 0 || t->saddr_h == 0) {
        read_in6_addr(&t->saddr_h, &t->saddr_l, &addr);
    }
    if (t->daddr_l == 0 || t->daddr_h == 0) {
        addr = BPF_CORE_READ(fl6, daddr);
        read_in6_addr(&t->daddr_h, &t->daddr_l, &addr);
    }

    if (!(t->saddr_h || t->saddr_l)) {
        log_debug("ERR(fl6): src addr not set src_l:%d,src_h:%d\n", t->saddr_l, t->saddr_h);
        return 0;
    }
    if (!(t->daddr_h || t->daddr_l)) {
        log_debug("ERR(fl6): dst addr not set dst_l:%d,dst_h:%d\n", t->daddr_l, t->daddr_h);
        return 0;
    }

    // Check if we can map IPv6 to IPv4
    if (is_ipv4_mapped_ipv6(t->saddr_h, t->saddr_l, t->daddr_h, t->daddr_l)) {
        t->metadata |= CONN_V4;
        t->saddr_h = 0;
        t->daddr_h = 0;
        t->saddr_l = (u32)(t->saddr_l >> 32);
        t->daddr_l = (u32)(t->daddr_l >> 32);
    } else {
        t->metadata |= CONN_V6;
    }

    if (t->sport == 0) {
        t->sport = BPF_CORE_READ(fl6, fl6_sport);
        t->sport = bpf_ntohs(t->sport);
    }
    if (t->dport == 0) {
        t->dport = BPF_CORE_READ(fl6, fl6_dport);
        t->dport = bpf_ntohs(t->dport);
    }

    if (t->sport == 0 || t->dport == 0) {
        log_debug("ERR(fl6): src/dst port not set: src:%d, dst:%d\n", t->sport, t->dport);
        return 0;
    }


    return 1;
}

SEC("fexit/tcp_sendmsg")
int BPF_PROG(tcp_sendmsg_exit, struct sock *sk, struct msghdr *msg, size_t size, int sent) {
    if (sent < 0) {
        log_debug("fexit/tcp_sendmsg: tcp_sendmsg err=%d\n", sent);
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("fexit/tcp_sendmsg: pid_tgid: %d, sent: %d, sock: %llx\n", pid_tgid, sent, sk);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, sk, 0);

    __u32 packets_in = 0;
    __u32 packets_out = 0;
    get_tcp_segment_counts(sk, &packets_in, &packets_out);

    return handle_message(&t, sent, 0, CONN_DIRECTION_UNKNOWN, packets_out, packets_in, PACKET_COUNT_ABSOLUTE, sk);
}

SEC("fexit/tcp_sendpage")
int BPF_PROG(tcp_sendpage_exit, struct sock *sk, struct page *page, int offset, size_t size, int flags, int sent) {
    if (sent < 0) {
        log_debug("fexit/tcp_sendpage: err=%d\n", sent);
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("fexit/tcp_sendpage: pid_tgid: %d, sent: %d, sock: %llx\n", pid_tgid, sent, sk);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, sk, 0);

    __u32 packets_in = 0;
    __u32 packets_out = 0;
    get_tcp_segment_counts(sk, &packets_in, &packets_out);

    return handle_message(&t, sent, 0, CONN_DIRECTION_UNKNOWN, packets_out, packets_in, PACKET_COUNT_ABSOLUTE, sk);
}

SEC("fexit/udp_sendpage")
int BPF_PROG(udp_sendpage_exit, struct sock *sk, struct page *page, int offset, size_t size, int flags, int sent) {
    if (sent < 0) {
        log_debug("fexit/udp_sendpage: err=%d\n", sent);
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("fexit/udp_sendpage: pid_tgid: %d, sent: %d, sock: %llx\n", pid_tgid, sent, sk);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP)) {
        return 0;
    }

    return handle_message(&t, sent, 0, CONN_DIRECTION_UNKNOWN, 0, 0, PACKET_COUNT_NONE, sk);
}

SEC("fexit/tcp_recvmsg")
int BPF_PROG(tcp_recvmsg_exit, struct sock *sk, struct msghdr *msg, size_t len, int flags, int *addr_len, int copied) {
    if (copied < 0) { // error
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    return handle_tcp_recv(pid_tgid, sk, copied);
}

SEC("fexit/tcp_recvmsg")
int BPF_PROG(tcp_recvmsg_exit_pre_5_19_0, struct sock *sk, struct msghdr *msg, size_t len, int nonblock, int flags, int *addr_len, int copied) {
    if (copied < 0) { // error
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    return handle_tcp_recv(pid_tgid, sk, copied);
}

SEC("fentry/tcp_close")
int BPF_PROG(tcp_close, struct sock *sk, long timeout) {
    conn_tuple_t t = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();

    // Should actually delete something only if the connection never got established
    bpf_map_delete_elem(&tcp_ongoing_connect_pid, &sk);

    clear_sockfd_maps(sk);

    // Get network namespace id
    log_debug("fentry/tcp_close: tgid: %u, pid: %u\n", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }
    log_debug("fentry/tcp_close: netns: %u, sport: %u, dport: %u\n", t.netns, t.sport, t.dport);

    cleanup_conn(&t, sk);
    return 0;
}

SEC("fexit/tcp_close")
int BPF_PROG(tcp_close_exit, struct sock *sk, long timeout) {
    flush_conn_close_if_full(ctx);
    return 0;
}

static __always_inline int handle_udp_send(struct sock *sk, int sent) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    conn_tuple_t * t = bpf_map_lookup_elem(&udp_send_skb_args, &pid_tgid);
    if (!t) {
        return 0;
    }

    if (sent > 0) {
        log_debug("udp_sendmsg: sent: %d\n", sent);
        handle_message(t, sent, 0, CONN_DIRECTION_UNKNOWN, 1, 0, PACKET_COUNT_NONE, sk);
    }

    bpf_map_delete_elem(&udp_send_skb_args, &pid_tgid);
    return 0;
}

SEC("kprobe/udp_v6_send_skb")
int kprobe__udp_v6_send_skb(struct pt_regs *ctx) {
    struct sk_buff *skb = (struct sk_buff*) PT_REGS_PARM1(ctx);
    struct flowi6 *fl6 = (struct flowi6*) PT_REGS_PARM2(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock *sk = BPF_CORE_READ(skb, sk);
    conn_tuple_t t;
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP) &&
        !read_conn_tuple_partial_from_flowi6(&t, fl6, pid_tgid, CONN_TYPE_UDP)) {
        goto miss;
    }

    bpf_map_update_elem(&udp_send_skb_args, &pid_tgid, &t, BPF_ANY);
    return 0;

 miss:
    increment_telemetry_count(udp_send_missed);
    return 0;
}

SEC("fexit/udpv6_sendmsg")
int BPF_PROG(udpv6_sendmsg_exit, struct sock *sk, struct msghdr *msg, size_t len, int sent) {
    return handle_udp_send(sk, sent);
}

SEC("kprobe/udp_send_skb")
int kprobe__udp_send_skb(struct pt_regs *ctx) {
    struct sk_buff *skb = (struct sk_buff*) PT_REGS_PARM1(ctx);
    struct flowi4 *fl4 = (struct flowi4*) PT_REGS_PARM2(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock *sk = BPF_CORE_READ(skb, sk);
    conn_tuple_t t;
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP) &&
        !read_conn_tuple_partial_from_flowi4(&t, fl4, pid_tgid, CONN_TYPE_UDP)) {
        goto miss;
    }

    bpf_map_update_elem(&udp_send_skb_args, &pid_tgid, &t, BPF_ANY);
    return 0;

 miss:
    increment_telemetry_count(udp_send_missed);
    return 0;
}

SEC("fexit/udp_sendmsg")
int BPF_PROG(udp_sendmsg_exit, struct sock *sk, struct msghdr *msg, size_t len, int sent) {
    return handle_udp_send(sk, sent);
}

static __always_inline int handle_udp_recvmsg(struct sock *sk, int flags) {
    if (flags & MSG_PEEK) {
        return 0;
    }
    // keep track of non-peeking calls, since skb_free_datagram_locked doesn't have that argument
    u64 pid_tgid = bpf_get_current_pid_tgid();
    udp_recv_sock_t t = {};
    bpf_map_update_with_telemetry(udp_recv_sock, &pid_tgid, &t, BPF_ANY);
    return 0;
}

static __always_inline int handle_udp_recvmsg_ret() {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    bpf_map_delete_elem(&udp_recv_sock, &pid_tgid);
    return 0;
}

SEC("fentry/udp_recvmsg")
int BPF_PROG(udp_recvmsg, struct sock *sk, struct msghdr *msg, size_t len, int noblock, int flags) {
    log_debug("fentry/udp_recvmsg: flags: %x\n", flags);
    return handle_udp_recvmsg(sk, flags);
}

SEC("fentry/udpv6_recvmsg")
int BPF_PROG(udpv6_recvmsg, struct sock *sk, struct msghdr *msg, size_t len, int noblock, int flags) {
    log_debug("fentry/udpv6_recvmsg: flags: %x\n", flags);
    return handle_udp_recvmsg(sk, flags);
}

SEC("fexit/udp_recvmsg")
int BPF_PROG(udp_recvmsg_exit, struct sock *sk, struct msghdr *msg, size_t len,
             int flags, int *addr_len,
             int copied) {
    return handle_udp_recvmsg_ret();
}

SEC("fexit/udp_recvmsg")
int BPF_PROG(udp_recvmsg_exit_pre_5_19_0, struct sock *sk, struct msghdr *msg, size_t len, int noblock,
             int flags, int *addr_len,
             int copied) {
    return handle_udp_recvmsg_ret();
}

SEC("fexit/udpv6_recvmsg")
int BPF_PROG(udpv6_recvmsg_exit, struct sock *sk, struct msghdr *msg, size_t len,
             int flags, int *addr_len,
             int copied) {
    return handle_udp_recvmsg_ret();
}

SEC("fexit/udpv6_recvmsg")
int BPF_PROG(udpv6_recvmsg_exit_pre_5_19_0, struct sock *sk, struct msghdr *msg, size_t len, int noblock,
             int flags, int *addr_len,
             int copied) {
    return handle_udp_recvmsg_ret();
}

SEC("fentry/skb_free_datagram_locked")
int BPF_PROG(skb_free_datagram_locked, struct sock *sk, struct sk_buff *skb) {
    return handle_skb_consume_udp(sk, skb, 0);
}

SEC("fentry/__skb_free_datagram_locked")
int BPF_PROG(__skb_free_datagram_locked, struct sock *sk, struct sk_buff *skb, int len) {
    return handle_skb_consume_udp(sk, skb, len);
}

SEC("fentry/skb_consume_udp")
int BPF_PROG(skb_consume_udp, struct sock *sk, struct sk_buff *skb, int len) {
    return handle_skb_consume_udp(sk, skb, len);
}

SEC("fentry/tcp_retransmit_skb")
int BPF_PROG(tcp_retransmit_skb, struct sock *sk, struct sk_buff *skb, int segs, int err) {
    log_debug("fexntry/tcp_retransmit\n");
    u64 tid = bpf_get_current_pid_tgid();
    tcp_retransmit_skb_args_t args = {};
    args.retrans_out_pre = BPF_CORE_READ(tcp_sk(sk), retrans_out);
    if (args.retrans_out_pre < 0) {
        return 0;
    }

    bpf_map_update_with_telemetry(pending_tcp_retransmit_skb, &tid, &args, BPF_ANY);

    return 0;
}

SEC("fexit/tcp_retransmit_skb")
int BPF_PROG(tcp_retransmit_skb_exit, struct sock *sk, struct sk_buff *skb, int segs, int err) {
    log_debug("fexit/tcp_retransmit\n");
    u64 tid = bpf_get_current_pid_tgid();
    if (err < 0) {
        bpf_map_delete_elem(&pending_tcp_retransmit_skb, &tid);
        return 0;
    }
    tcp_retransmit_skb_args_t *args = bpf_map_lookup_elem(&pending_tcp_retransmit_skb, &tid);
    if (args == NULL) {
        return 0;
    }
    u32 retrans_out_pre = args->retrans_out_pre;
    u32 retrans_out = BPF_CORE_READ(tcp_sk(sk), retrans_out);
    bpf_map_delete_elem(&pending_tcp_retransmit_skb, &tid);

    if (retrans_out < 0) {
        return 0;
    }

    return handle_retransmit(sk, retrans_out-retrans_out_pre);
}

SEC("fentry/tcp_set_state")
int BPF_PROG(tcp_set_state, struct sock *sk, int state) {
    // For now we're tracking only TCP_ESTABLISHED
    if (state != TCP_ESTABLISHED) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    tcp_stats_t stats = { .state_transitions = (1 << state) };
    update_tcp_stats(&t, stats);

    return 0;
}

SEC("fentry/tcp_connect")
int BPF_PROG(tcp_connect, struct sock *sk) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("fentry/tcp_connect: tgid: %u, pid: %u\n", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

    bpf_map_update_with_telemetry(tcp_ongoing_connect_pid, &sk, &pid_tgid, BPF_ANY);

    return 0;
}

SEC("fentry/tcp_finish_connect")
int BPF_PROG(tcp_finish_connect, struct sock *sk, struct sk_buff *skb, int rc) {
    u64 *pid_tgid_p = bpf_map_lookup_elem(&tcp_ongoing_connect_pid, &sk);
    if (!pid_tgid_p) {
        return 0;
    }

    u64 pid_tgid = *pid_tgid_p;
    bpf_map_delete_elem(&tcp_ongoing_connect_pid, &sk);
    log_debug("fentry/tcp_finish_connect: tgid: %u, pid: %u\n", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, sk, TCP_ESTABLISHED);
    handle_message(&t, 0, 0, CONN_DIRECTION_OUTGOING, 0, 0, PACKET_COUNT_NONE, sk);

    log_debug("fentry/tcp_connect: netns: %u, sport: %u, dport: %u\n", t.netns, t.sport, t.dport);

    return 0;
}

SEC("fexit/inet_csk_accept")
int BPF_PROG(inet_csk_accept_exit, struct sock *_sk, int flags, int *err, bool kern, struct sock *sk) {
    if (sk == NULL) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("fexit/inet_csk_accept: tgid: %u, pid: %u\n", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

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
    log_debug("fexit/inet_csk_accept: netns: %u, sport: %u, dport: %u\n", t.netns, t.sport, t.dport);
    return 0;
}

SEC("fentry/inet_csk_listen_stop")
int BPF_PROG(inet_csk_listen_stop_enter, struct sock *sk) {
    __u16 lport = read_sport(sk);
    if (lport == 0) {
        log_debug("ERR(inet_csk_listen_stop): lport is 0 \n");
        return 0;
    }

    port_binding_t pb = {};
    pb.netns = get_netns_from_sock(sk);
    pb.port = lport;
    remove_port_bind(&pb, &port_bindings);
    log_debug("fentry/inet_csk_listen_stop: net ns: %u, lport: %u\n", pb.netns, pb.port);
    return 0;
}

static __always_inline int handle_udp_destroy_sock(struct sock *sk) {
    conn_tuple_t tup = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();
    int valid_tuple = read_conn_tuple(&tup, sk, pid_tgid, CONN_TYPE_UDP);

    __u16 lport = 0;
    if (valid_tuple) {
        cleanup_conn(&tup, sk);
        lport = tup.sport;
    } else {
        // get the port for the current sock
        lport = read_sport(sk);
    }

    if (lport == 0) {
        log_debug("ERR(udp_destroy_sock): lport is 0\n");
        return 0;
    }

    // although we have net ns info, we don't use it in the key
    // since we don't have it everywhere for udp port bindings
    // (see sys_enter_bind/sys_exit_bind below)
    port_binding_t pb = {};
    pb.netns = 0;
    pb.port = lport;
    remove_port_bind(&pb, &udp_port_bindings);

    log_debug("fentry/udp_destroy_sock: port %d marked as closed\n", lport);

    return 0;
}

SEC("fentry/udp_destroy_sock")
int BPF_PROG(udp_destroy_sock, struct sock *sk) {
    return handle_udp_destroy_sock(sk);
}

SEC("fentry/udpv6_destroy_sock")
int BPF_PROG(udpv6_destroy_sock, struct sock *sk) {
    return handle_udp_destroy_sock(sk);
}

SEC("fexit/udp_destroy_sock")
int BPF_PROG(udp_destroy_sock_exit, struct sock *sk) {
    flush_conn_close_if_full(ctx);
    return 0;
}

SEC("fexit/udpv6_destroy_sock")
int BPF_PROG(udpv6_destroy_sock_exit, struct sock *sk) {
    flush_conn_close_if_full(ctx);
    return 0;
}

static __always_inline int sys_exit_bind(struct socket *sock, struct sockaddr *addr, int rc) {
    if (rc != 0) {
        return 0;
    }

    __u16 type = BPF_CORE_READ(sock, type);
    if ((type & SOCK_DGRAM) == 0) {
        return 0;
    }

    if (addr == NULL) {
        log_debug("sys_enter_bind: could not read sockaddr, sock=%llx\n", sock);
        return 0;
    }

    u16 sin_port = 0;
    sa_family_t family = addr->sa_family;
    if (family == AF_INET) {
        sin_port = ((struct sockaddr_in *)addr)->sin_port;
    } else if (family == AF_INET6) {
        sin_port = ((struct sockaddr_in6 *)addr)->sin6_port;
    }

    sin_port = bpf_ntohs(sin_port);
    if (sin_port == 0) {
        sin_port = read_sport(BPF_CORE_READ(sock, sk));
    }
    if (sin_port == 0) {
        log_debug("ERR(sys_exit_bind): sin_port is 0\n");
        return 0;
    }

    port_binding_t pb = {};
    pb.netns = 0; // don't have net ns info in this context
    pb.port = sin_port;
    add_port_bind(&pb, udp_port_bindings);
    log_debug("sys_exit_bind: bound UDP port %u\n", sin_port);

    return 0;
}

SEC("fexit/inet_bind")
int BPF_PROG(inet_bind_exit, struct socket *sock, struct sockaddr *uaddr, int addr_len, int rc) {
    log_debug("fexit/inet_bind: rc=%d\n", rc);
    return sys_exit_bind(sock, uaddr, rc);
}

SEC("fexit/inet6_bind")
int BPF_PROG(inet6_bind_exit, struct socket *sock, struct sockaddr *uaddr, int addr_len, int rc) {
    log_debug("fexit/inet6_bind: rc=%d\n", rc);
    return sys_exit_bind(sock, uaddr, rc);
}

// this kretprobe is essentially creating:
// * an index of pid_fd_t to a struct sock*;
// * an index of struct sock* to pid_fd_t;
SEC("fexit/sockfd_lookup_light")
int BPF_PROG(sockfd_lookup_light_exit, int fd, int *err, int *fput_needed, struct socket *socket) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    // Check if have already a map entry for this pid_fd_t
    // TODO: This lookup eliminates *4* map operations for existing entries
    // but can reduce the accuracy of programs relying on socket FDs for
    // processes with a lot of FD churn
    pid_fd_t key = {
        .pid = pid_tgid >> 32,
        .fd = fd,
    };

    struct sock **skpp = bpf_map_lookup_elem(&sock_by_pid_fd, &key);
    if (skpp != NULL) {
        return 0;
    }

    // For now let's only store information for TCP sockets
    const struct proto_ops *proto_ops = BPF_CORE_READ(socket, ops);
    if (!proto_ops) {
        return 0;
    }

    enum sock_type sock_type = BPF_CORE_READ(socket, type);
    int family = BPF_CORE_READ(proto_ops, family);
    if (sock_type != SOCK_STREAM || !(family == AF_INET || family == AF_INET6)) {
        return 0;
    }

    // Retrieve struct sock* pointer from struct socket*
    struct sock *sock = BPF_CORE_READ(socket, sk);

    pid_fd_t pid_fd = {
        .pid = pid_tgid >> 32,
        .fd = fd,
    };

    // These entries are cleaned up by tcp_close
    bpf_map_update_with_telemetry(pid_fd_by_sock, &sock, &pid_fd, BPF_ANY);
    bpf_map_update_with_telemetry(sock_by_pid_fd, &pid_fd, &sock, BPF_ANY);

    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
