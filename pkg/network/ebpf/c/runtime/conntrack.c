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

static __always_inline int kprobe_conntrack_common(struct nf_conn *ct) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("JMW kprobe_conntrack_common: pid_tgid: %llu ct=%p", pid_tgid, ct);

    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        return 0;
    }
    RETURN_IF_NOT_NAT(&orig, &reply);

    bpf_map_update_with_telemetry(conntrack_args, &pid_tgid, &ct, BPF_ANY);
    log_debug("JMW kprobe_conntrack_common: added to map pid_tgid=%llu ct=%p", pid_tgid, ct);

    return 0;
}

SEC("kprobe/__nf_conntrack_hash_insert")
int BPF_BYPASSABLE_KPROBE(kprobe___nf_conntrack_hash_insert, struct nf_conn *ct) {
    log_debug("kprobe/__nf_conntrack_hash_insert: netns: %u", get_netns(ct)); // JMW why do we log netns?  should anything else be logged?

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

SEC("kprobe/__nf_conntrack_confirm")
int BPF_BYPASSABLE_KPROBE(kprobe___nf_conntrack_confirm, struct sk_buff *skb) {
    struct nf_conn *ct = get_nfct(skb);
    if (!ct) {
        return 0;
    }
    log_debug("kprobe/__nf_conntrack_confirm: netns: %u", get_netns(ct)); // JMW why do we log netns?  should anything else be logged?

    kprobe_conntrack_common(ct);
    return 0;
}

SEC("kretprobe/__nf_conntrack_confirm")
int BPF_BYPASSABLE_KPROBE(kretprobe___nf_conntrack_confirm) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct nf_conn **ctpp = (struct nf_conn **)bpf_map_lookup_elem(&conntrack_args, &pid_tgid);
    if (!ctpp) {
        return 0;
    }

    bpf_map_delete_elem(&conntrack_args, &pid_tgid);

    struct nf_conn *ct = *ctpp;
    if (!ct) {
        log_debug("kretprobe/__nf_conntrack_confirm: ct pointer missing for pid_tgid=%llu", pid_tgid); // JMWRM?
        return 0;
    }

    // Only process if returned NF_ACCEPT (1)
    int retval = PT_REGS_RC(ctx);
    if (retval != 1) { // NF_ACCEPT = 1
        log_debug("kretprobe/__nf_conntrack_confirm: not NF_ACCEPT ct=%p ret=%d", ct, retval); // JMWRM?
        return 0;
    }

    // Check IPS_CONFIRMED flag before adding to conntrack map // JMWNEXT
    u32 status = 0;
    BPF_CORE_READ_INTO(&status, ct, status);
    if (!(status & IPS_CONFIRMED)) {
        log_debug("kretprobe/__nf_conntrack_confirm: not IPS_CONFIRMED ct=%p status=%x", ct, status); // JMWRM?
        return 0;
    }

    // JMW from here down - common code for two new probes - handle_conntrack_map_update()?
    // Successfully confirmed NAT connection - add to conntrack map
    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        log_debug("kretprobe/__nf_conntrack_confirm: failed to extract tuples ct=%p", ct); // JMWRM?
        return 0;
    }

    // Add both directions to conntrack map
    bpf_map_update_with_telemetry(conntrack, &orig, &reply, BPF_ANY);
    bpf_map_update_with_telemetry(conntrack, &reply, &orig, BPF_ANY);
    increment_telemetry_registers_count(); // JMW what is this, do we need separate counters for new probes?

    return 0;
}

SEC("kprobe/nf_conntrack_hash_check_insert")
int BPF_BYPASSABLE_KPROBE(kprobe_nf_conntrack_hash_check_insert, struct nf_conn *ct) {
    if (!ct) {
        return 0;
    }
    log_debug("kprobe/nf_conntrack_hash_check_insert: netns: %u", get_netns(ct)); // JMW why do we log netns?  should anything else be logged?

    kprobe_conntrack_common(ct);
    return 0;
}

//JMWRM
// Return probe for nf_conntrack_hash_check_insert
// Only update conntrack map if return value is 0 (success)
SEC("kretprobe/nf_conntrack_hash_check_insert")
int BPF_BYPASSABLE_KPROBE(kretprobe_nf_conntrack_hash_check_insert) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct nf_conn **ctpp = (struct nf_conn **)bpf_map_lookup_elem(&conntrack_args, &pid_tgid);
    if (!ctpp) {
        return 0;
    }

    bpf_map_delete_elem(&conntrack_args, &pid_tgid);

    struct nf_conn *ct = *ctpp;
    if (!ct) {
        log_debug("kretprobe/nf_conntrack_hash_check_insert: ct pointer missing for pid_tgid=%llu", pid_tgid);
        return 0;
    }

    // Only process if returned 0 (success)
    int retval = PT_REGS_RC(ctx);
    if (retval != 0) {
        log_debug("kretprobe/nf_conntrack_hash_check_insert: failed ct=%p ret=%d", ct, retval);
        return 0;
    }

    // Successfully inserted - add to conntrack map
    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        log_debug("kretprobe/nf_conntrack_hash_check_insert: failed to extract tuples ct=%p", ct);
        return 0;
    }

    // Add both directions to conntrack map
    bpf_map_update_with_telemetry(conntrack, &orig, &reply, BPF_ANY);
    bpf_map_update_with_telemetry(conntrack, &reply, &orig, BPF_ANY);
    increment_telemetry_registers_count(); // JMW same one ok?

    log_debug("kretprobe/nf_conntrack_hash_check_insert: added to conntrack map ct=%p", ct);

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
