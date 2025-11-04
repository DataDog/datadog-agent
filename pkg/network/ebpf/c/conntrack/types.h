#ifndef __CONNTRACK_TYPES_H
#define __CONNTRACK_TYPES_H

#include "ktypes.h"

typedef struct {
    /* Using the type unsigned __int128 generates an error in the ebpf verifier */
    __u64 saddr_h;
    __u64 saddr_l;
    __u64 daddr_h;
    __u64 daddr_l;
    __u16 sport;
    __u16 dport;
    __u32 netns;
    // Metadata description:
    // First bit indicates if the connection is TCP (1) or UDP (0)
    // Second bit indicates if the connection is V6 (1) or V4 (0)
    __u32 metadata; // This is that big because it seems that we atleast need a 32-bit aligned struct

    __u32 _pad;
} conntrack_tuple_t;

// JMWRENAMED
typedef struct {
    __u64 registers;
    __u64 kprobe__nf_conntrack_hash_insert_entry_count;
    __u64 kprobe__nf_conntrack_hash_insert_failed_to_get_conntrack_tuples_count;
    __u64 kprobe__nf_conntrack_hash_insert_regular_exists_count;
    __u64 kprobe__nf_conntrack_hash_insert_reverse_exists_count;
    __u64 kprobe__nf_conntrack_hash_insert_count;
    __u64 kretprobe_nf_conntrack_hash_check_insert_count;
    __u64 kprobe__nf_conntrack_confirm_entry_count;
    __u64 kprobe__nf_conntrack_confirm_skb_null_count;
    __u64 kprobe__nf_conntrack_confirm_nfct_null_count;
    __u64 kprobe__nf_conntrack_confirm_ct_null_count;
    __u64 kprobe__nf_conntrack_confirm_not_nat_count;
    __u64 kprobe__nf_conntrack_confirm_pending_added_count;
    __u64 kretprobe__nf_conntrack_confirm_entry_count;
    __u64 kretprobe__nf_conntrack_confirm_no_matching_entry_probe_count;
    __u64 kretprobe__nf_conntrack_confirm_not_accepted_count;
    __u64 kretprobe__nf_conntrack_confirm_not_confirmed_count;
    __u64 kretprobe__nf_conntrack_confirm_failed_to_get_conntrack_tuples_count;
    __u64 kretprobe__nf_conntrack_confirm_success_count;
    __u64 kprobe_ctnetlink_fill_info_failed_to_get_conntrack_tuples_count;
    __u64 kprobe_ctnetlink_fill_info_regular_exists_count;
    __u64 kprobe_ctnetlink_fill_info_reverse_exists_count;
    __u64 kprobe_ctnetlink_fill_info_count;
} conntrack_telemetry_t;


#endif
