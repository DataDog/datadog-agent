#ifndef __TCP_RECV_H
#define __TCP_RECV_H

#include "bpf_helpers.h"
#include "bpf_telemetry.h"
#include "bpf_bypass.h"

#include "tracer/stats.h"
#include "tracer/events.h"
#include "tracer/maps.h"
#include "sock.h"
#include "defs.h"

static __always_inline bool is_handle_tcp_recv_skipped() {
    __u64 val = 0;
    LOAD_CONSTANT("skip_handle_tcp_recv", val);
    return val == ENABLED;
}

SEC("kprobe/tcp_recvmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_recvmsg) {
    u64 pid_tgid;
    struct sock *skp;
    int flags;

    RECORD_TIMING(tcp_recvmsg_kprobe_args_calls, tcp_recvmsg_kprobe_args_time_ns, {
        pid_tgid = bpf_get_current_pid_tgid();
#if defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE < KERNEL_VERSION(4, 1, 0)
        skp = (struct sock*)PT_REGS_PARM2(ctx);
        flags = (int)PT_REGS_PARM6(ctx);
#elif defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE < KERNEL_VERSION(5, 19, 0)
        skp = (struct sock*)PT_REGS_PARM1(ctx);
        flags = (int)PT_REGS_PARM5(ctx);
#else
        skp = (struct sock*)PT_REGS_PARM1(ctx);
        flags = (int)PT_REGS_PARM4(ctx);
#endif
    });

    if (flags & MSG_PEEK) {
        return 0;
    }

    RECORD_TIMING(tcp_recvmsg_kprobe_map_update_calls, tcp_recvmsg_kprobe_map_update_time_ns, {
        bpf_map_update_with_telemetry(tcp_recvmsg_args, &pid_tgid, &skp, BPF_ANY);
    });

    return 0;
}

#if defined(COMPILE_CORE) || defined(COMPILE_PREBUILT)

SEC("kprobe/tcp_recvmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_recvmsg__pre_5_19_0) {
    u64 pid_tgid;
    int flags;
    struct sock *skp;

    RECORD_TIMING(tcp_recvmsg_kprobe_args_calls, tcp_recvmsg_kprobe_args_time_ns, {
        pid_tgid = bpf_get_current_pid_tgid();
        flags = (int)PT_REGS_PARM5(ctx);
        skp = (struct sock *)PT_REGS_PARM1(ctx);
    });

    if (flags & MSG_PEEK) {
        return 0;
    }

    RECORD_TIMING(tcp_recvmsg_kprobe_map_update_calls, tcp_recvmsg_kprobe_map_update_time_ns, {
        bpf_map_update_with_telemetry(tcp_recvmsg_args, &pid_tgid, &skp, BPF_ANY);
    });

    return 0;
}

SEC("kprobe/tcp_recvmsg")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_recvmsg__pre_4_1_0) {
    u64 pid_tgid;
    int flags;
    struct sock *skp;

    RECORD_TIMING(tcp_recvmsg_kprobe_args_calls, tcp_recvmsg_kprobe_args_time_ns, {
        pid_tgid = bpf_get_current_pid_tgid();
        flags = (int)PT_REGS_PARM6(ctx);
        skp = (struct sock*)PT_REGS_PARM2(ctx);
    });

    log_debug("kprobe/tcp_recvmsg: pid_tgid: %llu", pid_tgid);

    if (flags & MSG_PEEK) {
        return 0;
    }

    RECORD_TIMING(tcp_recvmsg_kprobe_map_update_calls, tcp_recvmsg_kprobe_map_update_time_ns, {
        bpf_map_update_with_telemetry(tcp_recvmsg_args, &pid_tgid, &skp, BPF_ANY);
    });

    return 0;
}

#endif // COMPILE_CORE || COMPILE_PREBUILT

SEC("kretprobe/tcp_recvmsg")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__tcp_recvmsg, int recv) {
    struct sock **skpp;
    u64 pid_tgid;

    RECORD_TIMING(tcp_recvmsg_kretprobe_map_lookup_calls, tcp_recvmsg_kretprobe_map_lookup_time_ns, {
        pid_tgid = bpf_get_current_pid_tgid();
        skpp = (struct sock**) bpf_map_lookup_elem(&tcp_recvmsg_args, &pid_tgid);
    });

    if (!skpp) {
        return 0;
    }

    struct sock *skp = *skpp;

    RECORD_TIMING(tcp_recvmsg_kretprobe_map_delete_calls, tcp_recvmsg_kretprobe_map_delete_time_ns, {
        bpf_map_delete_elem(&tcp_recvmsg_args, &pid_tgid);
    });

    // Early return for performance testing - skip handle_tcp_recv if configured
    if (is_handle_tcp_recv_skipped()) {
        return 0;
    }

    if (!skp) {
        return 0;
    }

    if (recv < 0) {
        return 0;
    }

    int result;
    RECORD_TIMING(tcp_recvmsg_kretprobe_handle_recv_calls, tcp_recvmsg_kretprobe_handle_recv_time_ns, {
        result = handle_tcp_recv(pid_tgid, skp, recv);
    });

    return result;
}

SEC("kprobe/tcp_read_sock")
int BPF_BYPASSABLE_KPROBE(kprobe__tcp_read_sock, struct sock *skp) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    // we reuse tcp_recvmsg_args here since there is no overlap
    // between the tcp_recvmsg and tcp_read_sock paths

    // Instrument map update with timing
    __u64 start_ns = bpf_ktime_get_ns();
    bpf_map_update_with_telemetry(tcp_recvmsg_args, &pid_tgid, &skp, BPF_ANY);
    __u64 end_ns = bpf_ktime_get_ns();
    __u64 duration = end_ns - start_ns;
    increment_telemetry_count(tcp_recvmsg_kprobe_map_update_calls);
    __u64 key = 0;
    telemetry_t *val = bpf_map_lookup_elem(&telemetry, &key);
    if (val != NULL) {
        __sync_fetch_and_add(&val->tcp_recvmsg_kprobe_map_update_time_ns, duration);
    }

    return 0;
}

SEC("kretprobe/tcp_read_sock")
int BPF_BYPASSABLE_KRETPROBE(kretprobe__tcp_read_sock, int recv) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    // we reuse tcp_recvmsg_args here since there is no overlap
    // between the tcp_recvmsg and tcp_read_sock paths

    struct sock **skpp;
    RECORD_TIMING(tcp_recvmsg_kretprobe_map_lookup_calls, tcp_recvmsg_kretprobe_map_lookup_time_ns, {
        skpp = (struct sock**) bpf_map_lookup_elem(&tcp_recvmsg_args, &pid_tgid);
    });

    if (!skpp) {
        return 0;
    }

    struct sock *skp = *skpp;

    RECORD_TIMING(tcp_recvmsg_kretprobe_map_delete_calls, tcp_recvmsg_kretprobe_map_delete_time_ns, {
        bpf_map_delete_elem(&tcp_recvmsg_args, &pid_tgid);
    });

    // Early return for performance testing - skip handle_tcp_recv if configured
    if (is_handle_tcp_recv_skipped()) {
        return 0;
    }

    if (!skp) {
        return 0;
    }

    if (recv < 0) {
        return 0;
    }

    int result;
    RECORD_TIMING(tcp_recvmsg_kretprobe_handle_recv_calls, tcp_recvmsg_kretprobe_handle_recv_time_ns, {
        result = handle_tcp_recv(pid_tgid, skp, recv);
    });

    return result;
}

#endif // __TCP_RECV_H__
