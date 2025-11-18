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

// JMWRM
/* // Primary probe: Track all conntrack insertions */
/* SEC("kprobe/__nf_conntrack_hash_insert") // JMWCONNTRACK */
/* int BPF_BYPASSABLE_KPROBE(kprobe__nf_conntrack_hash_insert, struct nf_conn *ct) { */
/*     increment_kprobe__nf_conntrack_hash_insert_entry_count(); */
/*     log_debug("JMW(runtime)kprobe__nf_conntrack_hash_insert: ct: %p netns: %u", ct, get_netns(ct)); */

/*     u32 status = 0; */
/*     BPF_CORE_READ_INTO(&status, ct, status); */
/*     // JMWWAS if (!(status&IPS_CONFIRMED) || !(status&IPS_NAT_MASK)) { */
/*     // JMW see https://github.com/DataDog/datadog-agent/pull/41848/files, */
/*     if (!(status&IPS_NAT_MASK)) { */
/*         return 0; */
/*     } */

/*     conntrack_tuple_t orig = {}, reply = {}; */
/*     if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) { */
/*         increment_nf_conntrack_hash_insert_failed_to_get_conntrack_tuples_count(); */
/*         return 0; */
/*     } */

/*     long ret1 = bpf_map_update_with_telemetry(conntrack, &orig, &reply, BPF_NOEXIST); */
/*     long ret2 = bpf_map_update_with_telemetry(conntrack, &reply, &orig, BPF_NOEXIST); */

/*     if (ret1 == -EEXIST) { */
/*         increment_nf_conntrack_hash_insert_regular_exists(); */
/*     } */
/*     if (ret2 == -EEXIST) { */
/*         increment_nf_conntrack_hash_insert_reverse_exists(); */
/*     } */

/*     // Only increment hash_insert_count if at least one entry was actually added */
/*     if (ret1 == 0 || ret2 == 0) { */
/*         increment_nf_conntrack_hash_insert_count(); */
/*         log_debug("JMW(runtime)kprobe__nf_conntrack_hash_insert: added to conntrack ct=%p", ct); */
/*     } */
/*     increment_telemetry_registers_count(); */

/*     return 0; */
/* } */

//JMWCOMMENT
// new probe: Track conntrack confirmations (entry) - correlation approach
// Entry probe: Store NAT connection info for correlation with return probe
SEC("kprobe/__nf_conntrack_confirm")
int BPF_BYPASSABLE_KPROBE(kprobe__nf_conntrack_confirm) {
    increment_kprobe__nf_conntrack_confirm_entry_count();
    log_debug("JMW(runtime)confirm: entry");
    // JMW update this kprobe and kretprobe to follow the pattern in /Users/jim.wilson/dd/datadog-agent/pkg/network/ebpf/c/tracer.c
    // kprobe/tcp_sendmsg/kretprobe__tcp_sendmsg and others
    struct sk_buff *skb = (struct sk_buff *)PT_REGS_PARM1(ctx); // skb is 1st parameter
    u64 pid_tgid = bpf_get_current_pid_tgid();

    if (!skb) {
        increment_kprobe__nf_conntrack_confirm_skb_null_count();
        return 0;
    }

    // Extract ct from skb using nf_ct_get()
    struct nf_conn *ct = NULL;
    // JMW what happens if we try to call nf_ct_get() from here?
    // Note: nf_ct_get() is typically inlined, so we need to access the skb fields directly (is get_netns() also typically inlined, we
    // use it above)
    // The conntrack info is stored in skb->_nfct
    u64 nfct = 0;
    BPF_CORE_READ_INTO(&nfct, skb, _nfct);
    if (!nfct) {
        increment_kprobe__nf_conntrack_confirm_nfct_null_count();
        return 0;
    }

    // Extract ct pointer from nfct (lower 3 bits contain ctinfo, upper bits contain ct pointer)
    // Standard Linux kernel mask is ~7UL to clear the lower 3 bits
    ct = (struct nf_conn *)(nfct & ~7UL);

    if (!ct) {
        increment_kprobe__nf_conntrack_confirm_ct_null_count();
        return 0;
    }

    u32 status = 0;
    BPF_CORE_READ_INTO(&status, ct, status);
    if (!(status&IPS_NAT_MASK)) {
        increment_kprobe__nf_conntrack_confirm_not_nat_count();
        log_debug("JMW(runtime)confirm: not IPS_NAT_MASK ct=%p status=%x", ct, status);
        return 0;
    }

    // Store ct pointer using pid_tgid for correlation with return probe
    u64 ct_ptr = (u64)ct;
    // JMWNAME nf_conntrack_confirm_args --> nf_conntrack_confirm_args
    bpf_map_update_with_telemetry(nf_conntrack_confirm_args, &pid_tgid, &ct_ptr, BPF_ANY);
    increment_kprobe__nf_conntrack_confirm_pending_added_count();
    log_debug("JMW(runtime)confirm: added to nf_conntrack_confirm_args: ct=%p pid_tgid=%llu", ct, pid_tgid);

    return 0;
}

// new probe: Track conntrack confirmations (return) - correlation approach
// Return probe: Process successful confirmations and populate conntrack map
SEC("kretprobe/__nf_conntrack_confirm") // JMWCONNTRACK
int BPF_BYPASSABLE_KPROBE(kretprobe__nf_conntrack_confirm) {
    increment_kretprobe__nf_conntrack_confirm_entry_count();
    log_debug("JMW(runtime)kretprobe__nf_conntrack_confirm: entry");
    int ret = PT_REGS_RC(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();

    // Look up the ct pointer from entry probe
    u64 *ct_ptr = bpf_map_lookup_elem(&nf_conntrack_confirm_args, &pid_tgid);
    if (!ct_ptr) {
        // No matching entry probe - this can happen if entry was filtered out JMWWHAT
        increment_kretprobe__nf_conntrack_confirm_no_matching_entry_probe_count();
        log_debug("JMW(runtime)kretprobe__nf_conntrack_confirm: no matching nf_conntrack_confirm_args entry, pid_tgid: %llu", pid_tgid);
        return 0;
    }

    struct nf_conn *ct = (struct nf_conn *)*ct_ptr;

    // Clean up the pending entry regardless of success/failure
    bpf_map_delete_elem(&nf_conntrack_confirm_args, &pid_tgid);

    // Only process if returned NF_ACCEPT (1)
    if (ret != 1) { // NF_ACCEPT = 1
        increment_kretprobe__nf_conntrack_confirm_not_accepted_count();
        log_debug("JMW(runtime)kretprobe__nf_conntrack_confirm: not NF_ACCEPT ct=%p ret=%d", ct, ret);
        return 0;
    }

    // JMW check flags before adding to conntrack map
    // Similar to kprobe__nf_conntrack_hash_insert
    u32 status = 0;
    BPF_CORE_READ_INTO(&status, ct, status);
    if (!(status&IPS_CONFIRMED)) {
        increment_kretprobe__nf_conntrack_confirm_not_confirmed_count();
        log_debug("JMW(runtime)kretprobe__nf_conntrack_confirm: not IPS_CONFIRMED ct=%p status=%x", ct, status);
        return 0;
    }

    // Successfully confirmed NAT connection - add to conntrack map
    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        increment_kretprobe__nf_conntrack_confirm_failed_to_get_conntrack_tuples_count();
        log_debug("JMW(runtime)kretprobe__nf_conntrack_confirm: failed_tuples ct=%p", ct);
        return 0;
    }

    // Add both directions to conntrack map
    //JMW
    long ret1 = bpf_map_update_with_telemetry(conntrack, &orig, &reply, BPF_NOEXIST);
    long ret2 = bpf_map_update_with_telemetry(conntrack, &reply, &orig, BPF_NOEXIST);
    
    if (ret1 == -EEXIST) {
        increment_kretprobe__nf_conntrack_confirm_regular_exists();
    }
    if (ret2 == -EEXIST) {
        increment_kretprobe__nf_conntrack_confirm_reverse_exists();
    }
    
    // Only increment success_count if at least one entry was actually added
    if (ret1 == 0 || ret2 == 0) {
        increment_kretprobe__nf_conntrack_confirm_success_count();
        log_debug("JMW(runtime)kretprobe__nf_conntrack_confirm: added ct=%p", ct);
    }

    return 0;
}

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
        increment_kprobe_ctnetlink_fill_info_failed_to_get_conntrack_tuples_count();
        return 0;
    }

    long ret1 = bpf_map_update_with_telemetry(conntrack, &orig, &reply, BPF_NOEXIST);
    long ret2 = bpf_map_update_with_telemetry(conntrack, &reply, &orig, BPF_NOEXIST);

    if (ret1 == -EEXIST) {
        increment_kprobe_ctnetlink_fill_info_regular_exists_count();
    }
    if (ret2 == -EEXIST) {
        increment_kprobe_ctnetlink_fill_info_reverse_exists_count();
    }

    // Only increment hash_insert_count if at least one entry was actually added
    if (ret1 == 0 || ret2 == 0) {
        increment_kprobe_ctnetlink_fill_info_added_count();
    }
    increment_telemetry_registers_count();

    return 0;
}

char _license[] SEC("license") = "GPL";
