#ifndef __TCP_RECV_H__
#define __TCP_RECV_H__

#include "bpf_helpers.h"
#include "bpf_telemetry.h"
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

    return handle_message(&t, 0, recv, CONN_DIRECTION_UNKNOWN, packets_out, packets_in, PACKET_COUNT_ABSOLUTE, skp);
}

SEC("kprobe/tcp_recvmsg")
int kprobe__tcp_recvmsg(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
#if LINUX_VERSION_CODE < KERNEL_VERSION(4, 1, 0)
    void *parm = (void*)PT_REGS_PARM2(ctx);
    int flags = (int)PT_REGS_PARM6(ctx);
#else
    void *parm = (void*)PT_REGS_PARM1(ctx);
    int flags = (int)PT_REGS_PARM5(ctx);
#endif
    if (flags & MSG_PEEK) {
        return 0;
    }

    struct sock *skp = parm;
    bpf_map_update_with_telemetry(tcp_recvmsg_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

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


#endif // __TCP_RECV_H__
