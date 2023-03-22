#ifndef __CONNTRACK_H
#define __CONNTRACK_H

#include <net/netfilter/nf_conntrack.h>
#include <linux/types.h>
#include <linux/sched.h>
#include "bpf_builtins.h"
#include "tracer.h"
#include "conntrack-types.h"
#include "conntrack-maps.h"
#include "ip.h"
#include "netns.h"

#include "conntrack-helpers.h"

static __always_inline u32 ct_status(const struct nf_conn *ct) {
    u32 status = 0;
    bpf_probe_read_kernel_with_telemetry(&status, sizeof(status), (void *)&ct->status);
    return status;
}

static __always_inline int nf_conn_to_conntrack_tuples(struct nf_conn* ct, conntrack_tuple_t* orig, conntrack_tuple_t* reply) {
    struct nf_conntrack_tuple_hash tuplehash[IP_CT_DIR_MAX];
    bpf_memset(tuplehash, 0, sizeof(tuplehash));
    bpf_probe_read_kernel_with_telemetry(&tuplehash, sizeof(tuplehash), &ct->tuplehash);

    struct nf_conntrack_tuple orig_tup = tuplehash[IP_CT_DIR_ORIGINAL].tuple;
    struct nf_conntrack_tuple reply_tup = tuplehash[IP_CT_DIR_REPLY].tuple;

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

#endif
