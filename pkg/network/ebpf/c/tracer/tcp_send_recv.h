#ifndef __TCP_RECV_H
#define __TCP_RECV_H

#include "bpf_helpers.h"
#include "bpf_telemetry.h"
#include "tracer/stats.h"
#include "tracer/events.h"
#include "tracer/maps.h"
#include "sock.h"

SEC("kprobe/tcp_recvmsg")
int kprobe__tcp_recvmsg(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
#if defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE < KERNEL_VERSION(4, 1, 0)
    struct sock *skp = (struct sock*)PT_REGS_PARM2(ctx);
    int flags = (int)PT_REGS_PARM6(ctx);
#elif defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE < KERNEL_VERSION(5, 19, 0)
    struct sock *skp = (struct sock*)PT_REGS_PARM1(ctx);
    int flags = (int)PT_REGS_PARM5(ctx);
#else
    struct sock *skp = (struct sock*)PT_REGS_PARM1(ctx);
    int flags = (int)PT_REGS_PARM4(ctx);
#endif
    if (flags & MSG_PEEK) {
        return 0;
    }

    bpf_map_update_with_telemetry(tcp_recvmsg_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

#if defined(COMPILE_CORE) || defined(COMPILE_PREBUILT)

SEC("kprobe/tcp_recvmsg")
int kprobe__tcp_recvmsg__pre_5_19_0(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    int flags = (int)PT_REGS_PARM5(ctx);
    if (flags & MSG_PEEK) {
        return 0;
    }
    struct sock *skp = (struct sock *)PT_REGS_PARM1(ctx);
    bpf_map_update_with_telemetry(tcp_recvmsg_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

SEC("kprobe/tcp_recvmsg")
int kprobe__tcp_recvmsg__pre_4_1_0(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_recvmsg: pid_tgid: %d\n", pid_tgid);
    int flags = (int)PT_REGS_PARM6(ctx);
    if (flags & MSG_PEEK) {
        return 0;
    }

    struct sock *skp = (struct sock*)PT_REGS_PARM2(ctx);
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

SEC("kprobe/tcp_read_sock")
int kprobe__tcp_read_sock(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock* skp = (struct sock*)PT_REGS_PARM1(ctx);
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

SEC("fexit/tcp_sendmsg")
int BPF_PROG(tcp_sendmsg_exit, struct sock *sk, struct msghdr *msg, size_t size, int sent) {
    RETURN_IF_NOT_IN_SYSPROBE_TASK("fexit/tcp_sendmsg");
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

#endif // __TCP_RECV_H__
