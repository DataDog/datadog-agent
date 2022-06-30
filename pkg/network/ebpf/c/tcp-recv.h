#ifndef __TCP_RECV_H__
#define __TCP_RECV_H__

#include "bpf_helpers.h"
#include "tracer-stats.h"
#include "tracer-maps.h"

int __always_inline handle_tcp_recv(u64 pid_tgid, struct sock *skp, int recv) {
    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, skp, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, skp, 0);

    __u32 packets_in = 0;
    __u32 packets_out = 0;
    get_tcp_segment_counts(skp, &packets_in, &packets_out);

    return handle_message(&t, 0, recv, CONN_DIRECTION_UNKNOWN, packets_out, packets_in, PACKET_COUNT_ABSOLUTE);
}

SEC("kprobe/tcp_recvmsg")
int kprobe__tcp_recvmsg(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock* skp = (struct sock*) PT_REGS_PARM1(ctx);
    bpf_map_update_elem(&tcp_recvmsg_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

SEC("kretprobe/tcp_recvmsg")
int kretprobe__tcp_recvmsg(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock **skpp = (struct sock**) bpf_map_lookup_elem(&tcp_recvmsg_args, &pid_tgid);
    if (!skpp) {
        return 0;
    }

    bpf_map_delete_elem(&tcp_recvmsg_args, &pid_tgid);

    struct sock *skp = *skpp;
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
    struct sock* skp = (struct sock*) PT_REGS_PARM1(ctx);
    bpf_map_update_elem(&tcp_recvmsg_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

SEC("kretprobe/tcp_read_sock")
int kretprobe__tcp_read_sock(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock **skpp = (struct sock**) bpf_map_lookup_elem(&tcp_recvmsg_args, &pid_tgid);
    if (!skpp) {
        return 0;
    }

    bpf_map_delete_elem(&tcp_recvmsg_args, &pid_tgid);

    struct sock *skp = *skpp;
    if (!skp) {
        return 0;
    }

    int recv = PT_REGS_RC(ctx);
    if (recv < 0) {
        return 0;
    }

    return handle_tcp_recv(pid_tgid, skp, recv);
}


#endif // __TCP_RECV_H__
