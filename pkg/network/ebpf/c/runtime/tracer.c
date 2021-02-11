#include "tracer.h"

#include "tracer-events.h"
#include "tracer-maps.h"
#include "tracer-telemetry.h"
#include "bpf_helpers.h"
#include "bpf_endian.h"
#include "syscalls.h"
#include "http.h"
#include "ip.h"

#ifdef FEATURE_IPV6_ENABLED
#include "ipv6.h"
#endif

#include <linux/kconfig.h>
#include <linux/version.h>
#include <net/inet_sock.h>
#include <net/net_namespace.h>
#include <net/tcp_states.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/ipv6.h>
#include <uapi/linux/ptrace.h>
#include <linux/tcp.h>
#include <uapi/linux/udp.h>

#ifndef LINUX_VERSION_CODE
# error "kernel version not included?"
#endif

static __always_inline __u32 get_netns_from_sock(struct sock* skp) {
    __u32 net_ns_inum = 0;
#ifdef CONFIG_NET_NS
    #if LINUX_VERSION_CODE < KERNEL_VERSION(4, 1, 0)
        struct net *skc_net = NULL;
        bpf_probe_read(&skc_net, sizeof(skc_net), &skp->sk_net);
        if (!skc_net) {
            return 0;
        }
        #if LINUX_VERSION_CODE < KERNEL_VERSION(3, 19, 0)
            bpf_probe_read(&net_ns_inum, sizeof(net_ns_inum), &skc_net->proc_inum);
        #else
            bpf_probe_read(&net_ns_inum, sizeof(net_ns_inum), &skc_net->ns.inum);
        #endif
    #else
        struct net *skc_net = NULL;
        bpf_probe_read(&skc_net, sizeof(skc_net), &skp->sk_net.net);
        if (!skc_net) {
            return 0;
        }
        bpf_probe_read(&net_ns_inum, sizeof(net_ns_inum), &skc_net->ns.inum);
    #endif
#endif
    return net_ns_inum;
}

static __always_inline __u16 read_sport(struct sock* skp) {
    __u16 sport = 0;
    bpf_probe_read(&sport, sizeof(sport), &skp->sk_num);
    if (sport == 0) {
        bpf_probe_read(&sport, sizeof(sport), &inet_sk(skp)->inet_sport);
        sport = bpf_ntohs(sport);
    }
    return sport;
}

/**
 * Reads values into a `conn_tuple_t` from a `sock`. Any values that are already set in conn_tuple_t
 * are not overwritten. Returns 1 success, 0 otherwise.
 */
static __always_inline int read_conn_tuple_partial(conn_tuple_t* t, struct sock* skp, u64 pid_tgid, metadata_mask_t type) {
    t->pid = pid_tgid >> 32;
    t->metadata = type;

    // Retrieve network namespace id first since addresses and ports may not be available for unconnected UDP
    // sends
    t->netns = get_netns_from_sock(skp);
    u16 family = 0;
    bpf_probe_read(&family, sizeof(family), &skp->sk_family);

    // Retrieve addresses
    if (family == AF_INET) {
        t->metadata |= CONN_V4;
        if (t->saddr_l == 0) bpf_probe_read(&t->saddr_l, sizeof(__be32), &skp->sk_rcv_saddr);
        if (t->daddr_l == 0) bpf_probe_read(&t->daddr_l, sizeof(__be32), &skp->sk_daddr);

        if (!t->saddr_l || !t->daddr_l) {
            log_debug("ERR(read_conn_tuple.v4): src/dst addr not set src:%d,dst:%d\n", t->saddr_l, t->daddr_l);
            return 0;
        }
    }
#ifdef FEATURE_IPV6_ENABLED
    else if (family == AF_INET6) {
        // TODO cleanup? having it split on 64 bits is not nice for kernel reads
        __be32 v6src[4] = {};
        __be32 v6dst[4] = {};
        bpf_probe_read(&v6src, sizeof(v6src), skp->sk_v6_rcv_saddr.in6_u.u6_addr32);
        bpf_probe_read(&v6dst, sizeof(v6dst), skp->sk_v6_daddr.in6_u.u6_addr32);

        if (t->saddr_h == 0) bpf_probe_read(&t->saddr_h, sizeof(t->saddr_h), v6src);
        if (t->saddr_l == 0) bpf_probe_read(&t->saddr_l, sizeof(t->saddr_l), v6src + 2);
        if (t->daddr_h == 0) bpf_probe_read(&t->daddr_h, sizeof(t->daddr_h), v6dst);
        if (t->daddr_l == 0) bpf_probe_read(&t->daddr_l, sizeof(t->daddr_l), v6dst + 2);

        // We can only pass 4 args to bpf_trace_printk
        // so split those 2 statements to be able to log everything
        if (!(t->saddr_h || t->saddr_l)) {
            log_debug("ERR(read_conn_tuple.v6): src addr not set: src_l:%d,src_h:%d\n",
                t->saddr_l, t->saddr_h);
            return 0;
        }

        if (!(t->daddr_h || t->daddr_l)) {
            log_debug("ERR(read_conn_tuple.v6): dst addr not set: dst_l:%d,dst_h:%d\n",
                t->daddr_l, t->daddr_h);
            return 0;
        }

        // Check if we can map IPv6 to IPv4
        if (is_ipv4_mapped_ipv6(t->saddr_h, t->saddr_l, t->daddr_h, t->daddr_l)) {
            t->metadata |= CONN_V4;
            t->saddr_h = 0;
            t->daddr_h = 0;
            t->saddr_l = (__u32)(t->saddr_l >> 32);
            t->daddr_l = (__u32)(t->daddr_l >> 32);
        } else {
            t->metadata |= CONN_V6;
        }
    }
#endif

    // Retrieve ports
    if (t->sport == 0) t->sport = read_sport(skp);
    if (t->dport == 0) {
        bpf_probe_read(&t->dport, sizeof(t->dport), &skp->sk_dport);
        t->dport = bpf_ntohs(t->dport);
    }

    if (t->sport == 0 || t->dport == 0) {
        log_debug("ERR(read_conn_tuple.v4): src/dst port not set: src:%d, dst:%d\n", t->sport, t->dport);
        return 0;
    }

    return 1;
}

/**
 * Reads values into a `conn_tuple_t` from a `sock`. Initializes all values in conn_tuple_t to `0`. Returns 1 success, 0 otherwise.
 */
static __always_inline int read_conn_tuple(conn_tuple_t* t, struct sock* skp, u64 pid_tgid, metadata_mask_t type) {
    __builtin_memset(t, 0, sizeof(conn_tuple_t));
    return read_conn_tuple_partial(t, skp, pid_tgid, type);
}

static __always_inline void update_conn_stats(conn_tuple_t* t, size_t sent_bytes, size_t recv_bytes, u64 ts) {
    conn_stats_ts_t* val;

    // initialize-if-no-exist the connection stat, and load it
    conn_stats_ts_t empty = {};
    __builtin_memset(&empty, 0, sizeof(conn_stats_ts_t));
    bpf_map_update_elem(&conn_stats, t, &empty, BPF_NOEXIST);
    val = bpf_map_lookup_elem(&conn_stats, t);

    // If already in our map, increment size in-place
    if (val != NULL) {
        if (sent_bytes) {
            __sync_fetch_and_add(&val->sent_bytes, sent_bytes);
        }
        if (recv_bytes) {
            __sync_fetch_and_add(&val->recv_bytes, recv_bytes);
        }
        val->timestamp = ts;
    }

}

static __always_inline void update_tcp_stats(conn_tuple_t* t, tcp_stats_t stats) {
    // query stats without the PID from the tuple
    __u32 pid = t->pid;
    t->pid = 0;

    // initialize-if-no-exist the connetion state, and load it
    tcp_stats_t empty = {};
    bpf_map_update_elem(&tcp_stats, t, &empty, BPF_NOEXIST);

    tcp_stats_t* val = bpf_map_lookup_elem(&tcp_stats, t);
    t->pid = pid;
    if (val == NULL) {
        return;
    }

    if (stats.retransmits > 0) {
        __sync_fetch_and_add(&val->retransmits, stats.retransmits);
    }

    if (stats.rtt > 0) {
        // For more information on the bit shift operations see:
        // https://elixir.bootlin.com/linux/v4.6/source/net/ipv4/tcp.c#L2686
        val->rtt = stats.rtt >> 3;
        val->rtt_var = stats.rtt_var >> 2;
    }

    if (stats.state_transitions > 0) {
        val->state_transitions |= stats.state_transitions;
    }
}

static __always_inline int handle_message(conn_tuple_t* t, size_t sent_bytes, size_t recv_bytes) {
    u64 ts = bpf_ktime_get_ns();

    update_conn_stats(t, sent_bytes, recv_bytes, ts);

    return 0;
}

static __always_inline int handle_retransmit(struct sock* sk, int segs) {
    conn_tuple_t t = {};
    u64 zero = 0;

    if (!read_conn_tuple(&t, sk, zero, CONN_TYPE_TCP)) {
        return 0;
    }

    tcp_stats_t stats = { .retransmits = segs, .rtt = 0, .rtt_var = 0 };
    update_tcp_stats(&t, stats);

    return 0;
}

static __always_inline void handle_tcp_stats(conn_tuple_t* t, struct sock* skp) {
    __u32 rtt = 0, rtt_var = 0;
    bpf_probe_read(&rtt, sizeof(rtt), &tcp_sk(skp)->srtt_us);
    bpf_probe_read(&rtt_var, sizeof(rtt_var), &tcp_sk(skp)->mdev_us);

    tcp_stats_t stats = { .retransmits = 0, .rtt = rtt, .rtt_var = rtt_var };
    update_tcp_stats(t, stats);
}

SEC("kprobe/tcp_sendmsg")
int kprobe__tcp_sendmsg(struct pt_regs* ctx) {
    struct sock* skp = (struct sock*)PT_REGS_PARM1(ctx);
    size_t size = (size_t)PT_REGS_PARM3(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_sendmsg: pid_tgid: %d, size: %d\n", pid_tgid, size);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, skp, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, skp);
    return handle_message(&t, size, 0);
}

SEC("kprobe/tcp_sendmsg/pre_4_1_0")
int kprobe__tcp_sendmsg__pre_4_1_0(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM2(ctx);
    size_t size = (size_t)PT_REGS_PARM4(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_sendmsg/pre_4_1_0: pid_tgid: %d, size: %d\n", pid_tgid, size);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, sk);
    return handle_message(&t, size, 0);
}

SEC("kretprobe/tcp_sendmsg")
int kretprobe__tcp_sendmsg(struct pt_regs* ctx) {
#if DEBUG == 1
    int ret = PT_REGS_RC(ctx);

    log_debug("kretprobe/tcp_sendmsg: return: %d\n", ret);
    // If ret < 0 it means an error occurred but we still counted the bytes as being sent
    // let's increment our miscount count
    if (ret < 0) {
        increment_telemetry_count(tcp_sent_miscounts);
    }
#endif
    http_notify_batch(ctx);

    return 0;
}

SEC("kprobe/tcp_cleanup_rbuf")
int kprobe__tcp_cleanup_rbuf(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    int copied = (int)PT_REGS_PARM2(ctx);
    if (copied < 0) {
        return 0;
    }
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_cleanup_rbuf: pid_tgid: %d, copied: %d\n", pid_tgid, copied);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    return handle_message(&t, 0, copied);
}

SEC("kprobe/tcp_close")
int kprobe__tcp_close(struct pt_regs* ctx) {
    struct sock* sk;
    conn_tuple_t t = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();
    sk = (struct sock*)PT_REGS_PARM1(ctx);

    // Get network namespace id
    log_debug("kprobe/tcp_close: pid_tgid: %d, ns: %d\n", pid_tgid, get_netns_from_sock(sk));

    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    cleanup_conn(&t);
    return 0;
}

SEC("kretprobe/tcp_close")
int kretprobe__tcp_close(struct pt_regs* ctx) {
    u32 cpu = bpf_get_smp_processor_id();
    batch_t* batch_ptr = bpf_map_lookup_elem(&conn_close_batch, &cpu);
    if (batch_ptr == NULL) {
        return 0;
    }

    if (batch_ptr->pos >= CONN_CLOSED_BATCH_SIZE) {
        // Here we copy the batch data to a variable allocated in the eBPF stack
        // This is necessary for older Kernel versions only (we validated this behavior on 4.4.0),
        // since you can't directly write a map entry to the perf buffer.
        batch_t batch_copy = {};
        __builtin_memcpy(&batch_copy, batch_ptr, sizeof(batch_copy));
        bpf_perf_event_output(ctx, &conn_close_event, cpu, &batch_copy, sizeof(batch_copy));
        batch_ptr->pos = 0;
    }

    return 0;
}

SEC("kprobe/ip6_make_skb")
int kprobe__ip6_make_skb(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    size_t size = (size_t)PT_REGS_PARM4(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    size = size - sizeof(struct udphdr);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP)) {
        increment_telemetry_count(udp_send_missed);
        return 0;
    }

    log_debug("kprobe/ip6_make_skb: pid_tgid: %d, size: %d\n", pid_tgid, size);
    handle_message(&t, size, 0);
    increment_telemetry_count(udp_send_processed);

    return 0;
}

// Note: This is used only in tne UDP send path.
SEC("kprobe/ip_make_skb")
int kprobe__ip_make_skb(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    size_t size = (size_t)PT_REGS_PARM5(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();

    size = size - sizeof(struct udphdr);

    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_UDP)) {
        struct flowi4* fl4 = (struct flowi4*)PT_REGS_PARM2(ctx);
        bpf_probe_read(&t.saddr_l, sizeof(__be32), &fl4->saddr);
        bpf_probe_read(&t.daddr_l, sizeof(__be32), &fl4->daddr);
        if (!t.saddr_l || !t.daddr_l) {
            log_debug("ERR(fl4): src/dst addr not set src:%d,dst:%d\n", t.saddr_l, t.daddr_l);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }

        bpf_probe_read(&t.sport, sizeof(t.sport), &fl4->fl4_sport);
        bpf_probe_read(&t.dport, sizeof(t.dport), &fl4->fl4_dport);
        t.sport = bpf_ntohs(t.sport);
        t.dport = bpf_ntohs(t.dport);
        if (t.sport == 0 || t.dport == 0) {
            log_debug("ERR(fl4): src/dst port not set: src:%d, dst:%d\n", t.sport, t.dport);
            increment_telemetry_count(udp_send_missed);
            return 0;
        }
    }

    log_debug("kprobe/ip_send_skb: pid_tgid: %d, size: %d\n", pid_tgid, size);
    handle_message(&t, size, 0);
    increment_telemetry_count(udp_send_processed);

    return 0;
}

// We can only get the accurate number of copied bytes from the return value, so we pass our
// sock* pointer from the kprobe to the kretprobe via a map (udp_recv_sock) to get all required info
//
// The same issue exists for TCP, but we can conveniently use the downstream function tcp_cleanup_rbuf
//
// On UDP side, no similar function exists in all kernel versions, though we may be able to use something like
// skb_consume_udp (v4.10+, https://elixir.bootlin.com/linux/v4.10/source/net/ipv4/udp.c#L1500)
SEC("kprobe/udp_recvmsg")
int kprobe__udp_recvmsg(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    struct msghdr* msg = (struct msghdr*) PT_REGS_PARM2(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    udp_recv_sock_t t = { .sk = NULL, .msg = NULL };
    if (sk) bpf_probe_read(&t.sk, sizeof(t.sk), &sk);
    if (msg) bpf_probe_read(&t.msg, sizeof(t.msg), &msg);

    // Store pointer to the socket using the pid/tgid
    bpf_map_update_elem(&udp_recv_sock, &pid_tgid, &t, BPF_ANY);
    log_debug("kprobe/udp_recvmsg: pid_tgid: %d\n", pid_tgid);

    return 0;
}

SEC("kprobe/udp_recvmsg/pre_4_1_0")
int kprobe__udp_recvmsg_pre_4_1_0(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM2(ctx);
    struct msghdr* msg = (struct msghdr*) PT_REGS_PARM3(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    udp_recv_sock_t t = { .sk = NULL, .msg = NULL };
    if (sk) bpf_probe_read(&t.sk, sizeof(t.sk), &sk);
    if (msg) bpf_probe_read(&t.msg, sizeof(t.msg), &msg);

    // Store pointer to the socket using the pid/tgid
    bpf_map_update_elem(&udp_recv_sock, &pid_tgid, &t, BPF_ANY);
    log_debug("kprobe/udp_recvmsg/pre_4_1_0: pid_tgid: %d\n", pid_tgid);

    return 0;
}

SEC("kretprobe/udp_recvmsg")
int kretprobe__udp_recvmsg(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();

    // Retrieve socket pointer from kprobe via pid/tgid
    udp_recv_sock_t* st = bpf_map_lookup_elem(&udp_recv_sock, &pid_tgid);
    if (!st) { // Missed entry
        return 0;
    }

    // Make sure we clean up the key
    bpf_map_delete_elem(&udp_recv_sock, &pid_tgid);

    int copied = (int)PT_REGS_RC(ctx);
    if (copied < 0) { // Non-zero values are errors (or a peek) (e.g -EINVAL)
        log_debug("kretprobe/udp_recvmsg: ret=%d < 0, pid_tgid=%d\n", copied, pid_tgid);
        return 0;
    }

    log_debug("kretprobe/udp_recvmsg: ret=%d\n", copied);

    struct sockaddr * sa = NULL;
    if (st->msg) {
        bpf_probe_read(&sa, sizeof(sa), &(st->msg->msg_name));
    }

    conn_tuple_t t = {};
    __builtin_memset(&t, 0, sizeof(conn_tuple_t));
    sockaddr_to_addr(sa, &t.daddr_h, &t.daddr_l, &t.dport);

    if (!read_conn_tuple_partial(&t, st->sk, pid_tgid, CONN_TYPE_UDP)) {
        log_debug("ERR(kretprobe/udp_recvmsg): error reading conn tuple, pid_tgid=%d\n", pid_tgid);
        return 0;
    }

    log_debug("kretprobe/udp_recvmsg: pid_tgid: %d, return: %d\n", pid_tgid, copied);
    handle_message(&t, 0, copied);

    return 0;
}

SEC("kprobe/tcp_retransmit_skb")
int kprobe__tcp_retransmit_skb(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);

#if LINUX_VERSION_CODE < KERNEL_VERSION(4, 7, 0)
    int segs = 1;
#else
    int segs = (int)PT_REGS_PARM3(ctx);
#endif
    log_debug("kprobe/tcp_retransmit\n");

    return handle_retransmit(sk, segs);
}

SEC("kprobe/tcp_set_state")
int kprobe__tcp_set_state(struct pt_regs* ctx) {
    u8 state = (u8)PT_REGS_PARM2(ctx);

    // For now we're tracking only TCP_ESTABLISHED
    if (state != TCP_ESTABLISHED) {
        return 0;
    }

    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    tcp_stats_t stats = { .state_transitions = (1 << state) };
    update_tcp_stats(&t, stats);

    return 0;
}

SEC("kretprobe/inet_csk_accept")
int kretprobe__inet_csk_accept(struct pt_regs* ctx) {
    struct sock* skp = (struct sock*)PT_REGS_RC(ctx);
    if (!skp) {
        return 0;
    }

    __u16 lport = read_sport(skp);
    if (lport == 0) {
        return 0;
    }

    port_binding_t t = {};
    t.net_ns = get_netns_from_sock(skp);
    t.port = lport;

    __u8* val = bpf_map_lookup_elem(&port_bindings, &t);

    if (val == NULL) {
        __u8 state = PORT_LISTENING;
        bpf_map_update_elem(&port_bindings, &t, &state, BPF_ANY);
    }

    log_debug("kretprobe/inet_csk_accept: net ns: %d, lport: %d\n", t.net_ns, t.port);
    return 0;
}

SEC("kprobe/tcp_v4_destroy_sock")
int kprobe__tcp_v4_destroy_sock(struct pt_regs* ctx) {
    struct sock* skp = (struct sock*)PT_REGS_PARM1(ctx);
    if (!skp) {
        log_debug("ERR(tcp_v4_destroy_sock): socket is null \n");
        return 0;
    }

    __u16 lport = read_sport(skp);
    if (lport == 0) {
        log_debug("ERR(tcp_v4_destroy_sock): lport is 0 \n");
        return 0;
    }

    port_binding_t t = { .net_ns = 0, .port = 0 };
    t.net_ns = get_netns_from_sock(skp);
    t.port = lport;
    __u8* val = bpf_map_lookup_elem(&port_bindings, &t);
    if (val != NULL) {
        __u8 state = PORT_CLOSED;
        bpf_map_update_elem(&port_bindings, &t, &state, BPF_ANY);
    }

    log_debug("kprobe/tcp_v4_destroy_sock: net ns: %u, lport: %u\n", t.net_ns, t.port);
    return 0;
}

SEC("kprobe/udp_destroy_sock")
int kprobe__udp_destroy_sock(struct pt_regs* ctx) {
    struct sock* skp = (struct sock*)PT_REGS_PARM1(ctx);
    if (!skp) {
        log_debug("ERR(udp_destroy_sock): socket is null \n");
        return 0;
    }

    conn_tuple_t tup = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();
    if (read_conn_tuple(&tup, skp, pid_tgid, CONN_TYPE_UDP)) {
        cleanup_conn(&tup);
    }

    // get the port for the current sock
    __u16 lport = read_sport(skp);
    if (lport == 0) {
        log_debug("ERR(udp_destroy_sock): lport is 0 \n");
        return 0;
    }

    // decide if the port is bound, if not, do nothing
    port_binding_t t = {};
    // although we have net ns info, we don't use it in the key
    // since we don't have it everywhere for udp port bindings
    // (see sys_enter_bind/sys_exit_bind below)
    t.net_ns = 0;
    t.port = lport;
    __u8* state = bpf_map_lookup_elem(&udp_port_bindings, &t);

    if (state == NULL) {
        log_debug("kprobe/udp_destroy_sock: sock was not listening, will drop event\n");
        return 0;
    }

    // set the state to closed
    __u8 new_state = PORT_CLOSED;
    bpf_map_update_elem(&udp_port_bindings, &t, &new_state, BPF_ANY);

    log_debug("kprobe/udp_destroy_sock: port %d marked as closed\n", lport);

    return 0;
}

SEC("kretprobe/udp_destroy_sock")
int kretprobe__udp_destroy_sock(struct pt_regs * ctx) {
    flush_conn_close_if_full(ctx);
    return 0;
}

//region sys_enter_bind

static __always_inline int sys_enter_bind(struct socket* sock, struct sockaddr* addr) {
    __u64 tid = bpf_get_current_pid_tgid();

    __u16 type = 0;
    bpf_probe_read(&type, sizeof(__u16), &sock->type);
    if ((type & SOCK_DGRAM) == 0) {
        return 0;
    }

    if (addr == NULL) {
        log_debug("sys_enter_bind: could not read sockaddr, sock=%llx, tid=%u\n", sock, tid);
        return 0;
    }

    u16 sin_port = 0;
    sa_family_t family = 0;
    bpf_probe_read(&family, sizeof(sa_family_t), &addr->sa_family);
    if (family == AF_INET) {
        bpf_probe_read(&sin_port, sizeof(u16), &(((struct sockaddr_in*)addr)->sin_port));
    } else if (family == AF_INET6) {
        bpf_probe_read(&sin_port, sizeof(u16), &(((struct sockaddr_in6*)addr)->sin6_port));
    }

    sin_port = bpf_ntohs(sin_port);
    if (sin_port == 0) {
        log_debug("ERR(sys_enter_bind): sin_port is 0\n");
        return 0;
    }

    // write to pending_binds so the retprobe knows we can mark this as binding.
    bind_syscall_args_t args = {};
    args.port = sin_port;

    bpf_map_update_elem(&pending_bind, &tid, &args, BPF_ANY);
    log_debug("sys_enter_bind: started a bind on UDP port=%d sock=%llx tid=%u\n", sin_port, sock, tid);

    return 0;
}

SEC("kprobe/inet_bind")
int kprobe__inet_bind(struct pt_regs* ctx) {
    struct socket *sock = (struct socket*)PT_REGS_PARM1(ctx);
    struct sockaddr* addr = (struct sockaddr*)PT_REGS_PARM2(ctx);
    log_debug("kprobe/inet_bind: sock=%llx, umyaddr=%x\n", sock, addr);
    return sys_enter_bind(sock, addr);
}

SEC("kprobe/inet6_bind")
int kprobe__inet6_bind(struct pt_regs* ctx) {
    struct socket *sock = (struct socket*)PT_REGS_PARM1(ctx);
    struct sockaddr* addr = (struct sockaddr*)PT_REGS_PARM2(ctx);
    log_debug("kprobe/inet6_bind: sock=%llx, umyaddr=%x\n", sock, addr);
    return sys_enter_bind(sock, addr);
}

//endregion

//region sys_exit_bind

static __always_inline int sys_exit_bind(__s64 ret) {
    __u64 tid = bpf_get_current_pid_tgid();

    // bail if this bind() is not the one we're instrumenting
    bind_syscall_args_t* args;
    args = bpf_map_lookup_elem(&pending_bind, &tid);

    log_debug("sys_exit_bind: tid=%u, ret=%d\n", tid, ret);

    if (args == NULL) {
        log_debug("sys_exit_bind: was not a UDP bind, will not process\n");
        return 0;
    }

    bpf_map_delete_elem(&pending_bind, &tid);

    if (ret != 0) {
        return 0;
    }

    __u16 sin_port = args->port;
    __u8 port_state = PORT_LISTENING;
    port_binding_t t = {};
    t.net_ns = 0; // don't have net ns info in this context
    t.port = sin_port;
    bpf_map_update_elem(&udp_port_bindings, &t, &port_state, BPF_ANY);
    log_debug("sys_exit_bind: bound UDP port %u\n", sin_port);

    return 0;
}

SEC("kretprobe/inet_bind")
int kretprobe__inet_bind(struct pt_regs* ctx) {
    __s64 ret = PT_REGS_RC(ctx);
    log_debug("kretprobe/inet_bind: ret=%d\n", ret);
    return sys_exit_bind(ret);
}

SEC("kretprobe/inet6_bind")
int kretprobe__inet6_bind(struct pt_regs* ctx) {
    __s64 ret = PT_REGS_RC(ctx);
    log_debug("kretprobe/inet6_bind: ret=%d\n", ret);
    return sys_exit_bind(ret);
}

//endregion

// This function is meant to be used as a BPF_PROG_TYPE_SOCKET_FILTER.
// When attached to a RAW_SOCKET, this code filters out everything but DNS traffic.
// All structs referenced here are kernel independent as they simply map protocol headers (Ethernet, IP and UDP).
SEC("socket/dns_filter")
int socket__dns_filter(struct __sk_buff* skb) {
    skb_info_t skb_info;

    if (!read_conn_tuple_skb(skb, &skb_info)) {
        return 0;
    }

#ifdef FEATURE_DNS_STATS_ENABLED
    if (skb_info.tup.sport != 53 && skb_info.tup.dport != 53) {
        return 0;
    }
#else
    if (skb_info.tup.sport != 53) {
        return 0;
    }
#endif

    return -1;
}

SEC("socket/http_filter")
int socket__http_filter(struct __sk_buff* skb) {
    skb_info_t skb_info;

    if (!read_conn_tuple_skb(skb, &skb_info)) {
        return 0;
    }

    if (skb_info.tup.sport != 80 && skb_info.tup.sport != 8080 && skb_info.tup.dport != 80 && skb_info.tup.dport != 8080) {
        return 0;
    }

    if (skb_info.tup.sport == 80 || skb_info.tup.sport == 8080) {
        // Normalize tuple
        flip_tuple(&skb_info.tup);
    }

    http_handle_packet(skb, &skb_info);

    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
