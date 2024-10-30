#include "ktypes.h"
#ifndef COMPILE_CORE
#include "kconfig.h"
#endif
#include "bpf_telemetry.h"
#include "bpf_builtins.h"
#include "bpf_tracing.h"
#include "bpf_endian.h"
#include "bpf_metadata.h"
#include "bpf_bypass.h"

#ifdef COMPILE_PREBUILT
#include "prebuilt/offsets.h"
#endif
#include "skb.h"
#include "tracer/bind.h"
#include "tracer/events.h"
#include "tracer/maps.h"
#include "tracer/port.h"
#include "tracer/tcp_recv.h"
#include "protocols/classification/protocol-classification.h"

__maybe_unused static __always_inline bool tcp_failed_connections_enabled() {
    __u64 val = 0;
    LOAD_CONSTANT("tcp_failed_connections_enabled", val);
    return val > 0;
}

SEC("socket/classifier_entry")
int socket__classifier_entry(struct __sk_buff *skb) {
    protocol_classifier_entrypoint(skb);
    return 0;
}

SEC("socket/classifier_queues")
int socket__classifier_queues(struct __sk_buff *skb) {
    protocol_classifier_entrypoint_queues(skb);
    return 0;
}

SEC("socket/classifier_dbs")
int socket__classifier_dbs(struct __sk_buff *skb) {
    protocol_classifier_entrypoint_dbs(skb);
    return 0;
}

SEC("socket/classifier_grpc")
int socket__classifier_grpc(struct __sk_buff *skb) {
    protocol_classifier_entrypoint_grpc(skb);
    return 0;
}

SEC("kprobe/tcp_sendmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_sendmsg) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_sendmsg: pid_tgid: %llu", pid_tgid);
#if defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE < KERNEL_VERSION(4, 1, 0)
    struct sock *skp = (struct sock *)PT_REGS_PARM2(ctx);
#else
    struct sock *skp = (struct sock *)PT_REGS_PARM1(ctx);
#endif
    log_debug("kprobe/tcp_sendmsg: pid_tgid: %llu, sock: %p", pid_tgid, skp);
    bpf_map_update_with_telemetry(tcp_sendmsg_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

#if defined(COMPILE_CORE) || defined(COMPILE_PREBUILT)
SEC("kprobe/tcp_sendmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_sendmsg__pre_4_1_0) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_sendmsg: pid_tgid: %llu", pid_tgid);
    struct sock *skp = (struct sock *)PT_REGS_PARM2(ctx);
    bpf_map_update_with_telemetry(tcp_sendmsg_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}
#endif

SEC("kretprobe/tcp_sendmsg")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__tcp_sendmsg, int sent) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock **skpp = (struct sock **)bpf_map_lookup_elem(&tcp_sendmsg_args, &pid_tgid);
    if (!skpp) {
        log_debug("kretprobe/tcp_sendmsg: sock not found");
        return 0;
    }

    struct sock *skp = *skpp;
    bpf_map_delete_elem(&tcp_sendmsg_args, &pid_tgid);

    if (sent < 0) {
        return 0;
    }

    if (!skp) {
        return 0;
    }

    log_debug("kretprobe/tcp_sendmsg: pid_tgid: %llu, sent: %d, sock: %p", pid_tgid, sent, skp);
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

SEC("kprobe/tcp_sendpage")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_sendpage) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_sendpage: pid_tgid: %llu", pid_tgid);
    struct sock *skp = (struct sock *)PT_REGS_PARM1(ctx);
    bpf_map_update_with_telemetry(tcp_sendpage_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

SEC("kretprobe/tcp_sendpage")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__tcp_sendpage, int sent) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock **skpp = (struct sock **)bpf_map_lookup_elem(&tcp_sendpage_args, &pid_tgid);
    if (!skpp) {
        log_debug("kretprobe/tcp_sendpage: sock not found");
        return 0;
    }

    struct sock *skp = *skpp;
    bpf_map_delete_elem(&tcp_sendpage_args, &pid_tgid);

    if (sent < 0) {
        return 0;
    }

    if (!skp) {
        return 0;
    }

    log_debug("kretprobe/tcp_sendpage: pid_tgid: %llu, sent: %d, sock: %p", pid_tgid, sent, skp);
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

SEC("kprobe/udp_sendpage")
int BPF_BYPASSABLE_KPROBE(kprobe__udp_sendpage, struct sock *skp) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/udp_sendpage: pid_tgid: %llu", pid_tgid);
    bpf_map_update_with_telemetry(udp_sendpage_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

SEC("kretprobe/udp_sendpage")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__udp_sendpage, int sent) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock **skpp = (struct sock **)bpf_map_lookup_elem(&udp_sendpage_args, &pid_tgid);
    if (!skpp) {
        log_debug("kretprobe/udp_sendpage: sock not found");
        return 0;
    }

    struct sock *skp = *skpp;
    bpf_map_delete_elem(&udp_sendpage_args, &pid_tgid);

    if (sent < 0) {
        return 0;
    }
    if (!skp) {
        return 0;
    }

    log_debug("kretprobe/udp_sendpage: pid_tgid: %llu, sent: %d, sock: %p", pid_tgid, sent, skp);
    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, skp, pid_tgid, CONN_TYPE_UDP)) {
        return 0;
    }

    return handle_message(&t, sent, 0, CONN_DIRECTION_UNKNOWN, 1, 0, PACKET_COUNT_INCREMENT, skp);
}

SEC("kprobe/tcp_done")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_done, struct sock *sk) {
    conn_tuple_t t = {};

    if (!read_conn_tuple(&t, sk, 0, CONN_TYPE_TCP)) {
        increment_telemetry_count(tcp_done_failed_tuple);
        return 0;
    }
    log_debug("kprobe/tcp_done: netns: %u, sport: %u, dport: %u", t.netns, t.sport, t.dport);
    skp_conn_tuple_t skp_conn = {.sk = sk, .tup = t};

    if (!tcp_failed_connections_enabled()) {
        bpf_map_delete_elem(&tcp_ongoing_connect_pid, &skp_conn);
        return 0;
    }

    int err = 0;
    bpf_probe_read_kernel_with_telemetry(&err, sizeof(err), (&sk->sk_err));
    if (err == 0) {
        bpf_map_delete_elem(&tcp_ongoing_connect_pid, &skp_conn);
        return 0; // no failure
    }

    if (err != TCP_CONN_FAILED_RESET && err != TCP_CONN_FAILED_TIMEOUT && err != TCP_CONN_FAILED_REFUSED) {
        log_debug("kprobe/tcp_done: unsupported error code: %d", err);
        increment_telemetry_count(unsupported_tcp_failures);
        bpf_map_delete_elem(&tcp_ongoing_connect_pid, &skp_conn);
        return 0;
    }

    // connection timeouts will have 0 pids as they are cleaned up by an idle process. 
    // resets can also have kernel pids are they are triggered by receiving an RST packet from the server
    // get the pid from the ongoing failure map in this case, as it should have been set in connect(). else bail
    pid_ts_t *failed_conn_pid = bpf_map_lookup_elem(&tcp_ongoing_connect_pid, &skp_conn);
    if (failed_conn_pid) {
        bpf_map_delete_elem(&tcp_ongoing_connect_pid, &skp_conn);
        t.pid = failed_conn_pid->pid_tgid >> 32;
    } else {
        increment_telemetry_count(tcp_done_missing_pid);
        return 0;
    }

    // check if this connection was already flushed and ensure we don't flush again
    // upsert the timestamp to the map and delete if it already exists, flush connection otherwise
    // skip EEXIST errors for telemetry since it is an expected error
    __u64 timestamp = bpf_ktime_get_ns();
    if (bpf_map_update_with_telemetry(conn_close_flushed, &t, &timestamp, BPF_NOEXIST, -EEXIST) == 0) {
        cleanup_conn(ctx, &t, sk);
        increment_telemetry_count(tcp_done_connection_flush);
        flush_tcp_failure(ctx, &t, err);
    } else {
        bpf_map_delete_elem(&conn_close_flushed, &t);
        increment_telemetry_count(double_flush_attempts_done);
    }

    return 0;
}

SEC("kretprobe/tcp_done")
int BPF_KRETPROBE(kretprobe__tcp_done_flush) {
    flush_conn_close_if_full(ctx);
    return 0;
}

SEC("kprobe/tcp_close")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_close, struct sock *sk) {
    conn_tuple_t t = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();

    // Get network namespace id
    log_debug("kprobe/tcp_close: tgid: %llu, pid: %llu", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }
    log_debug("kprobe/tcp_close: netns: %u, sport: %u, dport: %u", t.netns, t.sport, t.dport);

    // If protocol classification is disabled, then we don't have kretprobe__tcp_close_clean_protocols hook
    // so, there is no one to use the map and clean it.
    if (is_protocol_classification_supported()) {
        bpf_map_update_with_telemetry(tcp_close_args, &pid_tgid, &t, BPF_ANY);
    }

    skp_conn_tuple_t skp_conn = {.sk = sk, .tup = t};
    skp_conn.tup.pid = 0;

    bpf_map_delete_elem(&tcp_ongoing_connect_pid, &skp_conn);

    if (!tcp_failed_connections_enabled()) {
        cleanup_conn(ctx, &t, sk);
        return 0;
    }

    // check if this connection was already flushed and ensure we don't flush again
    // upsert the timestamp to the map and delete if it already exists, flush connection otherwise
    // skip EEXIST errors for telemetry since it is an expected error
    __u64 timestamp = bpf_ktime_get_ns();
    if (bpf_map_update_with_telemetry(conn_close_flushed, &t, &timestamp, BPF_NOEXIST, -EEXIST) == 0) {
        cleanup_conn(ctx, &t, sk);
        increment_telemetry_count(tcp_close_connection_flush);
        int err = 0;
        bpf_probe_read_kernel_with_telemetry(&err, sizeof(err), (&sk->sk_err));
        if (err == TCP_CONN_FAILED_RESET || err == TCP_CONN_FAILED_TIMEOUT || err == TCP_CONN_FAILED_REFUSED) {
            increment_telemetry_count(tcp_close_target_failures);
            flush_tcp_failure(ctx, &t, err);
        }
    } else {
        bpf_map_delete_elem(&conn_close_flushed, &t);
        increment_telemetry_count(double_flush_attempts_close);
    }

    return 0;
}

SEC("kretprobe/tcp_close")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__tcp_close_clean_protocols) {
    u64 pid_tgid = bpf_get_current_pid_tgid();

    conn_tuple_t *tup_ptr = (conn_tuple_t *)bpf_map_lookup_elem(&tcp_close_args, &pid_tgid);
    if (tup_ptr) {
        clean_protocol_classification(tup_ptr);
        bpf_map_delete_elem(&tcp_close_args, &pid_tgid);
    }

    bpf_tail_call_compat(ctx, &tcp_close_progs, 0);

    return 0;
}

SEC("kretprobe/tcp_close")
int BPF_KRETPROBE(kretprobe__tcp_close_flush) {
    flush_conn_close_if_full(ctx);
    return 0;
}

#if !defined(COMPILE_RUNTIME) || defined(FEATURE_UDPV6_ENABLED)

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
    if (size <= sizeof(struct udphdr)) {
        return 0;
    }

    size -= sizeof(struct udphdr);
    u64 pid_tgid = bpf_get_current_pid_tgid();

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP)) {
#ifdef COMPILE_PREBUILT
        if (!are_fl6_offsets_known()) {
            log_debug("ERR: src/dst addr not set, fl6 offsets are not known");
            increment_telemetry_count(udp_send_missed);
            return 0;
        }
#endif
        fl6_saddr(fl6, &t.saddr_h, &t.saddr_l);
        if (!(t.saddr_h || t.saddr_l)) {
            log_debug("ERR(fl6): src addr not set src_l:%llu,src_h:%llu", t.saddr_l, t.saddr_h);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }

        fl6_daddr(fl6, &t.daddr_h, &t.daddr_l);
        if (!(t.daddr_h || t.daddr_l)) {
            log_debug("ERR(fl6): dst addr not set dst_l:%llu,dst_h:%llu", t.daddr_l, t.daddr_h);
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
            log_debug("ERR(fl6): src/dst port not set: src:%d, dst:%d", t.sport, t.dport);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }

        t.sport = bpf_ntohs(t.sport);
        t.dport = bpf_ntohs(t.dport);
    }

    log_debug("kprobe/ip6_make_skb: pid_tgid: %llu, size: %zu", pid_tgid, size);
    handle_message(&t, size, 0, CONN_DIRECTION_UNKNOWN, 1, 0, PACKET_COUNT_INCREMENT, sk);
    increment_telemetry_count(udp_send_processed);

    return 0;
}

#if defined(COMPILE_CORE) || defined(COMPILE_PREBUILT)
// commit: https://github.com/torvalds/linux/commit/26879da58711aa604a1b866cbeedd7e0f78f90ad
// changed the arguments to ip6_make_skb and introduced the struct ipcm6_cookie
SEC("kprobe/ip6_make_skb")
int BPF_BYPASSABLE_KPROBE(kprobe__ip6_make_skb__pre_4_7_0, struct sock *sk) {
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

SEC("kprobe/ip6_make_skb")
int BPF_BYPASSABLE_KPROBE(kprobe__ip6_make_skb__pre_5_18_0, struct sock *sk) {
    size_t len = (size_t)PT_REGS_PARM4(ctx);
    struct flowi6 *fl6 = (struct flowi6 *)PT_REGS_PARM7(ctx);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    ip_make_skb_args_t args = {};
    bpf_probe_read_kernel_with_telemetry(&args.sk, sizeof(args.sk), &sk);
    bpf_probe_read_kernel_with_telemetry(&args.len, sizeof(args.len), &len);
    bpf_probe_read_kernel_with_telemetry(&args.fl6, sizeof(args.fl6), &fl6);
    bpf_map_update_with_telemetry(ip_make_skb_args, &pid_tgid, &args, BPF_ANY);
    return 0;
}

#endif // COMPILE_CORE || COMPILE_PREBUILT

#if defined(COMPILE_RUNTIME) || defined(COMPILE_CORE)

SEC("kprobe/ip6_make_skb")
int BPF_BYPASSABLE_KPROBE(kprobe__ip6_make_skb, struct sock *sk) {
    size_t len = (size_t)PT_REGS_PARM4(ctx);
#if defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE >= KERNEL_VERSION(5, 18, 0)
    // commit: https://github.com/torvalds/linux/commit/f37a4cc6bb0ba08c2d9fd7d18a1da87161cbb7f9
    struct inet_cork_full *cork_full = (struct inet_cork_full *)PT_REGS_PARM9(ctx);
    struct flowi6 *fl6 = &cork_full->fl.u.ip6;
#elif defined(COMPILE_CORE)
    struct inet_cork_full *cork_full = (struct inet_cork_full *)PT_REGS_PARM9(ctx);
    struct flowi6 *fl6 = (struct flowi6 *)__builtin_preserve_access_index(&cork_full->fl.u.ip6);
#elif LINUX_VERSION_CODE >= KERNEL_VERSION(4, 7, 0)
    // commit: https://github.com/torvalds/linux/commit/26879da58711aa604a1b866cbeedd7e0f78f90ad
    // changed the arguments to ip6_make_skb and introduced the struct ipcm6_cookie
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

#endif // COMPILE_RUNTIME || COMPILE_CORE

SEC("kretprobe/ip6_make_skb")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__ip6_make_skb, void *rc) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    ip_make_skb_args_t *args = bpf_map_lookup_elem(&ip_make_skb_args, &pid_tgid);
    if (!args) {
        return 0;
    }

    struct sock *sk = args->sk;
    struct flowi6 *fl6 = args->fl6;
    size_t size = args->len;
    bpf_map_delete_elem(&ip_make_skb_args, &pid_tgid);

    if (IS_ERR_OR_NULL(rc)) {
        return 0;
    }

    return handle_ip6_skb(sk, size, fl6);
}

#endif // !COMPILE_RUNTIME || FEATURE_UDPV6_ENABLED

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
    if (size <= sizeof(struct udphdr)) {
        return 0;
    }

    size -= sizeof(struct udphdr);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP)) {
#ifdef COMPILE_PREBUILT
        if (!are_fl4_offsets_known()) {
            log_debug("ERR: src/dst addr not set src:%llu,dst:%llu. fl4 offsets are not known", t.saddr_l, t.daddr_l);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }
#endif

        t.saddr_l = fl4_saddr(fl4);
        t.daddr_l = fl4_daddr(fl4);

        if (!t.saddr_l || !t.daddr_l) {
            log_debug("ERR(fl4): src/dst addr not set src:%llu,dst:%llu", t.saddr_l, t.daddr_l);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }

        t.sport = _fl4_sport(fl4);
        t.dport = _fl4_dport(fl4);

        if (t.sport == 0 || t.dport == 0) {
            log_debug("ERR(fl4): src/dst port not set: src:%d, dst:%d", t.sport, t.dport);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }

        t.sport = bpf_ntohs(t.sport);
        t.dport = bpf_ntohs(t.dport);
    }

    log_debug("kprobe/ip_make_skb: pid_tgid: %llu, size: %zu", pid_tgid, size);

    handle_message(&t, size, 0, CONN_DIRECTION_UNKNOWN, 1, 0, PACKET_COUNT_INCREMENT, sk);
    increment_telemetry_count(udp_send_processed);

    return 0;
}

__maybe_unused static __always_inline bool udp_send_page_enabled() {
    __u64 val = 0;
    LOAD_CONSTANT("udp_send_page_enabled", val);
    return val > 0;
}

// Note: This is used only in the UDP send path.
SEC("kprobe/ip_make_skb")
int BPF_BYPASSABLE_KPROBE(kprobe__ip_make_skb, struct sock *sk) {
    size_t len = (size_t)PT_REGS_PARM5(ctx);
    struct flowi4 *fl4 = (struct flowi4 *)PT_REGS_PARM2(ctx);
#if defined(COMPILE_PREBUILT) || defined(COMPILE_CORE) || (defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE >= KERNEL_VERSION(4, 18, 0))
    unsigned int flags = PT_REGS_PARM10(ctx);
    if (flags & MSG_SPLICE_PAGES && udp_send_page_enabled()) {
        return 0;
    }
#endif

    u64 pid_tgid = bpf_get_current_pid_tgid();
    ip_make_skb_args_t args = {};
    bpf_probe_read_kernel_with_telemetry(&args.sk, sizeof(args.sk), &sk);
    bpf_probe_read_kernel_with_telemetry(&args.len, sizeof(args.len), &len);
    bpf_probe_read_kernel_with_telemetry(&args.fl4, sizeof(args.fl4), &fl4);
    bpf_map_update_with_telemetry(ip_make_skb_args, &pid_tgid, &args, BPF_ANY);

    return 0;
}

SEC("kprobe/ip_make_skb")
int BPF_BYPASSABLE_KPROBE(kprobe__ip_make_skb__pre_4_18_0, struct sock *sk) {
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
int BPF_BYPASSABLE_KRETPROBE(kretprobe__ip_make_skb, void *rc) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    ip_make_skb_args_t *args = bpf_map_lookup_elem(&ip_make_skb_args, &pid_tgid);
    if (!args) {
        return 0;
    }

    struct sock *sk = args->sk;
    struct flowi4 *fl4 = args->fl4;
    size_t size = args->len;
    bpf_map_delete_elem(&ip_make_skb_args, &pid_tgid);

    if (IS_ERR_OR_NULL(rc)) {
        return 0;
    }

    return handle_ip_skb(sk, size, fl4);
}

#define handle_udp_recvmsg(sk, msg, flags, udp_sock_map)                                                 \
    do {                                                                                                 \
        log_debug("kprobe/udp_recvmsg: flags: %x", flags);                                               \
        if (flags & MSG_PEEK) {                                                                          \
            return 0;                                                                                    \
        }                                                                                                \
                                                                                                         \
        /* keep track of non-peeking calls, since skb_free_datagram_locked doesn't have that argument */ \
        u64 pid_tgid = bpf_get_current_pid_tgid();                                                       \
        udp_recv_sock_t t = { .sk = sk, .msg = msg };                                                    \
        bpf_map_update_with_telemetry(udp_sock_map, &pid_tgid, &t, BPF_ANY);                             \
        return 0;                                                                                        \
    } while (0);

SEC("kprobe/udp_recvmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__udp_recvmsg) {
#if defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE < KERNEL_VERSION(4, 1, 0)
    int flags = (int)PT_REGS_PARM6(ctx);
#elif defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE < KERNEL_VERSION(5, 19, 0)
    int flags = (int)PT_REGS_PARM5(ctx);
#else
    int flags = (int)PT_REGS_PARM4(ctx);
#endif
    struct sock *sk = NULL;
    struct msghdr *msg = NULL;
    handle_udp_recvmsg(sk, msg, flags, udp_recv_sock);
}

#if !defined(COMPILE_RUNTIME) || defined(FEATURE_UDPV6_ENABLED)
SEC("kprobe/udpv6_recvmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__udpv6_recvmsg) {
#if defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE < KERNEL_VERSION(4, 1, 0)
    int flags = (int)PT_REGS_PARM6(ctx);
#elif defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE < KERNEL_VERSION(5, 19, 0)
    int flags = (int)PT_REGS_PARM5(ctx);
#else
    int flags = (int)PT_REGS_PARM4(ctx);
#endif
    struct sock *sk = NULL;
    struct msghdr *msg = NULL;
    handle_udp_recvmsg(sk, msg, flags, udp_recv_sock);
}
#endif // !COMPILE_RUNTIME || defined(FEATURE_UDPV6_ENABLED)

static __always_inline int handle_udp_recvmsg_ret() {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    bpf_map_delete_elem(&udp_recv_sock, &pid_tgid);
    return 0;
}

SEC("kretprobe/udp_recvmsg")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__udp_recvmsg) {
    return handle_udp_recvmsg_ret();
}

#if !defined(COMPILE_RUNTIME) || defined(FEATURE_UDPV6_ENABLED)
SEC("kretprobe/udpv6_recvmsg")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__udpv6_recvmsg) {
    return handle_udp_recvmsg_ret();
}
#endif // !COMPILE_RUNTIME || defined(FEATURE_UDPV6_ENABLED)

#if defined(COMPILE_CORE) || defined(COMPILE_PREBUILT)

static __always_inline int handle_ret_udp_recvmsg_pre_4_7_0(int copied, void *udp_sock_map) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kretprobe/udp_recvmsg: tgid: %llu, pid: %llu", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

    // Retrieve socket pointer from kprobe via pid/tgid
    udp_recv_sock_t *st = bpf_map_lookup_elem(udp_sock_map, &pid_tgid);
    if (!st) { // Missed entry
        return 0;
    }

    if (copied < 0) { // Non-zero values are errors (or a peek) (e.g -EINVAL)
        log_debug("kretprobe/udp_recvmsg: ret=%d < 0, pid_tgid=%llu", copied, pid_tgid);
        // Make sure we clean up the key
        bpf_map_delete_elem(udp_sock_map, &pid_tgid);
        return 0;
    }

    log_debug("kretprobe/udp_recvmsg: ret=%d", copied);

    conn_tuple_t t = {};
    bpf_memset(&t, 0, sizeof(conn_tuple_t));
    if (st->msg) {
        struct sockaddr *sap = NULL;
        bpf_probe_read_kernel_with_telemetry(&sap, sizeof(sap), &(st->msg->msg_name));
        sockaddr_to_addr(sap, &t.daddr_h, &t.daddr_l, &t.dport, &t.metadata);
    }

    if (!read_conn_tuple_partial(&t, st->sk, pid_tgid, CONN_TYPE_UDP)) {
        log_debug("ERR(kretprobe/udp_recvmsg): error reading conn tuple, pid_tgid=%llu", pid_tgid);
        bpf_map_delete_elem(udp_sock_map, &pid_tgid);
        return 0;
    }
    bpf_map_delete_elem(udp_sock_map, &pid_tgid);

    log_debug("kretprobe/udp_recvmsg: pid_tgid: %llu, return: %d", pid_tgid, copied);
    handle_message(&t, 0, copied, CONN_DIRECTION_UNKNOWN, 0, 1, PACKET_COUNT_INCREMENT, st->sk);

    return 0;
}

SEC("kprobe/udp_recvmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__udp_recvmsg_pre_5_19_0) {
    struct sock *sk = NULL;
    struct msghdr *msg = NULL;
    int flags = (int)PT_REGS_PARM5(ctx);
    handle_udp_recvmsg(sk, msg, flags, udp_recv_sock);
}

SEC("kprobe/udpv6_recvmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__udpv6_recvmsg_pre_5_19_0) {
    struct sock *sk = NULL;
    struct msghdr *msg = NULL;
    int flags = (int)PT_REGS_PARM5(ctx);
    handle_udp_recvmsg(sk, msg, flags, udp_recv_sock);
}

SEC("kprobe/udp_recvmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__udp_recvmsg_pre_4_7_0, struct sock *sk, struct msghdr *msg) {
    int flags = (int)PT_REGS_PARM5(ctx);
    handle_udp_recvmsg(sk, msg, flags, udp_recv_sock);
}

SEC("kprobe/udpv6_recvmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__udpv6_recvmsg_pre_4_7_0, struct sock *sk, struct msghdr *msg) {
    int flags = (int)PT_REGS_PARM5(ctx);
#ifdef COMPILE_CORE
    // on CO-RE we use only use the map to check if the
    // receive was a peek, since we the use the kprobes
    // on `skb_consume_udp` (and alternatives). These
    // kprobes explicitly check the `udp_recv_sock` map
    handle_udp_recvmsg(sk, msg, flags, udp_recv_sock);
#else
    handle_udp_recvmsg(sk, msg, flags, udpv6_recv_sock);
#endif
}

SEC("kprobe/udp_recvmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__udp_recvmsg_pre_4_1_0) {
    struct sock *sk = (struct sock *)PT_REGS_PARM2(ctx);
    struct msghdr *msg = (struct msghdr *)PT_REGS_PARM3(ctx);
    int flags = (int)PT_REGS_PARM6(ctx);
    handle_udp_recvmsg(sk, msg, flags, udp_recv_sock);
}

SEC("kprobe/udpv6_recvmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__udpv6_recvmsg_pre_4_1_0) {
    struct sock *sk = (struct sock *)PT_REGS_PARM2(ctx);
    struct msghdr *msg = (struct msghdr *)PT_REGS_PARM3(ctx);
    int flags = (int)PT_REGS_PARM6(ctx);
#ifdef COMPILE_CORE
    // on CO-RE we use only use the map to check if the
    // receive was a peek, since we the use the kprobes
    // on `skb_consume_udp` (and alternatives). These
    // kprobes explicitly check the `udp_recv_sock` map
    handle_udp_recvmsg(sk, msg, flags, udp_recv_sock);
#else
    handle_udp_recvmsg(sk, msg, flags, udpv6_recv_sock);
#endif
}

SEC("kretprobe/udp_recvmsg")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__udp_recvmsg_pre_4_7_0, int copied) {
    return handle_ret_udp_recvmsg_pre_4_7_0(copied, &udp_recv_sock);
}

SEC("kretprobe/udpv6_recvmsg")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__udpv6_recvmsg_pre_4_7_0, int copied) {
    return handle_ret_udp_recvmsg_pre_4_7_0(copied, &udpv6_recv_sock);
}

#endif // COMPILE_CORE || COMPILE_PREBUILT

SEC("kprobe/skb_free_datagram_locked")
int BPF_BYPASSABLE_KPROBE(kprobe__skb_free_datagram_locked, struct sock *sk, struct sk_buff *skb) {
    return handle_skb_consume_udp(sk, skb, 0);
}

SEC("kprobe/__skb_free_datagram_locked")
int BPF_BYPASSABLE_KPROBE(kprobe____skb_free_datagram_locked, struct sock *sk, struct sk_buff *skb, int len) {
    return handle_skb_consume_udp(sk, skb, len);
}

SEC("kprobe/skb_consume_udp")
int BPF_BYPASSABLE_KPROBE(kprobe__skb_consume_udp, struct sock *sk, struct sk_buff *skb, int len) {
    return handle_skb_consume_udp(sk, skb, len);
}

#ifdef COMPILE_PREBUILT

SEC("kprobe/tcp_retransmit_skb")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_retransmit_skb, struct sock *sk) {
    int segs = (int)PT_REGS_PARM3(ctx);
    log_debug("kprobe/tcp_retransmit: segs: %d", segs);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    tcp_retransmit_skb_args_t args = {};
    args.sk = sk;
    args.segs = segs;
    bpf_map_update_with_telemetry(pending_tcp_retransmit_skb, &pid_tgid, &args, BPF_ANY);
    return 0;
}

SEC("kprobe/tcp_retransmit_skb")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_retransmit_skb_pre_4_7_0, struct sock *sk) {
    log_debug("kprobe/tcp_retransmit");
    u64 pid_tgid = bpf_get_current_pid_tgid();
    tcp_retransmit_skb_args_t args = {};
    args.sk = sk;
    args.segs = 1;
    bpf_map_update_with_telemetry(pending_tcp_retransmit_skb, &pid_tgid, &args, BPF_ANY);
    return 0;
}

SEC("kretprobe/tcp_retransmit_skb")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__tcp_retransmit_skb, int ret) {
    __u64 tid = bpf_get_current_pid_tgid();
    if (ret < 0) {
        bpf_map_delete_elem(&pending_tcp_retransmit_skb, &tid);
        return 0;
    }
    tcp_retransmit_skb_args_t *args = bpf_map_lookup_elem(&pending_tcp_retransmit_skb, &tid);
    if (args == NULL) {
        return 0;
    }
    struct sock *sk = args->sk;
    int segs = args->segs;
    bpf_map_delete_elem(&pending_tcp_retransmit_skb, &tid);
    log_debug("kretprobe/tcp_retransmit: segs: %d", segs);
    return handle_retransmit(sk, segs);
}

#endif // COMPILE_PREBUILT

#if defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)

SEC("kprobe/tcp_retransmit_skb")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_retransmit_skb, struct sock *sk) {
    u64 tid = bpf_get_current_pid_tgid();
    tcp_retransmit_skb_args_t args = {};
    args.sk = sk;
    args.segs = 0;
    BPF_CORE_READ_INTO(&args.retrans_out_pre, tcp_sk(sk), retrans_out);
    bpf_map_update_with_telemetry(pending_tcp_retransmit_skb, &tid, &args, BPF_ANY);
    return 0;
}

SEC("kretprobe/tcp_retransmit_skb")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__tcp_retransmit_skb, int rc) {
    log_debug("kretprobe/tcp_retransmit");
    u64 tid = bpf_get_current_pid_tgid();
    if (rc < 0) {
        bpf_map_delete_elem(&pending_tcp_retransmit_skb, &tid);
        return 0;
    }
    tcp_retransmit_skb_args_t *args = bpf_map_lookup_elem(&pending_tcp_retransmit_skb, &tid);
    if (args == NULL) {
        return 0;
    }
    struct sock *sk = args->sk;
    u32 retrans_out_pre = args->retrans_out_pre;
    bpf_map_delete_elem(&pending_tcp_retransmit_skb, &tid);
    u32 retrans_out = 0;
    BPF_CORE_READ_INTO(&retrans_out, tcp_sk(sk), retrans_out);
    return handle_retransmit(sk, retrans_out - retrans_out_pre);
}

#endif // COMPILE_CORE || COMPILE_RUNTIME

SEC("kprobe/tcp_connect")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_connect, struct sock *skp) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_connect: tgid: %llu, pid: %llu", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, skp, 0, CONN_TYPE_TCP)) {
        increment_telemetry_count(tcp_connect_failed_tuple);
        return 0;
    }

    skp_conn_tuple_t skp_conn = {.sk = skp, .tup = t};
    pid_ts_t pid_ts = {.pid_tgid = pid_tgid, .timestamp = bpf_ktime_get_ns()};
    bpf_map_update_with_telemetry(tcp_ongoing_connect_pid, &skp_conn, &pid_ts, BPF_ANY);

    return 0;
}

SEC("kprobe/tcp_finish_connect")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_finish_connect, struct sock *skp) {
    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, skp, 0, CONN_TYPE_TCP)) {
        increment_telemetry_count(tcp_finish_connect_failed_tuple);
        return 0;
    }
    skp_conn_tuple_t skp_conn = {.sk = skp, .tup = t};
    pid_ts_t *pid_tgid_p = bpf_map_lookup_elem(&tcp_ongoing_connect_pid, &skp_conn);
    if (!pid_tgid_p) {
        return 0;
    }

    u64 pid_tgid = pid_tgid_p->pid_tgid;
    t.pid = pid_tgid >> 32;
    log_debug("kprobe/tcp_finish_connect: tgid: %llu, pid: %llu", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

    handle_tcp_stats(&t, skp, TCP_ESTABLISHED);
    handle_message(&t, 0, 0, CONN_DIRECTION_OUTGOING, 0, 0, PACKET_COUNT_NONE, skp);

    log_debug("kprobe/tcp_finish_connect: netns: %u, sport: %u, dport: %u", t.netns, t.sport, t.dport);

    return 0;
}

SEC("kretprobe/inet_csk_accept")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__inet_csk_accept, struct sock *sk) {
    if (!sk) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kretprobe/inet_csk_accept: tgid: %llu, pid: %llu", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }
    log_debug("kretprobe/inet_csk_accept: netns: %u, sport: %u, dport: %u", t.netns, t.sport, t.dport);

    handle_tcp_stats(&t, sk, TCP_ESTABLISHED);
    handle_message(&t, 0, 0, CONN_DIRECTION_INCOMING, 0, 0, PACKET_COUNT_NONE, sk);

    port_binding_t pb = {};
    pb.netns = t.netns;
    pb.port = t.sport;
    add_port_bind(&pb, port_bindings);

    skp_conn_tuple_t skp_conn = {.sk = sk, .tup = t};
    skp_conn.tup.pid = 0;
    pid_ts_t pid_ts = {.pid_tgid = pid_tgid, .timestamp = bpf_ktime_get_ns()};
    bpf_map_update_with_telemetry(tcp_ongoing_connect_pid, &skp_conn, &pid_ts, BPF_ANY);

    return 0;
}

SEC("kprobe/inet_csk_listen_stop")
int BPF_BYPASSABLE_KPROBE(kprobe__inet_csk_listen_stop, struct sock *skp) {
    __u16 lport = read_sport(skp);
    if (lport == 0) {
        log_debug("ERR(inet_csk_listen_stop): lport is 0 ");
        return 0;
    }

    port_binding_t pb = { .netns = 0, .port = 0 };
    pb.netns = get_netns_from_sock(skp);
    pb.port = lport;
    remove_port_bind(&pb, &port_bindings);

    log_debug("kprobe/inet_csk_listen_stop: net ns: %u, lport: %u", pb.netns, pb.port);
    return 0;
}

static __always_inline int handle_udp_destroy_sock(void *ctx, struct sock *skp) {
    conn_tuple_t tup = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();
    int valid_tuple = read_conn_tuple(&tup, skp, pid_tgid, CONN_TYPE_UDP);

    __u16 lport = 0;
    if (valid_tuple) {
        cleanup_conn(ctx, &tup, skp);
        lport = tup.sport;
    } else {
        lport = read_sport(skp);
    }

    if (lport == 0) {
        log_debug("ERR(udp_destroy_sock): lport is 0");
        return 0;
    }

    port_binding_t pb = {};
    pb.netns = get_netns_from_sock(skp);
    pb.port = lport;
    remove_port_bind(&pb, &udp_port_bindings);
    return 0;
}

SEC("kprobe/udp_destroy_sock")
int BPF_BYPASSABLE_KPROBE(kprobe__udp_destroy_sock, struct sock *sk) {
    return handle_udp_destroy_sock(ctx, sk);
}

SEC("kprobe/udpv6_destroy_sock")
int BPF_BYPASSABLE_KPROBE(kprobe__udpv6_destroy_sock, struct sock *sk) {
    return handle_udp_destroy_sock(ctx, sk);
}

SEC("kretprobe/udp_destroy_sock")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__udp_destroy_sock) {
    flush_conn_close_if_full(ctx);
    return 0;
}

SEC("kretprobe/udpv6_destroy_sock")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__udpv6_destroy_sock) {
    flush_conn_close_if_full(ctx);
    return 0;
}

SEC("kprobe/inet_bind")
int BPF_BYPASSABLE_KPROBE(kprobe__inet_bind, struct socket *sock, struct sockaddr *addr) {
    log_debug("kprobe/inet_bind: sock=%p, umyaddr=%p", sock, addr);
    return sys_enter_bind(sock, addr);
}

SEC("kprobe/inet6_bind")
int BPF_BYPASSABLE_KPROBE(kprobe__inet6_bind, struct socket *sock, struct sockaddr *addr) {
    log_debug("kprobe/inet6_bind: sock=%p, umyaddr=%p", sock, addr);
    return sys_enter_bind(sock, addr);
}

SEC("kretprobe/inet_bind")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__inet_bind, __s64 ret) {
    log_debug("kretprobe/inet_bind: ret=%lld", ret);
    return sys_exit_bind(ret);
}

SEC("kretprobe/inet6_bind")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__inet6_bind, __s64 ret) {
    log_debug("kretprobe/inet6_bind: ret=%lld", ret);
    return sys_exit_bind(ret);
}

// Represents the parameters being passed to the tracepoint net/net_dev_queue
struct net_dev_queue_ctx {
    u64 unused;
    struct sk_buff *skb;
};

static __always_inline struct sock *sk_buff_sk(struct sk_buff *skb) {
    struct sock *sk = NULL;
#ifdef COMPILE_PREBUILT
    bpf_probe_read(&sk, sizeof(struct sock *), (char *)skb + offset_sk_buff_sock());
#elif defined(COMPILE_CORE) || defined(COMPILE_RUNTIME)
    BPF_CORE_READ_INTO(&sk, skb, sk);
#endif

    return sk;
}

SEC("tracepoint/net/net_dev_queue")
int tracepoint__net__net_dev_queue(struct net_dev_queue_ctx *ctx) {
    CHECK_BPF_PROGRAM_BYPASSED()
    struct sk_buff *skb = ctx->skb;
    if (!skb) {
        return 0;
    }
    struct sock *sk = sk_buff_sk(skb);
    if (!sk) {
        return 0;
    }

    conn_tuple_t skb_tup;
    bpf_memset(&skb_tup, 0, sizeof(conn_tuple_t));
    if (sk_buff_to_tuple(skb, &skb_tup) <= 0) {
        return 0;
    }

    if (!(skb_tup.metadata & CONN_TYPE_TCP)) {
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
        normalize_tuple(&skb_tup);
        normalize_tuple(&sock_tup);
        // We skip EEXIST because of the use of BPF_NOEXIST flag. Emitting telemetry for EEXIST here spams metrics
        // and do not provide any useful signal since the key is expected to be present sometimes.
        bpf_map_update_with_telemetry(conn_tuple_to_socket_skb_conn_tuple, &sock_tup, &skb_tup, BPF_NOEXIST, -EEXIST);
    }

    return 0;
}

char _license[] SEC("license") = "GPL";
