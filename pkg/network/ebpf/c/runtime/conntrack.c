#include "ktypes.h"
#include "bpf_metadata.h"
#ifdef COMPILE_RUNTIME
#include "kconfig.h"
#endif

#include "bpf_tracing.h"
#include "bpf_telemetry.h"
#include "bpf_endian.h"
#include "bpf_bypass.h"

#ifdef COMPILE_RUNTIME
#include <linux/version.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/ipv6.h>
#include <uapi/linux/udp.h>
#endif

#include "defs.h"
#include "conntrack.h"
#include "conntrack/maps.h"
#include "netns.h"
#include "ip.h"
#include "pid_tgid.h"

#if defined(FEATURE_TCPV6_ENABLED) || defined(FEATURE_UDPV6_ENABLED)
#include "ipv6.h"
#endif

SEC("kprobe/__nf_conntrack_hash_insert")
int BPF_BYPASSABLE_KPROBE(kprobe___nf_conntrack_hash_insert, struct nf_conn *ct) {
    log_debug("kprobe/__nf_conntrack_hash_insert: netns: %u", get_netns(ct));

    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        return 0;
    }
    RETURN_IF_NOT_NAT(&orig, &reply);

    bpf_map_update_with_telemetry(conntrack, &orig, &reply, BPF_ANY);
    bpf_map_update_with_telemetry(conntrack, &reply, &orig, BPF_ANY);
    increment_telemetry_registers_count();

    return 0;
}

#if defined(COMPILE_CORE) || defined(FEATURE_CONNTRACK_ALTERNATE_PROBES_SUPPORTED)
static __always_inline int kprobe_conntrack_common(struct nf_conn *ct) {
    u64 pid_tgid = bpf_get_current_pid_tgid();

    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        return 0;
    }
    RETURN_IF_NOT_NAT(&orig, &reply);

    bpf_map_update_with_telemetry(conntrack_args, &pid_tgid, &ct, BPF_ANY);

    return 0;
}

SEC("kprobe/__nf_conntrack_confirm")
int BPF_BYPASSABLE_KPROBE(kprobe___nf_conntrack_confirm, struct sk_buff *skb) {
    struct nf_conn *ct = get_nfct(skb);
    if (!ct) {
        log_debug("kprobe/__nf_conntrack_confirm: null ct");
        return 0;
    }
    log_debug("kprobe/__nf_conntrack_confirm: netns: %u", get_netns(ct));

    kprobe_conntrack_common(ct);
    return 0;
}

SEC("kprobe/nf_conntrack_hash_check_insert")
int BPF_BYPASSABLE_KPROBE(kprobe_nf_conntrack_hash_check_insert, struct nf_conn *ct) {
    if (!ct) {
        log_debug("kprobe/nf_conntrack_hash_check_insert: null ct");
        return 0;
    }
    log_debug("kprobe/nf_conntrack_hash_check_insert: netns: %u", get_netns(ct));

    kprobe_conntrack_common(ct);
    return 0;
}

static __always_inline int kretprobe_conntrack_common(struct pt_regs *ctx, int expected_retval) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct nf_conn **ctpp = (struct nf_conn **)bpf_map_lookup_elem(&conntrack_args, &pid_tgid);
    if (!ctpp) {
        return 0;
    }

    struct nf_conn *ct = *ctpp;
    bpf_map_delete_elem(&conntrack_args, &pid_tgid);
    if (!ct) {
        return 0;
    }

    int retval = PT_REGS_RC(ctx);
    if (retval != expected_retval) {
        return 0;
    }

    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        return 0;
    }

    bpf_map_update_with_telemetry(conntrack, &orig, &reply, BPF_ANY);
    bpf_map_update_with_telemetry(conntrack, &reply, &orig, BPF_ANY);
    increment_telemetry_registers_count();

    return 0;
}

SEC("kretprobe/__nf_conntrack_confirm")
int BPF_BYPASSABLE_KPROBE(kretprobe___nf_conntrack_confirm) {
    return kretprobe_conntrack_common(ctx, 1); // NF_ACCEPT = 1
}

SEC("kretprobe/nf_conntrack_hash_check_insert")
int BPF_BYPASSABLE_KPROBE(kretprobe_nf_conntrack_hash_check_insert) {
    return kretprobe_conntrack_common(ctx, 0); // success = 0
}
#endif // COMPILE_CORE || FEATURE_CONNTRACK_ALTERNATE_PROBES_SUPPORTED

SEC("kprobe/ctnetlink_fill_info")
int BPF_BYPASSABLE_KPROBE(kprobe_ctnetlink_fill_info) {
    u32 pid = GET_USER_MODE_PID(bpf_get_current_pid_tgid());
    if (pid != systemprobe_pid()) {
        log_debug("skipping kprobe/ctnetlink_fill_info invocation from non-system-probe process");
        return 0;
    }

    struct nf_conn *ct = (struct nf_conn*)PT_REGS_PARM5(ctx);

    u32 status = 0;
    BPF_CORE_READ_INTO(&status, ct, status);
    if (!(status&IPS_CONFIRMED) || !(status&IPS_NAT_MASK)) {
        return 0;
    }

    log_debug("kprobe/ctnetlink_fill_info: netns: %u, status: %x", get_netns(ct), status);

    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        return 0;
    }

    bpf_map_update_with_telemetry(conntrack, &orig, &reply, BPF_ANY);
    bpf_map_update_with_telemetry(conntrack, &reply, &orig, BPF_ANY);
    increment_telemetry_registers_count();

    return 0;
}

char _license[] SEC("license") = "GPL";
