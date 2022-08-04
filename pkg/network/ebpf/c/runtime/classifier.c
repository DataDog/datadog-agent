#include "bpf_helpers.h"

/* some header soup here
 * tracer.h must be included before ip.h
 * and ip.h must be included before tls.h
 * This order satisfies these dependencies */
#include "classifier.h"
#include "ip.h"
#include "tls.h"
/* */

#include "classifier-telemetry.h"
#include "conn-tuple.h"
#include "skb.h"

#define PROTO_PROG_TLS 1
#define PROG_INDX(indx) ((indx)-1)
struct bpf_map_def SEC("maps/proto_progs") proto_progs = {
    .type = BPF_MAP_TYPE_PROG_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
};


struct connection_tuple {
    __u64 saddr_h;
    __u64 saddr_l;
    __u64 daddr_h;
    __u64 daddr_l;
    __u16 sport;
    __u16 dport;
};

struct bpf_map_def SEC("maps/filter_args") filter_args = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(struct connection_tuple),
    .value_size = sizeof(conn_tuple_t),
    .max_entries = 0,
};

static __always_inline int fingerprint_proto(skb_info_t* skb_info, struct __sk_buff* skb) {
    if (is_tls(skb, skb_info->data_off))
        return PROTO_PROG_TLS;

    return 0;
}

static __always_inline void do_tail_call(void* ctx, int protocol) {
        bpf_tail_call_compat(ctx, &proto_progs, PROG_INDX(protocol));
}

SEC("kprobe/__cgroup_bpf_run_filter_skb")
int kprobe____cgroup_bpf_run_filter_skb(struct pt_regs* ctx) {
    conn_tuple_t tup;
    conn_tuple_t skb_tup;
    struct sock* sk = (struct sock *)PT_REGS_PARM1(ctx);
    struct sk_buff *skb = (struct sk_buff *)PT_REGS_PARM2(ctx);

    if (!sk || !skb)
        return 0;

    if (!read_conn_tuple(&tup, sk, 0, 0))
        return 0;

    __builtin_memset(&skb_tup, 0, sizeof(conn_tuple_t));
    if (sk_buff_to_tuple(skb, &skb_tup) <= 0)
        return 0;

    tup.netns = get_netns(&sk->sk_net);
    bpf_map_update_elem(&filter_args, &skb_tup, &tup, BPF_ANY);

    return 0;
}

SEC("socket/classifier_filter")
int socket__classifier_filter(struct __sk_buff* skb) {
    conn_tuple_t *intup;
    proto_args_t args;
    session_t new_session;
    conn_tuple_t skb_tup;

    __builtin_memset(&args, 0, sizeof(proto_args_t));
    __builtin_memset(&new_session, 0, sizeof(new_session));
    __builtin_memset(&skb_tup, 0, sizeof(conn_tuple_t));

    skb_info_t* skb_info = &args.skb_info;
    conn_tuple_t* tup = &args.tup;

    if (!read_conn_tuple_skb(skb, skb_info, &skb_tup))
        return 0;

    intup = (conn_tuple_t *)bpf_map_lookup_elem(&filter_args, &skb_tup);
    if (intup == NULL)
        return 0;

    if (!(skb_tup.metadata&CONN_TYPE_TCP)) {
        bpf_map_delete_elem(&filter_args, &skb_tup);
        return 0;
    }

    intup->metadata |= CONN_TYPE_TCP;

    __builtin_memcpy(tup, intup, sizeof(conn_tuple_t));
    bpf_map_delete_elem(&filter_args, &skb_tup);

    normalize_tuple(tup);
    if (skb_info->tcp_flags & TCPHDR_FIN) {
	    bpf_map_delete_elem(&proto_in_flight, tup);
	    return 0;
    }

    cnx_info_t *info = bpf_map_lookup_elem(&proto_in_flight, tup);
    if (info != NULL) {
        if (info->done)
            return 0;
    }

    int protocol = fingerprint_proto(skb_info, skb);
    u32 cpu = bpf_get_smp_processor_id();
    if (protocol) {
        int err = bpf_map_update_elem(&proto_args, &cpu, &args, BPF_ANY);
        if (err < 0)
            return 0;

        bpf_map_update_elem(&proto_in_flight, tup, &new_session, BPF_NOEXIST);
        do_tail_call(skb, protocol);
        increment_classifier_telemetry_count(tail_call_failed);
    }

    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
