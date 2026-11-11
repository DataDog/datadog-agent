#include "ktypes.h"
#include "bpf_metadata.h"
#include "bpf_helpers.h"
#include "map-defs.h"
#include "ip.h"

// Test-only program: runs the bulk, per-field, and combined header parsers on the same skb so
// TestReadConnTupleSKBEquivalence can compare them byte for byte.

typedef struct {
    __u64 ret;
    skb_info_t info;
    conn_tuple_t tup;
} skb_parse_result_t;

typedef struct {
    __s64 bulk_ret;
    skb_parse_result_t fields;
    skb_parse_result_t bulk;
    skb_parse_result_t combined;
} skb_parse_test_out_t;

// The test seeds the outputs with a byte pattern so missing or extra writes show up.
BPF_ARRAY_MAP(skb_parse_test_seed, skb_parse_result_t, 1);
BPF_ARRAY_MAP(skb_parse_test_out, skb_parse_test_out_t, 1);

SEC("socket/skb_parse_test")
int socket__skb_parse_test(struct __sk_buff *skb) {
    __u32 key = 0;
    skb_parse_result_t *seed = bpf_map_lookup_elem(&skb_parse_test_seed, &key);
    skb_parse_test_out_t *out = bpf_map_lookup_elem(&skb_parse_test_out, &key);
    if (!seed || !out) {
        return 0;
    }

    out->fields = *seed;
    out->fields.ret = read_conn_tuple_skb_fields(skb, &out->fields.info, &out->fields.tup);

    out->bulk = *seed;
    out->bulk_ret = read_conn_tuple_skb_bulk(skb, &out->bulk.info, &out->bulk.tup);
    out->bulk.ret = out->bulk_ret;

    out->combined = *seed;
    out->combined.ret = read_conn_tuple_skb(skb, &out->combined.info, &out->combined.tup);
    return 0;
}

char _license[] SEC("license") = "GPL";
