#include "tracer.h"
#include "bpf_helpers.h"
#include "ip.h"
#include "tls.h"
#include "defs.h"
#include "classifier-telemetry.h"

#define PROTO_PROG_TLS 0
struct bpf_map_def SEC("maps/proto_progs") proto_progs = {
    .type = BPF_MAP_TYPE_PROG_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
};

SEC("socket/classifier_filter")
int socket__classifier_filter(struct __sk_buff* skb) {
    skb_info_t skb_info;
    conn_tuple_t tup;
    __builtin_memset(&tup, 0, sizeof(tup));
    if (!read_conn_tuple_skb(skb, &skb_info, &tup)) {
        return 0;
    }

    if (!(tup.metadata&CONN_TYPE_TCP)) {
        return 0;
    }
    if (skb_info.tcp_flags & TCPHDR_FIN) {
        tls_cleanup(&tup);
    } else {
        bpf_tail_call_compat(skb, &proto_progs, PROTO_PROG_TLS);
        increment_classifier_telemetry_count(tail_call_failed);
        return 0;
    }

    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
