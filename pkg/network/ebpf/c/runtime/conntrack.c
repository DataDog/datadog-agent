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

    if (!is_conn_nat(&orig, &reply)) {
        return 0;
    }

    bpf_map_update_with_telemetry(conntrack, &orig, &reply, BPF_ANY);
    bpf_map_update_with_telemetry(conntrack, &reply, &orig, BPF_ANY);
    increment_telemetry_registers_count();

    return 0;
}

SEC("kprobe/__nf_conntrack_confirm")
int BPF_BYPASSABLE_KPROBE(kprobe__nf_conntrack_confirm, struct sk_buff *skb) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/__nf_conntrack_confirm: pid_tgid: %llu", pid_tgid);

    // Extract ct from skb using get_nfct() helper which handles _nfct vs nfct field name
    struct nf_conn *ct = get_nfct(skb);
    if (!ct) {
        return 0;
    }

    u32 status = 0;
    BPF_CORE_READ_INTO(&status, ct, status);
    // JMW better check for NAT?
    if (!(status & IPS_NAT_MASK)) {
        log_debug("kprobe/__nf_conntrack_confirm: not IPS_NAT_MASK ct=%p status=%x", ct, status);
        return 0;
    }

    // Store ct pointer using pid_tgid for correlation with kretprobe
    u64 ct_ptr = (u64)ct;
    bpf_map_update_with_telemetry(conntrack_args, &pid_tgid, &ct_ptr, BPF_ANY);
    log_debug("kprobe/__nf_conntrack_confirm: added to map ct=%p pid_tgid=%llu", ct, pid_tgid);

    return 0;
}

// Track conntrack confirmations (return) - correlation approach
// Return probe: Process successful confirmations and populate conntrack map
SEC("kretprobe/__nf_conntrack_confirm")
int BPF_BYPASSABLE_KPROBE(kretprobe__nf_conntrack_confirm) {
    u64 pid_tgid = bpf_get_current_pid_tgid();

    // Look up the ct pointer from entry probe
    u64 *ct_ptr = bpf_map_lookup_elem(&conntrack_args, &pid_tgid);
    if (!ct_ptr) {
        // No matching entry probe - this can happen if entry was filtered out (not NAT)
        return 0;
    }

    struct nf_conn *ct = (struct nf_conn *)*ct_ptr;

    // Clean up the pending entry regardless of success/failure
    bpf_map_delete_elem(&conntrack_args, &pid_tgid);

    // Only process if returned NF_ACCEPT (1)
    int retval = PT_REGS_RC(ctx);
    if (retval != 1) { // NF_ACCEPT = 1
        log_debug("kretprobe/__nf_conntrack_confirm: not NF_ACCEPT ct=%p ret=%d", ct, retval);
        return 0;
    }

    // Check IPS_CONFIRMED flag before adding to conntrack map
    u32 status = 0;
    BPF_CORE_READ_INTO(&status, ct, status);
    if (!(status & IPS_CONFIRMED)) {
        log_debug("kretprobe/__nf_conntrack_confirm: not IPS_CONFIRMED ct=%p status=%x", ct, status);
        return 0;
    }

    // Successfully confirmed NAT connection - add to conntrack map
    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        log_debug("kretprobe/__nf_conntrack_confirm: failed to extract tuples ct=%p", ct);
        return 0;
    }

    // Add both directions to conntrack map
    bpf_map_update_with_telemetry(conntrack, &orig, &reply, BPF_NOEXIST);
    bpf_map_update_with_telemetry(conntrack, &reply, &orig, BPF_NOEXIST);
    increment_telemetry_registers_count();

    return 0;
}

// JMWNEXT add kprobe for nf_conntrack_hash_check_insert
// JMWNEXT add kretprobe for nf_conntrack_hash_check_insert

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
