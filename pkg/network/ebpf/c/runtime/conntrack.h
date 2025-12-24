#ifndef __CONNTRACK_H
#define __CONNTRACK_H

#ifdef COMPILE_RUNTIME
#include <net/netfilter/nf_conntrack.h>
#include <linux/types.h>
#include <linux/sched.h>
#include <linux/skbuff.h>
#include <linux/version.h>
#endif

#include "bpf_core_read.h"
#include "bpf_builtins.h"
#include "bpf_telemetry.h"
#include "conn_tuple.h"
#include "ip.h"
#include "netns.h"

#include "conntrack/types.h"
#include "conntrack/maps.h"
#include "conntrack/helpers.h"

static __always_inline bool is_conn_nat(const conntrack_tuple_t* orig, const conntrack_tuple_t* reply) {
    return orig->daddr_l != reply->saddr_l || orig->dport != reply->sport ||
        orig->saddr_l != reply->daddr_l || orig->sport != reply->dport ||
        orig->daddr_h != reply->saddr_h;
}

// JMWREVIEW
// CO-RE: Define old sk_buff structure for kernels < 4.7 where field was named 'nfct' instead of '_nfct'
// The field was renamed in kernel 4.7 to discourage direct access.
// RHEL7 (kernel 3.10) may or may not have backported this change.
#ifdef COMPILE_CORE
// JMW what's this for and how does it work?
struct sk_buff___nfct_old {
    unsigned long nfct;
};
#endif

// get_nfct extracts the nf_conn pointer from an sk_buff.
// The conntrack info is stored in skb->_nfct (or skb->nfct on older kernels).
// The lower 3 bits contain ctinfo, upper bits contain the nf_conn pointer.
// This function handles kernel version differences:
// JMW 
// - Kernel >= 4.7: field is named '_nfct'
// - Kernel < 4.7 (including potentially RHEL7 3.10): field is named 'nfct'
static __always_inline struct nf_conn *get_nfct(struct sk_buff *skb) {
    u64 nfct = 0;

#ifdef COMPILE_RUNTIME
    // Runtime compilation: kernel headers determine which field exists.
    // For kernels >= 4.7, _nfct exists. For older kernels, nfct exists.
    // Since minimum supported kernel for conntracker is 4.14, _nfct should always exist. // JMW???
    // However, RHEL7 (3.10) is also supported via IsRH7Kernel() check, and may have
    // the old 'nfct' field name if Red Hat didn't backport the rename.
#if defined(LINUX_VERSION_CODE) && LINUX_VERSION_CODE >= KERNEL_VERSION(4, 7, 0)
    bpf_probe_read_kernel_with_telemetry(&nfct, sizeof(nfct), &skb->_nfct);
#else
    // Older kernels (including potentially RHEL7 3.10) use 'nfct'
    bpf_probe_read_kernel_with_telemetry(&nfct, sizeof(nfct), &skb->nfct);
#endif
#endif // COMPILE_RUNTIME

#ifdef COMPILE_CORE
    // JMW
    // CO-RE: Use bpf_core_field_exists to check which field name is present at runtime.
    // This handles both modern kernels (_nfct) and older/RHEL7 kernels (nfct).
    if (bpf_core_field_exists(skb->_nfct)) {
        BPF_CORE_READ_INTO(&nfct, skb, _nfct);
    } else if (bpf_core_field_exists(((struct sk_buff___nfct_old *)skb)->nfct)) {
        BPF_CORE_READ_INTO(&nfct, (struct sk_buff___nfct_old *)skb, nfct);
    }
#endif // COMPILE_CORE

    if (!nfct) {
        return NULL;
    }

    // Extract ct pointer: lower 3 bits contain ctinfo, mask them off
    // Standard Linux kernel mask is ~7UL (0xFFFFFFFFFFFFFFF8)
    return (struct nf_conn *)(nfct & ~7UL);
}

static __always_inline u32 get_netns(const struct nf_conn *ct) {
    u32 net_ns_inum = 0;

#ifdef COMPILE_RUNTIME
    // depending on the kernel version p_net may be a struct net** or possible_net_t*
    void *p_net = (void *)&ct->ct_net;
#ifdef CONFIG_NET_NS
    struct net *ns = NULL;
    bpf_probe_read_kernel_with_telemetry(&ns, sizeof(ns), p_net);
#ifdef _LINUX_NS_COMMON_H
    bpf_probe_read_kernel_with_telemetry(&net_ns_inum, sizeof(net_ns_inum), &ns->ns.inum);
#else
    bpf_probe_read_kernel_with_telemetry(&net_ns_inum, sizeof(net_ns_inum), &ns->proc_inum);
#endif // _LINUX_NS_COMMON_H
#endif // CONFIG_NET_NS
#endif // COMPILE_RUNTIME

#ifdef COMPILE_CORE
    struct net *nt = NULL;
    if (bpf_core_type_exists(possible_net_t)) {
        possible_net_t pnet = BPF_CORE_READ(ct, ct_net);
        // will not exist if CONFIG_NET_NS undefined
        if (!bpf_core_field_exists(pnet.net)) {
            return 0;
        }
        nt = pnet.net;
    } else {
        // will not exist if CONFIG_NET_NS undefined
        if (!bpf_core_field_exists(((struct nf_conn___old*)ct)->ct_net)) {
            return 0;
        }
        BPF_CORE_READ_INTO(&nt, (struct nf_conn___old*)ct, ct_net);
    }

    if (bpf_core_field_exists(((struct net___old*)nt)->proc_inum)) {
        // struct net * -> unsigned int proc_inum
        BPF_CORE_READ_INTO(&net_ns_inum, (struct net___old*)nt, proc_inum);
    } else if (bpf_core_field_exists(nt->ns)) {
        // struct net * -> ns_common ns . unsigned int inum
        BPF_CORE_READ_INTO(&net_ns_inum, nt, ns.inum);
    }
#endif // COMPILE_CORE

    return net_ns_inum;
}

static __always_inline int nf_conn_to_conntrack_tuples(struct nf_conn* ct, conntrack_tuple_t* orig, conntrack_tuple_t* reply) {
    struct nf_conntrack_tuple_hash tuplehash[IP_CT_DIR_MAX];
    bpf_memset(tuplehash, 0, sizeof(tuplehash));

    BPF_CORE_READ_INTO(&tuplehash, ct, tuplehash);
    struct nf_conntrack_tuple orig_tup = tuplehash[IP_CT_DIR_ORIGINAL].tuple;
    struct nf_conntrack_tuple reply_tup = tuplehash[IP_CT_DIR_REPLY].tuple;

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
