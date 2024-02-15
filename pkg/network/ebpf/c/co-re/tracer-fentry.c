#include "ktypes.h"
#include "bpf_telemetry.h"
#include "bpf_endian.h"
#include "bpf_tracing.h"

#include "ip.h"
#include "ipv6.h"
#include "sock.h"
#include "skb.h"

#include "tracer/tracer.h"
#include "tracer/events.h"
#include "tracer/bind.h"
#include "tracer/maps.h"
#include "tracer/stats.h"
#include "tracer/telemetry.h"
#include "tracer/port.h"

BPF_PERCPU_HASH_MAP(udp6_send_skb_args, u64, u64, 1024)
BPF_PERCPU_HASH_MAP(udp_send_skb_args, u64, conn_tuple_t, 1024)

#define RETURN_IF_NOT_IN_SYSPROBE_TASK(prog_name)           \
    if (!event_in_task(prog_name)) {                        \
        return 0;                                           \
    }

static __always_inline __u32 systemprobe_dev() {
    __u64 val = 0;
    LOAD_CONSTANT("systemprobe_device", val);
    return (__u32)val;
}

static __always_inline __u32 systemprobe_ino() {
    __u64 val = 0;
    LOAD_CONSTANT("systemprobe_ino", val);
    return (__u32)val;
}

static __always_inline bool event_in_task(char *prog_name) {
    __u32 dev = systemprobe_dev();
    __u32 ino = systemprobe_ino();
    struct bpf_pidns_info ns = {};

    u64 error = bpf_get_ns_current_pid_tgid(dev, ino, &ns, sizeof(struct bpf_pidns_info));

    if (error) {
        log_debug("%s: err=event originates from outside current fargate task", prog_name);
    }

    return !error;
}

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
        log_debug("ERR(fl4): src/dst addr not set src:%d,dst:%d", t->saddr_l, t->daddr_l);
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
        log_debug("ERR(fl4): src/dst port not set: src:%d, dst:%d", t->sport, t->dport);
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
        log_debug("ERR(fl6): src addr not set src_l:%d,src_h:%d", t->saddr_l, t->saddr_h);
        return 0;
    }
    if (!(t->daddr_h || t->daddr_l)) {
        log_debug("ERR(fl6): dst addr not set dst_l:%d,dst_h:%d", t->daddr_l, t->daddr_h);
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
        log_debug("ERR(fl6): src/dst port not set: src:%d, dst:%d", t->sport, t->dport);
        return 0;
    }


    return 1;
}

SEC("fexit/tcp_sendmsg")
int BPF_PROG(tcp_sendmsg_exit, struct sock *sk, struct msghdr *msg, size_t size, int sent) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/tcp_sendmsg");
    if (sent < 0) {
        log_debug("fexit/tcp_sendmsg: tcp_sendmsg err=%d", sent);
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("fexit/tcp_sendmsg: pid_tgid: %d, sent: %d, sock: %llx", pid_tgid, sent, sk);

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
RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/tcp_sendpage");
    if (sent < 0) {
        log_debug("fexit/tcp_sendpage: err=%d", sent);
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("fexit/tcp_sendpage: pid_tgid: %d, sent: %d, sock: %llx", pid_tgid, sent, sk);

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
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/udp_sendpage");
    if (sent < 0) {
        log_debug("fexit/udp_sendpage: err=%d", sent);
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("fexit/udp_sendpage: pid_tgid: %d, sent: %d, sock: %llx", pid_tgid, sent, sk);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP)) {
        return 0;
    }

    return handle_message(&t, sent, 0, CONN_DIRECTION_UNKNOWN, 0, 0, PACKET_COUNT_NONE, sk);
}

SEC("fexit/tcp_recvmsg")
int BPF_PROG(tcp_recvmsg_exit, struct sock *sk, struct msghdr *msg, size_t len, int flags, int *addr_len, int copied) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/tcp_recvmsg");
    if (copied < 0) { // error
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    return handle_tcp_recv(pid_tgid, sk, copied);
}

SEC("fexit/tcp_recvmsg")
int BPF_PROG(tcp_recvmsg_exit_pre_5_19_0, struct sock *sk, struct msghdr *msg, size_t len, int nonblock, int flags, int *addr_len, int copied) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/tcp_recvmsg");
    if (copied < 0) { // error
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    return handle_tcp_recv(pid_tgid, sk, copied);
}

SEC("fentry/tcp_close")
int BPF_PROG(tcp_close, struct sock *sk, long timeout) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fentry/tcp_close");
    conn_tuple_t t = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();

    // Should actually delete something only if the connection never got established
    bpf_map_delete_elem(&tcp_ongoing_connect_pid, &sk);

    // Get network namespace id
    log_debug("fentry/tcp_close: tgid: %u, pid: %u", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }
    log_debug("fentry/tcp_close: netns: %u, sport: %u, dport: %u", t.netns, t.sport, t.dport);

    cleanup_conn(ctx, &t, sk);
    return 0;
}

SEC("fexit/tcp_close")
int BPF_PROG(tcp_close_exit, struct sock *sk, long timeout) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/tcp_close");
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
        log_debug("udp_sendmsg: sent: %d", sent);
        handle_message(t, sent, 0, CONN_DIRECTION_UNKNOWN, 1, 0, PACKET_COUNT_NONE, sk);
    }

    bpf_map_delete_elem(&udp_send_skb_args, &pid_tgid);
    return 0;
}

SEC("kprobe/udp_v6_send_skb")
int kprobe__udp_v6_send_skb(struct pt_regs *ctx) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("kprobe/udp_v6_send_skb");
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
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/udpv6_sendmsg");
    return handle_udp_send(sk, sent);
}

SEC("kprobe/udp_send_skb")
int kprobe__udp_send_skb(struct pt_regs *ctx) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("kprobe/udp_send_skb");
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
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/udp_sendmsg");
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
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fentry/udp_recvmsg");
    log_debug("fentry/udp_recvmsg: flags: %x", flags);
    return handle_udp_recvmsg(sk, flags);
}

SEC("fentry/udpv6_recvmsg")
int BPF_PROG(udpv6_recvmsg, struct sock *sk, struct msghdr *msg, size_t len, int noblock, int flags) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fentry/udpv6_recvmsg");
    log_debug("fentry/udpv6_recvmsg: flags: %x", flags);
    return handle_udp_recvmsg(sk, flags);
}

SEC("fexit/udp_recvmsg")
int BPF_PROG(udp_recvmsg_exit, struct sock *sk, struct msghdr *msg, size_t len,
             int flags, int *addr_len,
             int copied) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/udp_recvmsg");
    return handle_udp_recvmsg_ret();
}

SEC("fexit/udp_recvmsg")
int BPF_PROG(udp_recvmsg_exit_pre_5_19_0, struct sock *sk, struct msghdr *msg, size_t len, int noblock,
             int flags, int *addr_len,
             int copied) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/udp_recvmsg");
    return handle_udp_recvmsg_ret();
}

SEC("fexit/udpv6_recvmsg")
int BPF_PROG(udpv6_recvmsg_exit, struct sock *sk, struct msghdr *msg, size_t len,
             int flags, int *addr_len,
             int copied) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/udpv6_recvmsg");
    return handle_udp_recvmsg_ret();
}

SEC("fexit/udpv6_recvmsg")
int BPF_PROG(udpv6_recvmsg_exit_pre_5_19_0, struct sock *sk, struct msghdr *msg, size_t len, int noblock,
             int flags, int *addr_len,
             int copied) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/udpv6_recvmsg");
    return handle_udp_recvmsg_ret();
}

SEC("fentry/skb_free_datagram_locked")
int BPF_PROG(skb_free_datagram_locked, struct sock *sk, struct sk_buff *skb) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fentry/skb_free_datagram_locked");
    return handle_skb_consume_udp(sk, skb, 0);
}

SEC("fentry/__skb_free_datagram_locked")
int BPF_PROG(__skb_free_datagram_locked, struct sock *sk, struct sk_buff *skb, int len) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fentry/__skb_free_datagram_locked");
    return handle_skb_consume_udp(sk, skb, len);
}

SEC("fentry/skb_consume_udp")
int BPF_PROG(skb_consume_udp, struct sock *sk, struct sk_buff *skb, int len) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fentry/skb_consume_udp");
    return handle_skb_consume_udp(sk, skb, len);
}

SEC("fentry/tcp_retransmit_skb")
int BPF_PROG(tcp_retransmit_skb, struct sock *sk, struct sk_buff *skb, int segs, int err) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fentry/tcp_retransmit_skb");
    log_debug("fexntry/tcp_retransmit");
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
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/tcp_retransmit_skb");
    log_debug("fexit/tcp_retransmit");
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

SEC("fentry/tcp_connect")
int BPF_PROG(tcp_connect, struct sock *sk) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fentry/tcp_connect");
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("fentry/tcp_connect: tgid: %u, pid: %u", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

    bpf_map_update_with_telemetry(tcp_ongoing_connect_pid, &sk, &pid_tgid, BPF_ANY);

    return 0;
}

SEC("fentry/tcp_finish_connect")
int BPF_PROG(tcp_finish_connect, struct sock *sk, struct sk_buff *skb, int rc) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fentry/tcp_finish_connect");
    u64 *pid_tgid_p = bpf_map_lookup_elem(&tcp_ongoing_connect_pid, &sk);
    if (!pid_tgid_p) {
        return 0;
    }

    u64 pid_tgid = *pid_tgid_p;
    bpf_map_delete_elem(&tcp_ongoing_connect_pid, &sk);
    log_debug("fentry/tcp_finish_connect: tgid: %u, pid: %u", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, sk, TCP_ESTABLISHED);
    handle_message(&t, 0, 0, CONN_DIRECTION_OUTGOING, 0, 0, PACKET_COUNT_NONE, sk);

    log_debug("fentry/tcp_connect: netns: %u, sport: %u, dport: %u", t.netns, t.sport, t.dport);

    return 0;
}

SEC("fexit/inet_csk_accept")
int BPF_PROG(inet_csk_accept_exit, struct sock *_sk, int flags, int *err, bool kern, struct sock *sk) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/inet_csk_accept");
    if (sk == NULL) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("fexit/inet_csk_accept: tgid: %u, pid: %u", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

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
    log_debug("fexit/inet_csk_accept: netns: %u, sport: %u, dport: %u", t.netns, t.sport, t.dport);
    return 0;
}

SEC("fentry/inet_csk_listen_stop")
int BPF_PROG(inet_csk_listen_stop_enter, struct sock *sk) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fentry/inet_csk_listen_stop");
    __u16 lport = read_sport(sk);
    if (lport == 0) {
        log_debug("ERR(inet_csk_listen_stop): lport is 0 ");
        return 0;
    }

    port_binding_t pb = {};
    pb.netns = get_netns_from_sock(sk);
    pb.port = lport;
    remove_port_bind(&pb, &port_bindings);
    log_debug("fentry/inet_csk_listen_stop: net ns: %u, lport: %u", pb.netns, pb.port);
    return 0;
}

static __always_inline int handle_udp_destroy_sock(void *ctx, struct sock *sk) {
    conn_tuple_t tup = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();
    int valid_tuple = read_conn_tuple(&tup, sk, pid_tgid, CONN_TYPE_UDP);

    __u16 lport = 0;
    if (valid_tuple) {
        cleanup_conn(ctx, &tup, sk);
        lport = tup.sport;
    } else {
        // get the port for the current sock
        lport = read_sport(sk);
    }

    if (lport == 0) {
        log_debug("ERR(udp_destroy_sock): lport is 0");
        return 0;
    }

    port_binding_t pb = {};
    pb.netns = get_netns_from_sock(sk);
    pb.port = lport;
    remove_port_bind(&pb, &udp_port_bindings);

    log_debug("fentry/udp_destroy_sock: port %d marked as closed", lport);

    return 0;
}

SEC("fentry/udp_destroy_sock")
int BPF_PROG(udp_destroy_sock, struct sock *sk) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fentry/udp_destroy_sock");
    return handle_udp_destroy_sock(ctx, sk);
}

SEC("fentry/udpv6_destroy_sock")
int BPF_PROG(udpv6_destroy_sock, struct sock *sk) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fentry/udpv6_destroy_sock");
    return handle_udp_destroy_sock(ctx, sk);
}

SEC("fexit/udp_destroy_sock")
int BPF_PROG(udp_destroy_sock_exit, struct sock *sk) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/udp_destroy_sock");
    flush_conn_close_if_full(ctx);
    return 0;
}

SEC("fexit/udpv6_destroy_sock")
int BPF_PROG(udpv6_destroy_sock_exit, struct sock *sk) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/udpv6_destroy_sock");
    flush_conn_close_if_full(ctx);
    return 0;
}

SEC("fentry/inet_bind")
int BPF_PROG(inet_bind_enter, struct socket *sock, struct sockaddr *uaddr, int addr_len) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fentry/inet_bind");
    log_debug("fentry/inet_bind");
    return sys_enter_bind(sock, uaddr);
}

SEC("fentry/inet6_bind")
int BPF_PROG(inet6_bind_enter, struct socket *sock, struct sockaddr *uaddr, int addr_len) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fentry/inet6_bind");
    log_debug("fentry/inet6_bind");
    return sys_enter_bind(sock, uaddr);
}

SEC("fexit/inet_bind")
int BPF_PROG(inet_bind_exit, struct socket *sock, struct sockaddr *uaddr, int addr_len, int rc) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/inet_bind");
    log_debug("fexit/inet_bind: rc=%d", rc);
    return sys_exit_bind(rc);
}

SEC("fexit/inet6_bind")
int BPF_PROG(inet6_bind_exit, struct socket *sock, struct sockaddr *uaddr, int addr_len, int rc) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/inet6_bind");
    log_debug("fexit/inet6_bind: rc=%d", rc);
    return sys_exit_bind(rc);
}

char _license[] SEC("license") = "GPL";
