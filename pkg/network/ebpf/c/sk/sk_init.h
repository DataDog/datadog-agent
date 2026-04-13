#ifndef __SK_INIT_H
#define __SK_INIT_H

#include "ktypes.h"
#include "bpf_helpers.h"

#include "defs.h"
#include "maps.h"
#include "tracer/port.h"
#include "ip.h"
#include "ipv6.h"
#include "netns.h"

struct inode___418 {
    struct timespec64 i_ctime;
};

struct inode___66 {
    struct timespec64 __i_ctime;
};

static __always_inline u64 read_inode_ctime(struct inode *inode) {
    // 4.18 - https://github.com/torvalds/linux/commit/95582b00838837fc07e042979320caf917ce3fe6
    if (bpf_core_field_exists(((struct inode___418*)inode)->i_ctime)) {
        return ((u64)BPF_CORE_READ((struct inode___418*)inode, i_ctime.tv_sec) * NSEC_PER_SEC) +
            (u64)BPF_CORE_READ((struct inode___418*)inode, i_ctime.tv_nsec);
    }
    // 6.6 - https://github.com/torvalds/linux/commit/13bc24457850583a2e7203ded05b7209ab4bc5ef
    if (bpf_core_field_exists(((struct inode___66*)inode)->__i_ctime)) {
        return ((u64)BPF_CORE_READ((struct inode___66*)inode, __i_ctime.tv_sec) * NSEC_PER_SEC) +
            (u64)BPF_CORE_READ((struct inode___66*)inode, __i_ctime.tv_nsec);
    }
    // 6.11 - https://github.com/torvalds/linux/commit/3aa63a569c64e708df547a8913c84e64a06e7853
    return ((u64)inode->i_ctime_sec * NSEC_PER_SEC) + (u64)inode->i_ctime_nsec;
}

static __always_inline void initialize_tcp_socket(struct sock *sk, struct task_struct *task, struct file *file) {
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get(&sk_tcp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return;
    }
    struct tcp_sock *tp = bpf_skc_to_tcp_sock(sk);
    if (!tp) {
        return;
    }

    sk_stats->initial.sent_bytes = tp->bytes_sent;
    sk_stats->initial.recv_bytes = tp->bytes_received;
    sk_stats->initial.sent_packets = tp->segs_out;
    sk_stats->initial.recv_packets = tp->segs_in;
    sk_stats->initial.retransmits = tp->total_retrans;
    sk_stats->initial.reord_seen = tp->reord_seen;
    sk_stats->initial.rcv_ooopack = tp->rcv_ooopack;
    sk_stats->initial.delivered_ce = tp->delivered_ce;

    sk_stats->tup.pid = task ? task->tgid : GET_USER_MODE_PID(bpf_get_current_pid_tgid());
    sk_stats->tup.netns = get_netns_from_sock(sk);
    sk_stats->start_ns = (file && file->f_inode) ? read_inode_ctime(file->f_inode) : tp->tcp_mstamp;

    port_binding_t pb = {};
    pb.netns = sk_stats->tup.netns;
    pb.port = read_sport(sk);
    u32 *port_count = bpf_map_lookup_elem(&port_bindings, &pb);
    sk_stats->direction = (port_count != NULL && *port_count > 0) ? CONN_DIRECTION_INCOMING : CONN_DIRECTION_OUTGOING;
}

static __always_inline void initialize_udp_socket(struct sock *sk, struct task_struct *task, struct file *file) {
    sk_udp_stats_t *sk_stats = bpf_sk_storage_get(&sk_udp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return;
    }

    sk_stats->tup.pid = task ? task->tgid : GET_USER_MODE_PID(bpf_get_current_pid_tgid());
    sk_stats->tup.netns = get_netns_from_sock(sk);
    sk_stats->timestamp_ns = bpf_ktime_get_ns();
    sk_stats->start_ns = (file && file->f_inode) ? read_inode_ctime(file->f_inode) : sk_stats->timestamp_ns;

    port_binding_t pb = {};
    pb.netns = sk_stats->tup.netns;
    pb.port = read_sport(sk);
    u32 *port_count = bpf_map_lookup_elem(&udp_port_bindings, &pb);
    sk_stats->direction = (port_count != NULL && *port_count > 0) ? CONN_DIRECTION_INCOMING : CONN_DIRECTION_OUTGOING;
}

SEC("iter/task_file")
int bpf_iter__task_file_initial_sockets(struct bpf_iter__task_file *ctx) {
    struct task_struct *task = ctx->task;
    struct file *file = ctx->file;
    if (!task || !file) {
        return 0;
    }
    struct socket *sock = bpf_sock_from_file(file);
    if (!sock) {
        return 0;
    }
    struct sock *sk = sock->sk;
    if (!is_protocol_family_enabled(sk)) {
        return 0;
    }

    if (sk->sk_protocol == IPPROTO_TCP || sk->sk_protocol == IPPROTO_MPTCP) {
        initialize_tcp_socket(sk, task, file);
    } else if (sk->sk_protocol == IPPROTO_UDP) {
        initialize_udp_socket(sk, task, file);
    }
    return 0;
}

#endif
