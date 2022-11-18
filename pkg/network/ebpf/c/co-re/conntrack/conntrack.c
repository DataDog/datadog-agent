#define DEBUG 1
#include "conntrack.h"

static __always_inline int nf_conn_to_conntrack_tuples(struct nf_conn* ct, conntrack_tuple_t* orig, conntrack_tuple_t* reply) {
    struct nf_conntrack_tuple_hash tuplehash[IP_CT_DIR_MAX];
    memset(tuplehash, 0, sizeof(tuplehash));
    bpf_core_read(&tuplehash, sizeof(tuplehash), &ct->tuplehash);

    struct nf_conntrack_tuple orig_tup = BPF_CORE_READ(&tuplehash[IP_CT_DIR_ORIGINAL], tuple);
    struct nf_conntrack_tuple reply_tup = BPF_CORE_READ(&tuplehash[IP_CT_DIR_REPLY], tuple);

    u32 netns = get_netns(&ct->ct_net);

    if (!nf_conntrack_tuple_to_conntrack_tuple(orig, &orig_tup)) {
        return 1;
    }
    orig->netns = netns;

    log_debug("orig\n");
    print_translation(orig);

    if (!nf_conntrack_tuple_to_conntrack_tuple(reply, &reply_tup)) {
        return 1;
    }
    reply->netns = netns;

    log_debug("reply\n");
    print_translation(reply);

    return 0;
}

SEC("kprobe/__nf_conntrack_hash_insert")
int BPF_KPROBE(kprobe___nf_conntrack_hash_insert, struct nf_conn *ct,unsigned int hash, unsigned int reply_hash) {
    u32 status = ct_status(ct);
    if (!(status&IPS_CONFIRMED)) {
        log_debug("kprobe/__nf_conntrack_hash_insert include IPS_CONFIRMED: netns: %u, status: %x\n", get_netns(&ct->ct_net), status);
        return 0;
    }
    if (!(status&IPS_NAT_MASK)) {
        log_debug("kprobe/__nf_conntrack_hash_insert include IPS_NAT_MASK: netns: %u, status: %x\n", get_netns(&ct->ct_net), status);
        return 0;
    }
    if (!(status&IPS_CONFIRMED) || !(status&IPS_NAT_MASK)) {
        log_debug("kprobe/filter: netns: %u, status: %x\n", get_netns(&ct->ct_net), status);
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

SEC("kprobe/ctnetlink_fill_info")
int BPF_KPROBE(kprobe_ctnetlink_fill_info, struct nf_conn *ct) {

    proc_t proc = {};
    bpf_get_current_comm(&proc.comm, sizeof(proc.comm));

    if (!proc_t_comm_prefix_equals("system-probe", 12, proc)) {
        log_debug("skipping kprobe/ctnetlink_fill_info invocation from non-system-probe process\n");
        return 0;
    }

    u32 status = ct_status(ct);
    if (!(status&IPS_CONFIRMED) || !(status&IPS_NAT_MASK)) {
        return 0;
    }

    __maybe_unused possible_net_t c_net = BPF_CORE_READ(ct, ct_net);
    log_debug("kprobe/ctnetlink_fill_info: netns: %u, status: %x\n", get_netns(&c_net), status);

    conntrack_tuple_t orig = {}, reply = {};
    if (nf_conn_to_conntrack_tuples(ct, &orig, &reply) != 0) {
        return 0;
    }

    bpf_map_update_with_telemetry(conntrack, &orig, &reply, BPF_ANY);
    bpf_map_update_with_telemetry(conntrack, &reply, &orig, BPF_ANY);
    increment_telemetry_registers_count();

    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
