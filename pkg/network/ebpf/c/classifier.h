#ifndef CLASSIFIER_H
#define CLASSIFIER_H

#include "tracer.h"

typedef struct {
    skb_info_t skb_info;
    conn_tuple_t tup;
} proto_args_t;

struct bpf_map_def SEC("maps/proto_args") proto_args = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(proto_args_t),
    .max_entries = 1,
};

// this objects is emebedded in all protocol objects.
// it holds information needed in the classifier
// socket filter, before invoking the subprogram for
// handling a particular protocol.
typedef struct {
    __u8 done;
    __u8 failed;
} cnx_info_t;



#endif // CLASSIFIER_H
