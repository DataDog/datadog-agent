#ifndef __TRACER_UDP_H
#define __TRACER_UDP_H

#include "skb.h"
#include "tracer-stats.h"
#include "tracer-maps.h"

#ifdef COMPILE_CORE
#define MAX_ERRNO 4095
#define IS_ERR_VALUE(x) ((unsigned long)(void *)(x) >= (unsigned long)-MAX_ERRNO)

static __always_inline bool IS_ERR_OR_NULL(const void *ptr)
{
    return !ptr || IS_ERR_VALUE((unsigned long)ptr);
}
#else
#include <linux/err.h>
#endif // COMPILE_CORE

static __always_inline void handle_skb_consume_udp(struct sock *sk, struct sk_buff *skb, int len) {
    if (len < 0) {
        // peeking or an error happened
        return;
    }
    conn_tuple_t t = {};
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
    t.netns = get_netns_from_sock(sk);
    handle_message(&t, 0, data_len, CONN_DIRECTION_UNKNOWN, 0, 1, PACKET_COUNT_INCREMENT, sk);
}

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

#define handle_udp_recvmsg(sk, msg, flags, udp_sock_map)                \
    do {                                                                \
        log_debug("kprobe/udp_recvmsg: flags: %x\n", flags);            \
        if (flags & MSG_PEEK) {                                         \
            return 0;                                                   \
        }                                                               \
                                                                        \
        /* keep track of non-peeking calls, since skb_free_datagram_locked doesn't have that argument */ \
        u64 pid_tgid = bpf_get_current_pid_tgid();                      \
        udp_recv_sock_t t = { .sk = sk, .msg = msg };                   \
        bpf_map_update_with_telemetry(udp_sock_map, &pid_tgid, &t, BPF_ANY); \
        return 0;                                                       \
    } while (0);

SEC("kprobe/udp_recvmsg")
int kprobe__udp_recvmsg(struct pt_regs *ctx) {
#if defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE < KERNEL_VERSION(4, 1, 0)
    int flags = (int)PT_REGS_PARM6(ctx);
#else
    int flags = (int)PT_REGS_PARM5(ctx);
#endif
    struct sock *sk = NULL;
    struct msghdr *msg = NULL;
    handle_udp_recvmsg(sk, msg, flags, udp_recv_sock);
}

#if !defined(COMPILE_RUNTIME) || defined(FEATURE_IPV6_ENABLED)
SEC("kprobe/udpv6_recvmsg")
int kprobe__udpv6_recvmsg(struct pt_regs *ctx) {
#if defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE < KERNEL_VERSION(4, 1, 0)
    int flags = (int)PT_REGS_PARM6(ctx);
#else
    int flags = (int)PT_REGS_PARM5(ctx);
#endif
    struct sock *sk = NULL;
    struct msghdr *msg = NULL;
    handle_udp_recvmsg(sk, msg, flags, udp_recv_sock);
}
#endif // !COMPILE_RUNTIME || FEATURE_IPV6_ENABLED

static __always_inline int handle_udp_recvmsg_ret() {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    bpf_map_delete_elem(&udp_recv_sock, &pid_tgid);
    return 0;
}

SEC("kretprobe/udp_recvmsg")
int kretprobe__udp_recvmsg(struct pt_regs *ctx) {
    return handle_udp_recvmsg_ret();
}

#if !defined(COMPILE_RUNTIME) || defined(FEATURE_IPV6_ENABLED)
SEC("kretprobe/udpv6_recvmsg")
int kretprobe__udpv6_recvmsg(struct pt_regs *ctx) {
    return handle_udp_recvmsg_ret();
}
#endif // !COMPILE_RUNTIME || FEATURE_IPV6_ENABLED

#if defined(COMPILE_CORE) || defined(COMPILE_PREBUILT)

static __always_inline int handle_ret_udp_recvmsg_pre_4_7_0(int copied, void *udp_sock_map) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kretprobe/udp_recvmsg: tgid: %u, pid: %u\n", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

    // Retrieve socket pointer from kprobe via pid/tgid
    udp_recv_sock_t *st = bpf_map_lookup_elem(udp_sock_map, &pid_tgid);
    if (!st) { // Missed entry
        return 0;
    }

    if (copied < 0) { // Non-zero values are errors (or a peek) (e.g -EINVAL)
        log_debug("kretprobe/udp_recvmsg: ret=%d < 0, pid_tgid=%d\n", copied, pid_tgid);
        // Make sure we clean up the key
        bpf_map_delete_elem(udp_sock_map, &pid_tgid);
        return 0;
    }

    log_debug("kretprobe/udp_recvmsg: ret=%d\n", copied);

    conn_tuple_t t = {};
    bpf_memset(&t, 0, sizeof(conn_tuple_t));
    if (st->msg) {
        struct sockaddr *sap = NULL;
        bpf_probe_read_kernel_with_telemetry(&sap, sizeof(sap), &(st->msg->msg_name));
        sockaddr_to_addr(sap, &t.daddr_h, &t.daddr_l, &t.dport, &t.metadata);
    }

    if (!read_conn_tuple_partial(&t, st->sk, pid_tgid, CONN_TYPE_UDP)) {
        log_debug("ERR(kretprobe/udp_recvmsg): error reading conn tuple, pid_tgid=%d\n", pid_tgid);
        bpf_map_delete_elem(udp_sock_map, &pid_tgid);
        return 0;
    }
    bpf_map_delete_elem(udp_sock_map, &pid_tgid);

    log_debug("kretprobe/udp_recvmsg: pid_tgid: %d, return: %d\n", pid_tgid, copied);
    // segment count is not currently enabled on prebuilt.
    // to enable, change PACKET_COUNT_NONE => PACKET_COUNT_INCREMENT
    handle_message(&t, 0, copied, CONN_DIRECTION_UNKNOWN, 0, 1, PACKET_COUNT_NONE, st->sk);

    return 0;
}

SEC("kprobe/udp_recvmsg")
int kprobe__udp_recvmsg_pre_4_7_0(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    struct msghdr *msg = (struct msghdr *)PT_REGS_PARM2(ctx);
    int flags = (int)PT_REGS_PARM5(ctx);
    handle_udp_recvmsg(sk, msg, flags, udp_recv_sock);
}

SEC("kprobe/udpv6_recvmsg")
int kprobe__udpv6_recvmsg_pre_4_7_0(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    struct msghdr *msg = (struct msghdr *)PT_REGS_PARM2(ctx);
    int flags = (int)PT_REGS_PARM5(ctx);
    handle_udp_recvmsg(sk, msg, flags, udpv6_recv_sock);
}

SEC("kprobe/udp_recvmsg/pre_4_1_0")
int kprobe__udp_recvmsg_pre_4_1_0(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM2(ctx);
    struct msghdr *msg = (struct msghdr *)PT_REGS_PARM3(ctx);
    int flags = (int)PT_REGS_PARM6(ctx);
    handle_udp_recvmsg(sk, msg, flags, udp_recv_sock);
}

SEC("kprobe/udpv6_recvmsg/pre_4_1_0")
int kprobe__udpv6_recvmsg_pre_4_1_0(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM2(ctx);
    struct msghdr *msg = (struct msghdr *)PT_REGS_PARM3(ctx);
    int flags = (int)PT_REGS_PARM6(ctx);
    handle_udp_recvmsg(sk, msg, flags, udpv6_recv_sock);
}

SEC("kretprobe/udp_recvmsg")
int kretprobe__udp_recvmsg_pre_4_7_0(struct pt_regs *ctx) {
    int copied = (int)PT_REGS_RC(ctx);
    return handle_ret_udp_recvmsg_pre_4_7_0(copied, &udp_recv_sock);
}

SEC("kretprobe/udpv6_recvmsg")
int kretprobe__udpv6_recvmsg_pre_4_7_0(struct pt_regs *ctx) {
    int copied = (int)PT_REGS_RC(ctx);
    return handle_ret_udp_recvmsg_pre_4_7_0(copied, &udpv6_recv_sock);
}

#endif // COMPILE_CORE || COMPILE_PREBUILT

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

#if !defined(COMPILE_RUNTIME) || defined(FEATURE_IPV6_ENABLED)

static __always_inline void fl6_saddr(struct flowi6 *fl6, u64 *addr_h, u64 *addr_l) {
    if (!fl6 || !addr_h || !addr_l) {
        return;
    }

    struct in6_addr in6 = {};
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel_with_telemetry(&in6, sizeof(in6), ((char *)fl6) + offset_saddr_fl6());
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&in6, fl6, saddr);
#endif
    read_in6_addr(addr_h, addr_l, &in6);
}

static __always_inline void fl6_daddr(struct flowi6 *fl6, u64 *addr_h, u64 *addr_l) {
    if (!fl6 || !addr_h || !addr_l) {
        return;
    }

    struct in6_addr in6 = {};
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel_with_telemetry(&in6, sizeof(in6), ((char *)fl6) + offset_daddr_fl6());
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&in6, fl6, daddr);
#endif
    read_in6_addr(addr_h, addr_l, &in6);
}

static __always_inline u16 _fl6_sport(struct flowi6 *fl6) {
    u16 sport = 0;
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel_with_telemetry(&sport, sizeof(sport), ((char *)fl6) + offset_sport_fl6());
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&sport, fl6, fl6_sport);
#endif

    return sport;
}

static __always_inline u16 _fl6_dport(struct flowi6 *fl6) {
    u16 dport = 0;
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel_with_telemetry(&dport, sizeof(dport), ((char *)fl6) + offset_dport_fl6());
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&dport, fl6, fl6_dport);
#endif

    return dport;
}

static __always_inline int handle_ip6_skb(struct sock *sk, size_t size, struct flowi6 *fl6) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    size = size - sizeof(struct udphdr);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP)) {
#ifdef COMPILE_PREBUILT
        if (!are_fl6_offsets_known()) {
            log_debug("ERR: src/dst addr not set, fl6 offsets are not known\n");
            increment_telemetry_count(udp_send_missed);
            return 0;
        }
#endif
        fl6_saddr(fl6, &t.saddr_h, &t.saddr_l);
        if (!(t.saddr_h || t.saddr_l)) {
            log_debug("ERR(fl6): src addr not set src_l:%d,src_h:%d\n", t.saddr_l, t.saddr_h);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }

        fl6_daddr(fl6, &t.daddr_h, &t.daddr_l);
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

        t.sport = _fl6_sport(fl6);
        t.dport = _fl6_dport(fl6);

        if (t.sport == 0 || t.dport == 0) {
            log_debug("ERR(fl6): src/dst port not set: src:%d, dst:%d\n", t.sport, t.dport);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }

        t.sport = bpf_ntohs(t.sport);
        t.dport = bpf_ntohs(t.dport);
    }

    log_debug("kprobe/ip6_make_skb: pid_tgid: %d, size: %d\n", pid_tgid, size);
    handle_message(&t, size, 0, CONN_DIRECTION_UNKNOWN, 0, 0, PACKET_COUNT_NONE, sk);
    increment_telemetry_count(udp_send_processed);

    return 0;
}

SEC("kprobe/ip6_make_skb")
int kprobe__ip6_make_skb(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    size_t len = (size_t)PT_REGS_PARM4(ctx);
    // commit: https://github.com/torvalds/linux/commit/26879da58711aa604a1b866cbeedd7e0f78f90ad
    // changed the arguments to ip6_make_skb and introduced the struct ipcm6_cookie
#if !defined(COMPILE_RUNTIME) || LINUX_VERSION_CODE >= KERNEL_VERSION(4, 7, 0)
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

    return handle_ip6_skb(sk, size, fl6);
}

#endif // !COMPILE_RUNTIME || FEATURE_IVP6_ENABLED

#if defined(COMPILE_CORE) || defined(COMPILE_PREBUILT)
// commit: https://github.com/torvalds/linux/commit/26879da58711aa604a1b866cbeedd7e0f78f90ad
// changed the arguments to ip6_make_skb and introduced the struct ipcm6_cookie
SEC("kprobe/ip6_make_skb/pre_4_7_0")
int kprobe__ip6_make_skb__pre_4_7_0(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    size_t len = (size_t)PT_REGS_PARM4(ctx);
    struct flowi6 *fl6 = (struct flowi6 *)PT_REGS_PARM9(ctx);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    ip_make_skb_args_t args = {};
    bpf_probe_read_kernel_with_telemetry(&args.sk, sizeof(args.sk), &sk);
    bpf_probe_read_kernel_with_telemetry(&args.len, sizeof(args.len), &len);
    bpf_probe_read_kernel_with_telemetry(&args.fl6, sizeof(args.fl6), &fl6);
    bpf_map_update_with_telemetry(ip_make_skb_args, &pid_tgid, &args, BPF_ANY);
    return 0;
}

#endif // COMPILE_CORE || COMPILE_PREBUILT

static __always_inline u32 fl4_saddr(struct flowi4 *fl4) {
    u32 addr = 0;
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel_with_telemetry(&addr, sizeof(addr), ((char *)fl4) + offset_saddr_fl4());
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&addr, fl4, saddr);
#endif

    return addr;
}

static __always_inline u32 fl4_daddr(struct flowi4 *fl4) {
    u32 addr = 0;
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel_with_telemetry(&addr, sizeof(addr), ((char *)fl4) + offset_daddr_fl4());
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&addr, fl4, daddr);
#endif

    return addr;
}

static __always_inline u16 _fl4_sport(struct flowi4 *fl4) {
    u16 sport = 0;
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel_with_telemetry(&sport, sizeof(sport), ((char *)fl4) + offset_sport_fl4());
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&sport, fl4, fl4_sport);
#endif

    return sport;
}

static __always_inline u16 _fl4_dport(struct flowi4 *fl4) {
    u16 dport = 0;
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel_with_telemetry(&dport, sizeof(dport), ((char *)fl4) + offset_dport_fl4());
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&dport, fl4, fl4_dport);
#endif

    return dport;
}

static __always_inline int handle_ip_skb(struct sock *sk, size_t size, struct flowi4 *fl4) {
    size -= sizeof(struct udphdr);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP)) {
#ifdef COMPILE_PREBUILT
        if (!are_fl4_offsets_known()) {
            log_debug("ERR: src/dst addr not set src:%d,dst:%d. fl4 offsets are not known\n", t.saddr_l, t.daddr_l);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }
#endif

        t.saddr_l = fl4_saddr(fl4);
        t.daddr_l = fl4_daddr(fl4);

        if (!t.saddr_l || !t.daddr_l) {
            log_debug("ERR(fl4): src/dst addr not set src:%d,dst:%d\n", t.saddr_l, t.daddr_l);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }

        t.sport = _fl4_sport(fl4);
        t.dport = _fl4_dport(fl4);

        if (t.sport == 0 || t.dport == 0) {
            log_debug("ERR(fl4): src/dst port not set: src:%d, dst:%d\n", t.sport, t.dport);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }

        t.sport = bpf_ntohs(t.sport);
        t.dport = bpf_ntohs(t.dport);
    }

    log_debug("kprobe/ip_make_skb: pid_tgid: %d, size: %d\n", pid_tgid, size);

    // segment count is not currently enabled on prebuilt.
    // to enable, change PACKET_COUNT_NONE => PACKET_COUNT_INCREMENT
    handle_message(&t, size, 0, CONN_DIRECTION_UNKNOWN, 1, 0, PACKET_COUNT_NONE, sk);
    increment_telemetry_count(udp_send_processed);

    return 0;
}

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

    return handle_ip_skb(sk, size, fl4);
}

SEC("kprobe/udp_sendpage")
int kprobe__udp_sendpage(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/udp_sendpage: pid_tgid: %d\n", pid_tgid);
    struct sock *skp = (struct sock *)PT_REGS_PARM1(ctx);
    bpf_map_update_with_telemetry(udp_sendpage_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

SEC("kretprobe/udp_sendpage")
int kretprobe__udp_sendpage(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock **skpp = (struct sock **)bpf_map_lookup_elem(&udp_sendpage_args, &pid_tgid);
    if (!skpp) {
        log_debug("kretprobe/udp_sendpage: sock not found\n");
        return 0;
    }

    struct sock *skp = *skpp;
    bpf_map_delete_elem(&udp_sendpage_args, &pid_tgid);

    int sent = PT_REGS_RC(ctx);
    if (sent < 0) {
        return 0;
    }
    if (!skp) {
        return 0;
    }

    log_debug("kretprobe/udp_sendpage: pid_tgid: %d, sent: %d, sock: %x\n", pid_tgid, sent, skp);
    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, skp, pid_tgid, CONN_TYPE_UDP)) {
        return 0;
    }

    return handle_message(&t, sent, 0, CONN_DIRECTION_UNKNOWN, 0, 0, PACKET_COUNT_NONE, skp);
}

#endif // __TRACER_UDP_H
