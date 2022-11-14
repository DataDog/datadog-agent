#ifndef __TCP_RECV_H__
#define __TCP_RECV_H__

#include "bpf_helpers.h"
#include "tracer-stats.h"
#include "tracer-maps.h"
#include "tracer-events.h"

static __always_inline protocol_t classify_wrapper(conn_tuple_t *t, void *buffer_ptr, size_t buffer_size) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    if (pid_tgid) {
        log_debug("%llu guy recv addr %llu %llu\n", pid_tgid, t->saddr_l, t->daddr_l);
        log_debug("%llu guy recv port %d %d\n", pid_tgid, t->sport, t->dport);
        log_debug("%llu guy recv pid %d %lu\n", pid_tgid, t->pid, t->netns);
        log_debug("%llu guy recv metadata %d\n", pid_tgid, t->metadata);
    }

    protocol_t protocol = PROTOCOL_UNKNOWN; //get_cached_protocol_or_default(t);
    if (protocol != PROTOCOL_UNKNOWN && protocol != PROTOCOL_UNCLASSIFIED) {
        return protocol;
    }

    if (buffer_ptr == NULL) {
        return PROTOCOL_UNKNOWN;
    }

    size_t buffer_final_size = buffer_size > CLASSIFICATION_MAX_BUFFER ? (CLASSIFICATION_MAX_BUFFER - 1):buffer_size;
    if (buffer_final_size == 0) {
        return PROTOCOL_UNKNOWN;
    }

    char local_buffer_copy[CLASSIFICATION_MAX_BUFFER];
    bpf_memset(local_buffer_copy, 0, CLASSIFICATION_MAX_BUFFER);
    read_into_buffer1(local_buffer_copy, buffer_ptr, buffer_final_size);

    log_debug("guy tcp_recvmsg buffer1 %d; [%d, %d] - guy", buffer_size, local_buffer_copy[0], local_buffer_copy[1]);
    log_debug("guy tcp_recvmsg buffer2 %d; [%d, %d] - guy", buffer_size, local_buffer_copy[2], local_buffer_copy[3]);
    log_debug("guy tcp_recvmsg buffer3 %d; [%d, %d] - guy", buffer_size, local_buffer_copy[4], local_buffer_copy[5]);
    log_debug("guy tcp_recvmsg buffer4 %d; [%d, %d] - guy", buffer_size, local_buffer_copy[6], local_buffer_copy[6]);
    log_debug("guy tcp_recvmsg buffer5 %d; [%d, %d] - guy", buffer_size, local_buffer_copy[8], local_buffer_copy[10]);
    // detect protocol
    classify_protocol(&protocol, local_buffer_copy, buffer_final_size);
    if (protocol != PROTOCOL_UNKNOWN && protocol != PROTOCOL_UNCLASSIFIED) {
        log_debug("3 classified protocol %d", protocol);
        bpf_map_update_with_telemetry(connection_protocol, t, &protocol, BPF_NOEXIST);
    }

    return protocol;
}

int __always_inline handle_tcp_recv(u64 pid_tgid, struct sock *skp, void *buffer_ptr, int recv) {
    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, skp, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, skp, 0);

    __u32 packets_in = 0;
    __u32 packets_out = 0;
    get_tcp_segment_counts(skp, &packets_in, &packets_out);

    protocol_t protocol = classify_wrapper(&t, buffer_ptr, recv);
    return handle_message(&t, 0, recv, CONN_DIRECTION_UNKNOWN, packets_out, packets_in, PACKET_COUNT_ABSOLUTE, protocol, skp);
}

SEC("kprobe/tcp_recvmsg")
int kprobe__tcp_recvmsg(struct pt_regs *ctx) {
    log_debug("guy tcp_recvmsg called");
    u64 pid_tgid = bpf_get_current_pid_tgid();
#if LINUX_VERSION_CODE < KERNEL_VERSION(4, 1, 0)
    void *sk = (void*)PT_REGS_PARM2(ctx);
    void *msghdr = (void*)PT_REGS_PARM3(ctx);
    int nonblock = (int)PT_REGS_PARM5(ctx);
    int flags = (int)PT_REGS_PARM6(ctx);
#else
    void *sk = (void*)PT_REGS_PARM1(ctx);
    void *msghdr = (void*)PT_REGS_PARM2(ctx);
    int nonblock = (int)PT_REGS_PARM4(ctx);
    int flags = (int)PT_REGS_PARM5(ctx);

#endif
    if (flags & MSG_PEEK) {
        log_debug("guy tcp_recvmsg peek %s - guy");
        return 0;
    }

    if (nonblock) {

    }
    log_debug("guy tcp_recvmsg nonblock %d flags %d", nonblock, flags);
    tcp_recvmsg_args_t args = {0};
    args.sk = sk;
    args.msghdr = msghdr;
    bpf_map_update_with_telemetry(tcp_recvmsg_args, &pid_tgid, &args, BPF_ANY);
    return 0;
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

    return handle_tcp_recv(pid_tgid, skp, NULL, recv);
}


#endif // __TCP_RECV_H__
