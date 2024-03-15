#ifndef __CONNTRACK_H
#define __CONNTRACK_H

#include <net/netfilter/nf_conntrack.h>
#include <linux/types.h>
#include <linux/sched.h>

#include "bpf_builtins.h"

#include "ip.h"
#include "ipv6.h"
#include "sock.h"

#include "conntrack/types.h"
#include "conntrack/maps.h"
#include "conntrack/helpers.h"


#define offset_ct(f) \
    static __always_inline u64 offset_ct_##f() { \
        __u64 val = 0;                           \
        LOAD_CONSTANT("offset_ct_" #f, val);     \
        return val;                              \
    }

offset_ct(origin)
offset_ct(reply)
offset_ct(status)
offset_ct(netns)
offset_ct(ino)

#define RETURN_IF_NOT_NAT(orig, reply)                      \
    if (!is_conn_nat(orig, reply)) {                        \
        return 0;                                           \
    }

static __always_inline bool is_conn_nat(const conntrack_tuple_t* orig, const conntrack_tuple_t* reply) {
    return orig->daddr_l != reply->saddr_l || orig->dport != reply->sport || 
        orig->saddr_l != reply->daddr_l || orig->sport != reply->dport || 
        orig->daddr_h != reply->saddr_h;
}

static __always_inline u32 get_netns(struct nf_conn *ct) {
    void* ct_net = NULL;
    u32 net_ns_inum = 0;
    bpf_probe_read_kernel_with_telemetry(&ct_net, sizeof(void*), ((char*)ct) + offset_ct_netns());
    bpf_probe_read_kernel_with_telemetry(&net_ns_inum, sizeof(net_ns_inum), ((char*)ct_net) + offset_ct_ino());
    return net_ns_inum;
}

static __always_inline int nf_conn_to_conntrack_tuples(struct nf_conn* ct, conntrack_tuple_t* orig, conntrack_tuple_t* reply) {
    struct nf_conntrack_tuple orig_tup = {};
    bpf_probe_read_kernel_with_telemetry(&orig_tup, sizeof(orig_tup), (char*)ct + offset_ct_origin());
    struct nf_conntrack_tuple reply_tup = {};
    bpf_probe_read_kernel_with_telemetry(&reply_tup, sizeof(reply_tup), (char*)ct + offset_ct_reply());

    u32 netns = get_netns(ct);

    if (!nf_conntrack_tuple_to_conntrack_tuple(orig, &orig_tup)) {
        return 1;
    }
    orig->netns = netns;

    log_debug("orig");
    print_translation(orig);

    if (!nf_conntrack_tuple_to_conntrack_tuple(reply, &reply_tup)) {
        return 1;
    }
    reply->netns = netns;

    log_debug("reply");
    print_translation(reply);

    return 0;
}

#endif
