#ifndef __CONNTRACK_H
#define __CONNTRACK_H
#include "vmlinux.h"
#include "conntrack-types.h"
#include "bpf-common.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "map-defs.h"
#include "tracer.h"
#include "ip.h"
#include "socket.h"
#include "bpf_telemetry.h"
#include "bpf_endian.h"
#include "conntrack-user.h"

#ifdef FEATURE_IPV6_ENABLED
#include "ipv6.h"
#endif

#ifndef TASK_COMM_LEN
#define TASK_COMM_LEN 16
#endif


typedef struct {
    char comm[TASK_COMM_LEN];
} proc_t;

struct ct_net___old {
    unsigned int proc_inum;
} __attribute__((preserve_access_index));

//static __always_inline u32 ct_status(const struct nf_conn *ct) {
//    u32 status = 0;
//    bpf_probe_read_kernel_with_telemetry(&status, sizeof(status), (void *)&ct->status);
//     log_debug("status: :%u\n", status);
//    return status;
//}
//
static __always_inline u32 ct_status(const struct nf_conn *ct) {
    u32 status = 0;
    u32 ct_conn_status =  BPF_CORE_READ(ct, status);
    bpf_probe_read_kernel_with_telemetry(&status, sizeof(status), (void *)(&ct_conn_status));
    return status;
}

static __always_inline void increment_telemetry_registers_count() {
    u64 key = 0;
    conntrack_telemetry_t *val = bpf_map_lookup_elem(&conntrack_telemetry, &key);
    if (val == NULL) {
        return;
    }
    __sync_fetch_and_add(&val->registers, 1);
}

static __always_inline void print_translation(const conntrack_tuple_t *t) {
    if (t->metadata & CONN_TYPE_TCP) {
        log_debug("TCP\n");
    } else {
        log_debug("UDP\n");
    }

    print_ip(t->saddr_h, t->saddr_l, t->sport, t->metadata);
    print_ip(t->daddr_h, t->daddr_l, t->dport, t->metadata);
}

static __always_inline int nf_conntrack_tuple_to_conntrack_tuple(conntrack_tuple_t *t, const struct nf_conntrack_tuple *ct) {
    memset(t, 0, sizeof(conntrack_tuple_t));

    //ct->dst.protonum
    switch (BPF_CORE_READ(ct, dst.protonum)) {
    case IPPROTO_TCP:
        t->metadata = CONN_TYPE_TCP;
        t->sport = BPF_CORE_READ(ct, src.u.tcp.port);
        t->dport = BPF_CORE_READ(ct, dst.u.tcp.port);
        break;
    case IPPROTO_UDP:
        t->metadata = CONN_TYPE_UDP;
        t->sport = BPF_CORE_READ(ct, src.u.udp.port);
        t->dport = BPF_CORE_READ(ct, dst.u.udp.port);
        break;
    default:
        log_debug("ERR(to_conn_tuple): unknown protocol number: %u\n", ct->dst.protonum);
        return 0;
    }

    t->sport = bpf_ntohs(t->sport);
    t->dport = bpf_ntohs(t->dport);
    if (t->sport == 0 || t->dport == 0) {
        log_debug("ERR(to_conn_tuple): src/dst port not set: src: %u, dst: %u\n", t->sport, t->dport);
        return 0;
    }

    if (BPF_CORE_READ(ct, src.l3num) == AF_INET) {
        t->metadata |= CONN_V4;
        t->saddr_l = BPF_CORE_READ(ct, src.u3.ip);
        t->daddr_l = BPF_CORE_READ(ct, dst.u3.ip);

        if (!t->saddr_l || !t->daddr_l) {
            log_debug("ERR(to_conn_tuple.v4): src/dst addr not set src:%u, dst:%u\n", t->saddr_l, t->daddr_l);
            return 0;
        }
    }
#ifdef FEATURE_IPV6_ENABLED
    else if (BPF_CORE_READ(ct, src.l3num) == AF_INET6) {
        t->metadata |= CONN_V6;
        read_in6_addr(&t->saddr_h, &t->saddr_l, &BPF_CORE_READ(ct, src.u3.in6));
        read_in6_addr(&t->daddr_h, &t->daddr_l, &BPF_CORE_READ(ct, dst.u3.in6));

        if (!(t->saddr_h || t->saddr_l)) {
            log_debug("ERR(to_conn_tuple.v6): src addr not set: src_l: %llu, src_h: %llu\n",
                t->saddr_l, t->saddr_h);
            return 0;
        }
        if (!(t->daddr_h || t->daddr_l)) {
            log_debug("ERR(to_conn_tuple.v6): dst addr not set: dst_l: %llu, dst_h: %llu\n",
                t->daddr_l, t->daddr_h);
            return 0;
        }
    }
#endif

    return 1;
}

// depending on the kernel version p_net may be a struct net** or possible_net_t*
static __always_inline u32 get_netns(void *p_net) {
    u32 net_ns_inum = 0;
    struct net *ct_net = NULL;
    bpf_probe_read_kernel_with_telemetry(&ct_net, sizeof(ct_net), p_net);
    if (bpf_core_field_exists(ct_net->ns.inum)) {
        unsigned int inum = BPF_CORE_READ(ct_net, ns.inum);
        bpf_core_read(&net_ns_inum, sizeof(net_ns_inum), &inum);
    } else {
        struct ct_net___old *ct_net_old = (void *)ct_net;
        unsigned int proc_inum = BPF_CORE_READ(ct_net_old, proc_inum);
        bpf_probe_read(&net_ns_inum, sizeof(net_ns_inum), &proc_inum);
    }

    return net_ns_inum;
}

static __always_inline bool proc_t_comm_prefix_equals(const char* prefix, int prefix_len, proc_t c) {
    if (prefix_len > TASK_COMM_LEN) {
        return false;
    }

    for (int i = 0; i < prefix_len; i++) {
        if (c.comm[i] != prefix[i]) {
            return false;
        }
    }
    return true;
}
#endif
