#include "kconfig.h"
#include "bpf_telemetry.h"
#include "bpf_builtins.h"
#include "bpf_tracing.h"
#include "tracer.h"

#include "protocols/protocol-classification-helpers.h"
#include "tracer-events.h"
#include "tracer-maps.h"
#include "tracer-stats.h"
#include "tracer-telemetry.h"
#include "bpf_endian.h"
#include "ip.h"
#include "netns.h"
#include "sockfd.h"
#include "skb.h"
#include "port.h"
#include "tcp-recv.h"

#ifdef FEATURE_IPV6_ENABLED
#include "ipv6.h"
#endif

#include <linux/version.h>
#include <net/inet_sock.h>
#include <net/net_namespace.h>
#include <net/route.h>
#include <net/tcp_states.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/ipv6.h>
#include <uapi/linux/ptrace.h>
#include <linux/tcp.h>
#include <uapi/linux/udp.h>

#ifndef LINUX_VERSION_CODE
#error "kernel version not included?"
#endif

#include "conn-tuple.h"
#include "sock.h"

// The entrypoint for all packets.
SEC("socket/classifier")
int socket__classifier(struct __sk_buff *skb) {
    protocol_classifier_entrypoint(skb);
    return 0;
}

SEC("kprobe/tcp_sendmsg")
int kprobe__tcp_sendmsg(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_sendmsg: pid_tgid: %d\n", pid_tgid);
#if LINUX_VERSION_CODE < KERNEL_VERSION(4, 1, 0)
    struct sock *parm1 = (struct sock *)PT_REGS_PARM2(ctx);
#else
    struct sock *parm1 = (struct sock *)PT_REGS_PARM1(ctx);
#endif
    struct sock *skp = parm1;
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

    log_debug("kretprobe/tcp_sendmsg: pid_tgid: %d, sent: %d, sock: %x\n", pid_tgid, sent, skp);
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

#ifdef FEATURE_IPV6_ENABLED
SEC("kprobe/ip6_make_skb")
int kprobe__ip6_make_skb(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    size_t len = (size_t)PT_REGS_PARM4(ctx);
    // commit: https://github.com/torvalds/linux/commit/26879da58711aa604a1b866cbeedd7e0f78f90ad
    // changed the arguments to ip6_make_skb and introduced the struct ipcm6_cookie
#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 7, 0)
    struct flowi6 *fl6 = (struct flowi6 *)PT_REGS_PARM7(ctx);
#else
    struct flowi6 *fl6 = (struct flowi6 *)PT_REGS_PARM9(ctx);
#endif

    u64 pid_tgid = bpf_get_current_pid_tgid();
    ip_make_skb_args_t args = {};
    bpf_probe_read_kernel_with_telemetry(&args.sk, sizeof(args.sk), &sk);
    bpf_probe_read_kernel_with_telemetry(&args.len, sizeof(args.len), &len);
    bpf_probe_read_kernel_with_telemetry(&args.fl6, sizeof(args.fl6), &fl6);
    bpf_map_update_with_telemetry(ip_make_skb_args, &pid_tgid, &args, BPF_ANY);

    return 0;
}

SEC("kretprobe/ip6_make_skb")
int kretprobe__ip6_make_skb(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    ip_make_skb_args_t *args = bpf_map_lookup_elem(&ip_make_skb_args, &pid_tgid);
    if (!args) {
        return 0;
    }

    struct sock *sk = args->sk;
    struct flowi6 *fl6 = args->fl6;
    size_t size = args->len;
    bpf_map_delete_elem(&ip_make_skb_args, &pid_tgid);

    void *rc = (void *)PT_REGS_RC(ctx);
    if (IS_ERR_OR_NULL(rc)) {
        return 0;
    }

    size = size - sizeof(struct udphdr);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP)) {
        read_in6_addr(&t.saddr_h, &t.saddr_l, &fl6->saddr);
        read_in6_addr(&t.daddr_h, &t.daddr_l, &fl6->daddr);

        if (!(t.saddr_h || t.saddr_l)) {
            log_debug("ERR(fl6): src addr not set src_l:%d,src_h:%d\n", t.saddr_l, t.saddr_h);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }
        if (!(t.daddr_h || t.daddr_l)) {
            log_debug("ERR(fl6): dst addr not set dst_l:%d,dst_h:%d\n", t.daddr_l, t.daddr_h);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }

        // Check if we can map IPv6 to IPv4
        if (is_ipv4_mapped_ipv6(t.saddr_h, t.saddr_l, t.daddr_h, t.daddr_l)) {
            t.metadata |= CONN_V4;
            t.saddr_h = 0;
            t.daddr_h = 0;
            t.saddr_l = (u32)(t.saddr_l >> 32);
            t.daddr_l = (u32)(t.daddr_l >> 32);
        } else {
            t.metadata |= CONN_V6;
        }

        bpf_probe_read_kernel_with_telemetry(&t.sport, sizeof(t.sport), &fl6->fl6_sport);
        bpf_probe_read_kernel_with_telemetry(&t.dport, sizeof(t.dport), &fl6->fl6_dport);

        if (t.sport == 0 || t.dport == 0) {
            log_debug("ERR(fl6): src/dst port not set: src:%d, dst:%d\n", t.sport, t.dport);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }

        t.sport = bpf_ntohs(t.sport);
        t.dport = bpf_ntohs(t.dport);
    }

    log_debug("kprobe/ip6_make_skb: pid_tgid: %d, size: %d\n", pid_tgid, size);
    handle_message(&t, size, 0, CONN_DIRECTION_UNKNOWN, 1, 0, PACKET_COUNT_INCREMENT, sk);
    increment_telemetry_count(udp_send_processed);

    return 0;
}
#endif

// Note: This is used only in the UDP send path.
SEC("kprobe/ip_make_skb")
int kprobe__ip_make_skb(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    size_t len = (size_t)PT_REGS_PARM5(ctx);
    struct flowi4 *fl4 = (struct flowi4 *)PT_REGS_PARM2(ctx);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    ip_make_skb_args_t args = {};
    bpf_probe_read_kernel_with_telemetry(&args.sk, sizeof(args.sk), &sk);
    bpf_probe_read_kernel_with_telemetry(&args.len, sizeof(args.len), &len);
    bpf_probe_read_kernel_with_telemetry(&args.fl4, sizeof(args.fl4), &fl4);
    bpf_map_update_with_telemetry(ip_make_skb_args, &pid_tgid, &args, BPF_ANY);

    return 0;
}

SEC("kretprobe/ip_make_skb")
int kretprobe__ip_make_skb(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    ip_make_skb_args_t *args = bpf_map_lookup_elem(&ip_make_skb_args, &pid_tgid);
    if (!args) {
        return 0;
    }

    struct sock *sk = args->sk;
    struct flowi4 *fl4 = args->fl4;
    size_t size = args->len;
    bpf_map_delete_elem(&ip_make_skb_args, &pid_tgid);

    void *rc = (void *)PT_REGS_RC(ctx);
    if (IS_ERR_OR_NULL(rc)) {
        return 0;
    }

    size = size - sizeof(struct udphdr);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP)) {
        bpf_probe_read_kernel_with_telemetry(&t.saddr_l, sizeof(__be32), &fl4->saddr);
        bpf_probe_read_kernel_with_telemetry(&t.daddr_l, sizeof(__be32), &fl4->daddr);
        if (!t.saddr_l || !t.daddr_l) {
            log_debug("ERR(fl4): src/dst addr not set src:%d,dst:%d\n", t.saddr_l, t.daddr_l);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }

        bpf_probe_read_kernel_with_telemetry(&t.sport, sizeof(t.sport), &fl4->fl4_sport);
        bpf_probe_read_kernel_with_telemetry(&t.dport, sizeof(t.dport), &fl4->fl4_dport);
        t.sport = bpf_ntohs(t.sport);
        t.dport = bpf_ntohs(t.dport);
        if (t.sport == 0 || t.dport == 0) {
            log_debug("ERR(fl4): src/dst port not set: src:%d, dst:%d\n", t.sport, t.dport);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }
    }

    log_debug("kprobe/ip_send_skb: pid_tgid: %d, size: %d\n", pid_tgid, size);
    handle_message(&t, size, 0, CONN_DIRECTION_UNKNOWN, 1, 0, PACKET_COUNT_INCREMENT, sk);
    increment_telemetry_count(udp_send_processed);

    return 0;
}

static __always_inline void handle_skb_consume_udp(struct sock *sk, struct sk_buff *skb, int len) {
    if (len < 0) {
        // peeking or an error happened
        return;
    }
    conn_tuple_t t;
    bpf_memset(&t, 0, sizeof(conn_tuple_t));
    int data_len = sk_buff_to_tuple(skb, &t);
    if (data_len <= 0) {
        log_debug("ERR(skb_consume_udp): error reading tuple ret=%d\n", data_len);
        return;
    }
    // we are receiving, so we want the daddr to become the laddr
    flip_tuple(&t);

    log_debug("skb_consume_udp: bytes=%d\n", data_len);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    t.pid = pid_tgid >> 32;
    t.netns = get_netns(&sk->sk_net);
    handle_message(&t, 0, data_len, CONN_DIRECTION_UNKNOWN, 0, 1, PACKET_COUNT_INCREMENT, sk);
}

static __always_inline int handle_udp_recvmsg(struct pt_regs *ctx) {
#if LINUX_VERSION_CODE < KERNEL_VERSION(4, 1, 0)
    int flags = (int)PT_REGS_PARM6(ctx);
#else
    int flags = (int)PT_REGS_PARM5(ctx);
#endif
    log_debug("kprobe/udp_recvmsg: flags: %x\n", flags);
    if (flags & MSG_PEEK) {
        return 0;
    }

    // keep track of non-peeking calls, since skb_free_datagram_locked doesn't have that argument
    u64 pid_tgid = bpf_get_current_pid_tgid();
    udp_recv_sock_t t = {};
    bpf_map_update_with_telemetry(udp_recv_sock, &pid_tgid, &t, BPF_ANY);
    return 0;
}

SEC("kprobe/udp_recvmsg")
int kprobe__udp_recvmsg(struct pt_regs *ctx) {
    return handle_udp_recvmsg(ctx);
}

#ifdef FEATURE_IPV6_ENABLED
SEC("kprobe/udpv6_recvmsg")
int kprobe__udpv6_recvmsg(struct pt_regs *ctx) {
    return handle_udp_recvmsg(ctx);
}
#endif

static __always_inline int handle_udp_recvmsg_ret(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    bpf_map_delete_elem(&udp_recv_sock, &pid_tgid);
    return 0;
}

SEC("kretprobe/udp_recvmsg")
int kretprobe__udp_recvmsg(struct pt_regs *ctx) {
    return handle_udp_recvmsg_ret(ctx);
}

#ifdef FEATURE_IPV6_ENABLED
SEC("kretprobe/udpv6_recvmsg")
int kretprobe__udpv6_recvmsg(struct pt_regs *ctx) {
    return handle_udp_recvmsg_ret(ctx);
}
#endif

SEC("kprobe/skb_free_datagram_locked")
int kprobe__skb_free_datagram_locked(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    udp_recv_sock_t *st = bpf_map_lookup_elem(&udp_recv_sock, &pid_tgid);
    if (!st) { // no entry means a peek
        return 0;
    }

    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    struct sk_buff *skb = (struct sk_buff *)PT_REGS_PARM2(ctx);
    handle_skb_consume_udp(sk, skb, 0);
    return 0;
}

SEC("kprobe/__skb_free_datagram_locked")
int kprobe____skb_free_datagram_locked(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    udp_recv_sock_t *st = bpf_map_lookup_elem(&udp_recv_sock, &pid_tgid);
    if (!st) { // no entry means a peek
        return 0;
    }

    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    struct sk_buff *skb = (struct sk_buff *)PT_REGS_PARM2(ctx);
    int len = (int)PT_REGS_PARM3(ctx);
    handle_skb_consume_udp(sk, skb, len);
    return 0;
}

SEC("kprobe/skb_consume_udp")
int kprobe__skb_consume_udp(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    udp_recv_sock_t *st = bpf_map_lookup_elem(&udp_recv_sock, &pid_tgid);
    if (!st) { // no entry means a peek
        return 0;
    }

    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    struct sk_buff *skb = (struct sk_buff *)PT_REGS_PARM2(ctx);
    int len = (int)PT_REGS_PARM3(ctx);
    handle_skb_consume_udp(sk, skb, len);
    return 0;
}

SEC("kprobe/tcp_retransmit_skb")
int kprobe__tcp_retransmit_skb(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);

#if LINUX_VERSION_CODE < KERNEL_VERSION(4, 7, 0)
    int segs = 1;
#else
    int segs = (int)PT_REGS_PARM3(ctx);
#endif
    log_debug("kprobe/tcp_retransmit\n");

    return handle_retransmit(sk, segs);
}

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
    pb.netns = get_netns(&skp->sk_net);
    pb.port = lport;
    remove_port_bind(&pb, &port_bindings);

    log_debug("kprobe/inet_csk_listen_stop: net ns: %u, lport: %u\n", pb.netns, pb.port);
    return 0;
}

SEC("kprobe/udp_destroy_sock")
int kprobe__udp_destroy_sock(struct pt_regs *ctx) {
    struct sock *skp = (struct sock *)PT_REGS_PARM1(ctx);
    conn_tuple_t tup = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();
    int valid_tuple = read_conn_tuple(&tup, skp, pid_tgid, CONN_TYPE_UDP);

    __u16 lport = 0;
    if (valid_tuple) {
        cleanup_conn(&tup, skp);
        lport = tup.sport;
    } else {
        lport = read_sport(skp);
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
    return 0;
}

SEC("kretprobe/udp_destroy_sock")
int kretprobe__udp_destroy_sock(struct pt_regs *ctx) {
    flush_conn_close_if_full(ctx);
    return 0;
}

//region sys_enter_bind

static __always_inline int sys_enter_bind(struct socket *sock, struct sockaddr *addr) {
    __u64 tid = bpf_get_current_pid_tgid();

    __u16 type = 0;
    bpf_probe_read_kernel_with_telemetry(&type, sizeof(__u16), &sock->type);
    if ((type & SOCK_DGRAM) == 0) {
        return 0;
    }

    if (addr == NULL) {
        log_debug("sys_enter_bind: could not read sockaddr, sock=%llx, tid=%u\n", sock, tid);
        return 0;
    }

    u16 sin_port = 0;
    sa_family_t family = 0;
    bpf_probe_read_kernel_with_telemetry(&family, sizeof(sa_family_t), &addr->sa_family);
    if (family == AF_INET) {
        bpf_probe_read_kernel_with_telemetry(&sin_port, sizeof(u16), &(((struct sockaddr_in *)addr)->sin_port));
    } else if (family == AF_INET6) {
        bpf_probe_read_kernel_with_telemetry(&sin_port, sizeof(u16), &(((struct sockaddr_in6 *)addr)->sin6_port));
    }

    sin_port = bpf_ntohs(sin_port);
    if (sin_port == 0) {
        log_debug("ERR(sys_enter_bind): sin_port is 0\n");
        return 0;
    }

    // write to pending_binds so the retprobe knows we can mark this as binding.
    bind_syscall_args_t args = {};
    args.port = sin_port;

    bpf_map_update_with_telemetry(pending_bind, &tid, &args, BPF_ANY);
    log_debug("sys_enter_bind: started a bind on UDP port=%d sock=%llx tid=%u\n", sin_port, sock, tid);

    return 0;
}

SEC("kprobe/inet_bind")
int kprobe__inet_bind(struct pt_regs *ctx) {
    struct socket *sock = (struct socket *)PT_REGS_PARM1(ctx);
    struct sockaddr *addr = (struct sockaddr *)PT_REGS_PARM2(ctx);
    log_debug("kprobe/inet_bind: sock=%llx, umyaddr=%x\n", sock, addr);
    return sys_enter_bind(sock, addr);
}

SEC("kprobe/inet6_bind")
int kprobe__inet6_bind(struct pt_regs *ctx) {
    struct socket *sock = (struct socket *)PT_REGS_PARM1(ctx);
    struct sockaddr *addr = (struct sockaddr *)PT_REGS_PARM2(ctx);
    log_debug("kprobe/inet6_bind: sock=%llx, umyaddr=%x\n", sock, addr);
    return sys_enter_bind(sock, addr);
}

//endregion

//region sys_exit_bind

static __always_inline int sys_exit_bind(__s64 ret) {
    __u64 tid = bpf_get_current_pid_tgid();

    // bail if this bind() is not the one we're instrumenting
    bind_syscall_args_t *args;
    args = bpf_map_lookup_elem(&pending_bind, &tid);

    log_debug("sys_exit_bind: tid=%u, ret=%d\n", tid, ret);

    if (args == NULL) {
        log_debug("sys_exit_bind: was not a UDP bind, will not process\n");
        return 0;
    }
    __u16 sin_port = args->port;
    bpf_map_delete_elem(&pending_bind, &tid);

    if (ret != 0) {
        return 0;
    }

    port_binding_t pb = {};
    pb.netns = 0; // don't have net ns info in this context
    pb.port = sin_port;
    add_port_bind(&pb, udp_port_bindings);
    log_debug("sys_exit_bind: bound UDP port %u\n", sin_port);

    return 0;
}

SEC("kretprobe/inet_bind")
int kretprobe__inet_bind(struct pt_regs *ctx) {
    __s64 ret = PT_REGS_RC(ctx);
    log_debug("kretprobe/inet_bind: ret=%d\n", ret);
    return sys_exit_bind(ret);
}

SEC("kretprobe/inet6_bind")
int kretprobe__inet6_bind(struct pt_regs *ctx) {
    __s64 ret = PT_REGS_RC(ctx);
    log_debug("kretprobe/inet6_bind: ret=%d\n", ret);
    return sys_exit_bind(ret);
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

    struct proto_ops *proto_ops = NULL;
    bpf_probe_read_kernel_with_telemetry(&proto_ops, sizeof(proto_ops), &socket->ops);
    if (!proto_ops) {
        goto cleanup;
    }

    int family = 0;
    bpf_probe_read_kernel_with_telemetry(&family, sizeof(family), &proto_ops->family);
    if (sock_type != SOCK_STREAM || !(family == AF_INET || family == AF_INET6)) {
        goto cleanup;
    }

    // Retrieve struct sock* pointer from struct socket*
    struct sock *sock = NULL;
    bpf_probe_read_kernel_with_telemetry(&sock, sizeof(sock), &socket->sk);

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

SEC("kprobe/do_sendfile")
int kprobe__do_sendfile(struct pt_regs *ctx) {
    u32 fd_out = (int)PT_REGS_PARM1(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    pid_fd_t key = {
        .pid = pid_tgid >> 32,
        .fd = fd_out,
    };
    struct sock **sock = bpf_map_lookup_elem(&sock_by_pid_fd, &key);
    if (sock == NULL) {
        return 0;
    }

    // bring map value to eBPF stack to satisfy Kernel 4.4 verifier
    struct sock *skp = *sock;
    bpf_map_update_with_telemetry(do_sendfile_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

SEC("kretprobe/do_sendfile")
int kretprobe__do_sendfile(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock **sock = bpf_map_lookup_elem(&do_sendfile_args, &pid_tgid);
    if (sock == NULL) {
        return 0;
    }

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, *sock, pid_tgid, CONN_TYPE_TCP)) {
        goto cleanup;
    }

    ssize_t sent = (ssize_t)PT_REGS_RC(ctx);
    if (sent > 0) {
        __u32 packets_in = 0;
        __u32 packets_out = 0;
        get_tcp_segment_counts(*sock, &packets_in, &packets_out);
        handle_message(&t, sent, 0, CONN_DIRECTION_UNKNOWN, packets_out, packets_in, PACKET_COUNT_ABSOLUTE, *sock);
    }
cleanup:
    bpf_map_delete_elem(&do_sendfile_args, &pid_tgid);
    return 0;
}

// Represents the parameters being passed to the tracepoint net/net_dev_queue
struct net_dev_queue_ctx {
    u64 unused;
    struct sk_buff* skb;
};

SEC("tracepoint/net/net_dev_queue")
int tracepoint__net__net_dev_queue(struct net_dev_queue_ctx* ctx) {
    struct sk_buff* skb = ctx->skb;
    if (!skb) {
        return 0;
    }
    struct sock* sk;
    bpf_probe_read(&sk, sizeof(struct sock*), &skb->sk);
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

//endregion

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
