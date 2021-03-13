#include <linux/kconfig.h>
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

    struct nf_conntrack_tuple_hash tuplehash[IP_CT_DIR_MAX];
    __builtin_memset(tuplehash, 0, sizeof(tuplehash));
    bpf_probe_read(&tuplehash, sizeof(tuplehash), &ct->tuplehash);

    struct nf_conntrack_tuple orig = tuplehash[IP_CT_DIR_ORIGINAL].tuple;
    struct nf_conntrack_tuple reply = tuplehash[IP_CT_DIR_REPLY].tuple;

    u32 netns = get_netns(&ct->ct_net);
    log_debug("kprobe/__nf_conntrack_hash_insert: netns: %u, status: %x\n", netns, status);

    conn_tuple_t orig_conn = {};
    if (!conntrack_tuple_to_conn_tuple(&orig_conn, &orig)) {
        return 0;
    }
    orig_conn.netns = netns;

    log_debug("orig\n");
    print_translation(&orig_conn);

    conn_tuple_t reply_conn = {};
    if (!conntrack_tuple_to_conn_tuple(&reply_conn, &reply)) {
        return 0;
    }
    reply_conn.netns = netns;

    log_debug("reply\n");
    print_translation(&reply_conn);

    bpf_map_update_elem(&conntrack, &orig_conn, &reply_conn, BPF_ANY);
    bpf_map_update_elem(&conntrack, &reply_conn, &orig_conn, BPF_ANY);
    increment_telemetry_count(registers);

    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
