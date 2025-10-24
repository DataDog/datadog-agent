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

// Primary probe: Track all conntrack insertions
SEC("kprobe/__nf_conntrack_hash_insert") // JMWCONNTRACK
int BPF_BYPASSABLE_KPROBE(kprobe__nf_conntrack_hash_insert, struct nf_conn *ct) {
    increment_hash_insert_count();
    log_debug("kprobe/__nf_conntrack_hash_insert: netns: %u", get_netns(ct));
    log_debug("JMWTEST runtime kprobe/__nf_conntrack_hash_insert");

    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        return 0;
    }
    // Note: For hash_insert, we track all connections, not just NAT
    // The NAT filtering happens in the other probes

    bpf_map_update_with_telemetry(conntrack, &orig, &reply, BPF_ANY);
    bpf_map_update_with_telemetry(conntrack, &reply, &orig, BPF_ANY);
    increment_telemetry_registers_count();

    return 0;
}

// Second probe: Track NAT packet processing
SEC("kprobe/nf_nat_packet") // JMWCONNTRACK
int BPF_BYPASSABLE_KPROBE(kprobe_nf_nat_packet, struct nf_conn *ct) {
    increment_nat_packet_count();
    u32 status = 0;
    BPF_CORE_READ_INTO(&status, ct, status);
    if (!(status&IPS_NAT_MASK)) {
        return 0;
    }

    log_debug("kprobe/nf_nat_packet: netns: %u, status: %x", get_netns(ct), status);
    log_debug("JMWTEST runtime kprobe/nf_nat_packet");

    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        return 0;
    }

    bpf_map_update_with_telemetry(conntrack2, &orig, &reply, BPF_ANY);
    bpf_map_update_with_telemetry(conntrack2, &reply, &orig, BPF_ANY);
    increment_telemetry_registers_count();

    return 0;
}

// Approach 1: Use per-CPU correlation (most robust)
// Create a per-CPU map to store the ct pointer, then retrieve it in the kretprobe.
// Approach 2: Iterate pending_confirms (simpler but less efficient)
// In the kretprobe, iterate through pending_confirms to find and process entries.
// Approach 3: Use pid_tgid correlation (simplest)
// Use pid_tgid as correlation key (assuming single-threaded conntrack processing).
// Let me implement Approach 3 first since it's the simplest and matches the pattern used elsewhere in the codebase:

// Third probe: Track conntrack confirmations (entry) - correlation approach
// Entry probe: Store NAT connection info for correlation with return probe
SEC("kprobe/__nf_conntrack_confirm") // JMWCONNTRACK
int BPF_BYPASSABLE_KPROBE(kprobe__nf_conntrack_confirm) {
    increment_confirm_entry_count();
    struct nf_conn *ct = (struct nf_conn *)PT_REGS_PARM2(ctx); // ct is 2nd parameter
    u64 pid_tgid = bpf_get_current_pid_tgid();
    
    if (!ct)
        return 0;
    
    // Filter: Only track NAT connections
    u32 status = 0;
    BPF_CORE_READ_INTO(&status, ct, status);
    if (!(status & IPS_NAT_MASK))
        return 0;
    
    log_debug("kprobe/__nf_conntrack_confirm: netns: %u, status: %x", get_netns(ct), status);
    log_debug("JMWTEST runtime kprobe/__nf_conntrack_confirm entry");
    
    // Store ct pointer using pid_tgid for correlation with return probe
    u64 ct_ptr = (u64)ct;
    bpf_map_update_with_telemetry(pending_confirms, &pid_tgid, &ct_ptr, BPF_ANY);
    
    return 0;
}
    
// Third probe: Track conntrack confirmations (return) - correlation approach  
// Return probe: Process successful confirmations and populate conntrack3 map
SEC("kretprobe/__nf_conntrack_confirm") // JMWCONNTRACK
int BPF_BYPASSABLE_KPROBE(kretprobe__nf_conntrack_confirm) {
    increment_confirm_return_count();
    int ret = PT_REGS_RC(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    
    log_debug("kretprobe/__nf_conntrack_confirm: ret=%d", ret);
    log_debug("JMWTEST runtime kretprobe/__nf_conntrack_confirm");

    // Look up the ct pointer from entry probe
    u64 *ct_ptr = bpf_map_lookup_elem(&pending_confirms, &pid_tgid);
    if (!ct_ptr) {
        // No matching entry probe - this can happen if entry was filtered out
        return 0;
    }
    
    struct nf_conn *ct = (struct nf_conn *)*ct_ptr;
    
    // Clean up the pending entry regardless of success/failure
    bpf_map_delete_elem(&pending_confirms, &pid_tgid);
    
    // Only process if returned NF_ACCEPT (1)
    if (ret != 1) { // NF_ACCEPT = 1
        increment_confirm_return_failed_count();
        return 0;
    }
    
    increment_confirm_return_success_count();
    
    // Successfully confirmed NAT connection - add to conntrack3 map
    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        return 0;
    }
    
    // Add both directions to conntrack3 map
    bpf_map_update_with_telemetry(conntrack3, &orig, &reply, BPF_ANY);
    bpf_map_update_with_telemetry(conntrack3, &reply, &orig, BPF_ANY);
    increment_telemetry_registers_count();
    
    log_debug("kretprobe/__nf_conntrack_confirm: added to conntrack3, netns: %u", get_netns(ct));
    
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
        return 0;
    }

    bpf_map_update_with_telemetry(conntrack, &orig, &reply, BPF_ANY);
    bpf_map_update_with_telemetry(conntrack, &reply, &orig, BPF_ANY);
    increment_telemetry_registers_count();

    return 0;
}

char _license[] SEC("license") = "GPL";
