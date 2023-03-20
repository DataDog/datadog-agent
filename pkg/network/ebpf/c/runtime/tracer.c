#include "kconfig.h"
#include "bpf_telemetry.h"
#include "bpf_builtins.h"
#include "bpf_tracing.h"

#include "tracer-bind.h"
#include "tracer-tcp.h"
#include "tracer-udp.h"

#include "protocols/classification/tracer-maps.h"
#include "protocols/classification/protocol-classification.h"

#ifndef LINUX_VERSION_CODE
#error "kernel version not included?"
#endif

SEC("socket/classifier_entry")
int socket__classifier_entry(struct __sk_buff *skb) {
    #if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 6, 0)
    protocol_classifier_entrypoint(skb);
    #endif
    return 0;
}

SEC("socket/classifier_queues")
int socket__classifier_queues(struct __sk_buff *skb) {
    #if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 6, 0)
    protocol_classifier_entrypoint_queues(skb);
    #endif
    return 0;
}

SEC("socket/classifier_dbs")
int socket__classifier_dbs(struct __sk_buff *skb) {
    #if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 6, 0)
    protocol_classifier_entrypoint_dbs(skb);
    #endif
    return 0;
}

SEC("kprobe/tcp_sendpage")
int kprobe__tcp_sendpage(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/tcp_sendpage: pid_tgid: %d\n", pid_tgid);
    struct sock *skp = (struct sock *)PT_REGS_PARM1(ctx);
    bpf_map_update_with_telemetry(tcp_sendpage_args, &pid_tgid, &skp, BPF_ANY);
    return 0;
}

SEC("kretprobe/tcp_sendpage")
int kretprobe__tcp_sendpage(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock **skpp = (struct sock **)bpf_map_lookup_elem(&tcp_sendpage_args, &pid_tgid);
    if (!skpp) {
        log_debug("kretprobe/tcp_sendpage: sock not found\n");
        return 0;
    }

    struct sock *skp = *skpp;
    bpf_map_delete_elem(&tcp_sendpage_args, &pid_tgid);

    int sent = PT_REGS_RC(ctx);
    if (sent < 0) {
        return 0;
    }

    if (!skp) {
        return 0;
    }

    log_debug("kretprobe/tcp_sendpage: pid_tgid: %d, sent: %d, sock: %x\n", pid_tgid, sent, skp);
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

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
