#include "kconfig.h"
#include <linux/version.h>
#include <linux/types.h>

#include "bpf_helpers.h"
#include "bpf_endian.h"
#include "conntrack.h"
#include "conntrack-maps.h"
#include "netns.h"
#include "ip.h"

#ifdef FEATURE_IPV6_ENABLED
#include "ipv6.h"
#endif

#ifndef LINUX_VERSION_CODE
# error "kernel version not included?"
#endif

SEC("kprobe/__nf_conntrack_hash_insert")
int kprobe___nf_conntrack_hash_insert(struct pt_regs* ctx) {
    struct nf_conn *ct = (struct nf_conn*)PT_REGS_PARM1(ctx);

    u32 status = ct_status(ct);
    if (!(status&IPS_CONFIRMED)) {
        return 0;
    }
    if (!(status&IPS_NAT_MASK)) {
        increment_telemetry_count(registers_dropped);
        return 0;
    }
    
    log_debug("kprobe/__nf_conntrack_hash_insert: netns: %u, status: %x\n", get_netns(&ct->ct_net), status);

    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        return 0;
    }

    bpf_map_update_elem(&conntrack, &orig, &reply, BPF_ANY);
    bpf_map_update_elem(&conntrack, &reply, &orig, BPF_ANY);
    increment_telemetry_count(registers);

    return 0;
}

SEC("kprobe/ctnetlink_fill_info")
int kprobe_ctnetlink_fill_info(struct pt_regs* ctx) {
    struct nf_conn *ct = (struct nf_conn*)PT_REGS_PARM5(ctx);

    u32 status = ct_status(ct);
    if (!(status&IPS_CONFIRMED)) {
        return 0;
    }
    if (!(status&IPS_NAT_MASK)) {
        increment_telemetry_count(registers_dropped);
        return 0;
    }
    
    log_debug("kprobe/ctnetlink_fill_info: netns: %u, status: %x\n", get_netns(&ct->ct_net), status);

    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        return 0;
    }

    bpf_map_update_elem(&conntrack, &orig, &reply, BPF_ANY);
    bpf_map_update_elem(&conntrack, &reply, &orig, BPF_ANY);
    increment_telemetry_count(registers);

    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)