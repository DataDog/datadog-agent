#ifndef _HOOKS_NETWORK_FLOW_H_
#define _HOOKS_NETWORK_FLOW_H_

#include "constants/offsets/network.h"
#include "constants/offsets/netns.h"
#include "helpers/network.h"

HOOK_ENTRY("security_sk_classify_flow")
int hook_security_sk_classify_flow(ctx_t *ctx) {
    struct sock *sk = (struct sock *)CTX_PARM1(ctx);
    struct flowi *fl = (struct flowi *)CTX_PARM2(ctx);
    struct pid_route_t key = {};
    union flowi_uli uli;

    u16 family = get_family_from_sock_common((void *)sk);
    if (family == AF_INET6) {
        bpf_probe_read(&key.addr, sizeof(u64) * 2, (void *)fl + get_flowi6_saddr_offset());
        bpf_probe_read(&uli, sizeof(uli), (void *)fl + get_flowi6_uli_offset());
        bpf_probe_read(&key.port, sizeof(key.port), &uli.ports.sport);
    } else if (family == AF_INET) {
        bpf_probe_read(&key.addr, sizeof(u32), (void *)fl + get_flowi4_saddr_offset());
        bpf_probe_read(&uli, sizeof(uli), (void *)fl + get_flowi4_uli_offset());
        bpf_probe_read(&key.port, sizeof(key.port), &uli.ports.sport);
    } else {
        return 0;
    }

    // Register service PID
    if (key.port != 0) {
        u64 id = bpf_get_current_pid_tgid();
        u32 tid = (u32)id;
        u32 pid = id >> 32;

        // add netns information
        key.netns = get_netns_from_sock(sk);
        if (key.netns != 0) {
            bpf_map_update_elem(&netns_cache, &tid, &key.netns, BPF_ANY);
        }

        bpf_map_update_elem(&flow_pid, &key, &pid, BPF_ANY);

#ifdef DEBUG
        bpf_printk("# registered (flow) pid:%d netns:%u", pid, key.netns);
        bpf_printk("# p:%d a:%d a:%d", key.port, key.addr[0], key.addr[1]);
#endif
    }
    return 0;
}

__attribute__((always_inline)) int trace_nat_manip_pkt(struct nf_conn *ct) {
    u32 netns = get_netns_from_nf_conn(ct);

    struct nf_conntrack_tuple_hash tuplehash[IP_CT_DIR_MAX];
    bpf_probe_read(&tuplehash, sizeof(tuplehash), &ct->tuplehash);

    struct nf_conntrack_tuple *orig_tuple = &tuplehash[IP_CT_DIR_ORIGINAL].tuple;
    struct nf_conntrack_tuple *reply_tuple = &tuplehash[IP_CT_DIR_REPLY].tuple;

    // parse nat flows
    struct namespaced_flow_t orig = {
        .netns = netns,
    };
    struct namespaced_flow_t reply = {
        .netns = netns,
    };
    parse_tuple(orig_tuple, &orig.flow);
    parse_tuple(reply_tuple, &reply.flow);

    // save nat translation:
    //   - flip(reply) should be mapped to orig
    //   - reply should be mapped to flip(orig)
    flip(&reply.flow);
    bpf_map_update_elem(&conntrack, &reply, &orig, BPF_ANY);
    flip(&reply.flow);
    flip(&orig.flow);
    bpf_map_update_elem(&conntrack, &reply, &orig, BPF_ANY);
    return 0;
}

HOOK_ENTRY("nf_nat_manip_pkt")
int hook_nf_nat_manip_pkt(ctx_t *ctx) {
    struct nf_conn *ct = (struct nf_conn *)CTX_PARM2(ctx);
    return trace_nat_manip_pkt(ct);
}

HOOK_ENTRY("nf_nat_packet")
int hook_nf_nat_packet(ctx_t *ctx) {
    struct nf_conn *ct = (struct nf_conn *)CTX_PARM1(ctx);
    return trace_nat_manip_pkt(ct);
}

#endif
