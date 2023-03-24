#include "kconfig.h"
#include "bpf_telemetry.h"
#include "bpf_builtins.h"
#include "bpf_tracing.h"
#include "bpf_endian.h"


#include "offsets.h"
#include "tracer-bind.h"
#include "tracer-tcp.h"
#include "tracer-udp.h"

#include "protocols/classification/tracer-maps.h"
#include "protocols/classification/protocol-classification.h"

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

SEC("kprobe/tcp_retransmit_skb")
int kprobe__tcp_retransmit_skb(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    int segs = (int)PT_REGS_PARM3(ctx);
    log_debug("kprobe/tcp_retransmit: segs: %d\n", segs);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    tcp_retransmit_skb_args_t args = {};
    args.sk = sk;
    args.segs = segs;
    bpf_map_update_with_telemetry(pending_tcp_retransmit_skb, &pid_tgid, &args, BPF_ANY);
    return 0;
}

SEC("kprobe/tcp_retransmit_skb")
int kprobe__tcp_retransmit_skb_pre_4_7_0(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    log_debug("kprobe/tcp_retransmit/pre_4_7_0\n");
    u64 pid_tgid = bpf_get_current_pid_tgid();
    tcp_retransmit_skb_args_t args = {};
    args.sk = sk;
    args.segs = 1;
    bpf_map_update_with_telemetry(pending_tcp_retransmit_skb, &pid_tgid, &args, BPF_ANY);
    return 0;
}

SEC("kretprobe/tcp_retransmit_skb")
int kretprobe__tcp_retransmit_skb(struct pt_regs *ctx) {
    int ret = PT_REGS_RC(ctx);
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
    log_debug("kretprobe/tcp_retransmit: segs: %d\n", segs);
    return handle_retransmit(sk, segs);
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
