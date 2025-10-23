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

// Third probe: Track conntrack confirmations (entry) - simplified approach
// Entry probe: Mark NAT connections
SEC("kprobe/__nf_conntrack_confirm") // JMWCONNTRACK
int BPF_BYPASSABLE_KPROBE(kprobe__nf_conntrack_confirm, struct pt_regs *ctx) {
    increment_confirm_entry_count();
    struct sk_buff *skb = (struct sk_buff *)PT_REGS_PARM1(ctx);
    struct nf_conn *ct;
    enum ip_conntrack_info ctinfo;
    u64 ct_ptr;
    u8 val = 1;
    
    ct = nf_ct_get(skb, &ctinfo);
    if (!ct)
        return 0;
    
    log_debug("kprobe/__nf_conntrack_confirm: netns: %u, status: %x", get_netns(ct), status);
    log_debug("JMWTEST runtime kprobe/__nf_conntrack_confirm entry");

    // Filter: Only track NAT connections
    u32 status = 0;
    BPF_CORE_READ_INTO(&status, ct, status);
    if (!(status & IPS_NAT_MASK))
        return 0;
    
    // Store ct pointer temporarily
    ct_ptr = (u64)ct;
    bpf_map_update_with_telemetry(pending_confirms, &ct_ptr, &val, BPF_ANY);
    
    return 0;
}
    
// Third probe: Track conntrack confirmations (return) - simplified approach
// Return probe: Verify success and count
SEC("kretprobe/__nf_conntrack_confirm") // JMWCONNTRACK
int BPF_BYPASSABLE_KPROBE(kretprobe__nf_conntrack_confirm, struct pt_regs *ctx) {
    increment_confirm_return_count();
    int ret = PT_REGS_RC(ctx);
    u64 ct_ptr;
    
    log_debug("kretprobe/__nf_conntrack_confirm: ret=%d", ret);
    log_debug("JMWTEST runtime kretprobe/__nf_conntrack_confirm success");

    // Get the ct pointer (need to track this from entry)
    // This is the tricky part - need to correlate entry/exit
    
    // Only count if returned NF_ACCEPT
    if (ret != NF_ACCEPT) {
        increment_confirm_return_failed_count();
        goto cleanup;
    }
    
    increment_confirm_return_success_count();
    
    // Check if this was a NAT connection we tracked
    if (!bpf_map_lookup_elem(&pending_confirms, &ct_ptr))
        goto cleanup;
    
    // Successfully confirmed NAT connection!
    track_nat_connection_confirmed(ct);
    
cleanup:
    bpf_map_delete_elem(&pending_confirms, &ct_ptr);
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
