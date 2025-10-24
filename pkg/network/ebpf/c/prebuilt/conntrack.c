#include "kconfig.h"
#include <linux/version.h>
#include "bpf_metadata.h"

#include "bpf_helpers.h"
#include "bpf_endian.h"
#include "bpf_telemetry.h"
#include "bpf_bypass.h"

#include "offsets.h"
#include "conntrack.h"
#include "conntrack/maps.h"
#include "ip.h"
#include "ipv6.h"
#include "pid_tgid.h"

// Primary probe: Track all conntrack insertions
SEC("kprobe/__nf_conntrack_hash_insert") // JMWCONNTRACK
int BPF_BYPASSABLE_KPROBE(kprobe__nf_conntrack_hash_insert, struct nf_conn *ct) {
    increment_hash_insert_count();
    log_debug("kprobe/__nf_conntrack_hash_insert: netns: %u", get_netns(ct));
    log_debug("JMWTEST prebuilt kprobe/__nf_conntrack_hash_insert");

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
    log_debug("kprobe/nf_nat_packet: netns: %u", get_netns(ct));
    log_debug("JMWTEST prebuilt kprobe/nf_nat_packet");

    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        return 0;
    }
    RETURN_IF_NOT_NAT(&orig, &reply);

    bpf_map_update_with_telemetry(conntrack2, &orig, &reply, BPF_ANY);
    bpf_map_update_with_telemetry(conntrack2, &reply, &orig, BPF_ANY);
    increment_telemetry_registers_count();

    return 0;
}

// Third probe: Track confirmed NAT connections (entry)
SEC("kprobe/__nf_conntrack_confirm") // JMWCONNTRACK
int BPF_BYPASSABLE_KPROBE(kprobe__nf_conntrack_confirm) {
    increment_confirm_entry_count();
    struct nf_conn *ct = (struct nf_conn *)PT_REGS_PARM2(ctx);
    u64 ct_ptr;
    u8 val = 1;

    if (!ct)
        return 0;

    log_debug("kprobe/__nf_conntrack_confirm: netns: %u", get_netns(ct));
    log_debug("JMWTEST prebuilt kprobe/__nf_conntrack_confirm entry");

    // Filter: Only track NAT connections
    u32 status = 0;
    bpf_probe_read_kernel(&status, sizeof(status), &ct->status);
    if (!(status & IPS_NAT_MASK))
        return 0;

    // Store ct pointer temporarily for correlation with return probe
    ct_ptr = (u64)ct;
    bpf_map_update_with_telemetry(pending_confirms, &ct_ptr, &val, BPF_ANY);

    return 0;
}

// Fourth probe: Track confirmed NAT connections (return)
SEC("kretprobe/__nf_conntrack_confirm") // JMWCONNTRACK
int BPF_BYPASSABLE_KPROBE(kretprobe__nf_conntrack_confirm) {
    increment_confirm_return_count();
    int ret = PT_REGS_RC(ctx);
    u64 ct_ptr;
    u8 *pending_val;

    log_debug("kretprobe/__nf_conntrack_confirm: ret=%d", ret);
    log_debug("JMWTEST prebuilt kretprobe/__nf_conntrack_confirm");

    // Only process if returned NF_ACCEPT (1)
    if (ret != 1) { // NF_ACCEPT = 1
        increment_confirm_return_failed_count();
        return 0;
    }

    increment_confirm_return_success_count();

    // For prebuilt version, we can't easily correlate entry/exit
    // So we'll just count successful returns
    // The actual conntrack entry population would need the ct pointer
    // which is challenging to get in the return probe without correlation

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

    log_debug("kprobe/ctnetlink_fill_info: netns: %u", get_netns(ct));

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

char _license[] SEC("license") = "GPL";
