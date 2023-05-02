#ifndef __OFFSETS_H
#define __OFFSETS_H

#include "ktypes.h"
#include "compiler.h"

#include "defs.h"

static __always_inline bool dns_stats_enabled() {
    __u64 val = 0;
    LOAD_CONSTANT("dns_stats_enabled", val);
    return val == ENABLED;
}

static __always_inline __u64 offset_family() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_family", val);
    return val;
}

static __always_inline __u64 offset_saddr() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_saddr", val);
    return val;
}

static __always_inline __u64 offset_daddr() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_daddr", val);
    return val;
}

static __always_inline __u64 offset_daddr_ipv6() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_daddr_ipv6", val);
    return val;
}

static __always_inline __u64 offset_sport() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_sport", val);
    return val;
}

static __always_inline __u64 offset_dport() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_dport", val);
    return val;
}

static __always_inline __u64 offset_netns() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_netns", val);
    return val;
}

static __always_inline __u64 offset_ino() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_ino", val);
    return val;
}

static __always_inline __u64 offset_rtt() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_rtt", val);
    return val;
}

static __always_inline __u64 offset_rtt_var() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_rtt_var", val);
    return val;
}

static __always_inline bool are_fl4_offsets_known() {
    __u64 val = 0;
    LOAD_CONSTANT("fl4_offsets", val);
    return val == ENABLED;
}

static __always_inline __u64 offset_saddr_fl4() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_saddr_fl4", val);
    return val;
}

static __always_inline __u64 offset_daddr_fl4() {
     __u64 val = 0;
     LOAD_CONSTANT("offset_daddr_fl4", val);
     return val;
}

static __always_inline __u64 offset_sport_fl4() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_sport_fl4", val);
    return val;
}

static __always_inline __u64 offset_dport_fl4() {
     __u64 val = 0;
     LOAD_CONSTANT("offset_dport_fl4", val);
     return val;
}

static __always_inline bool are_fl6_offsets_known() {
    __u64 val = 0;
    LOAD_CONSTANT("fl6_offsets", val);
    return val == ENABLED;
}

static __always_inline __u64 offset_saddr_fl6() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_saddr_fl6", val);
    return val;
}

static __always_inline __u64 offset_daddr_fl6() {
     __u64 val = 0;
     LOAD_CONSTANT("offset_daddr_fl6", val);
     return val;
}

static __always_inline __u64 offset_sport_fl6() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_sport_fl6", val);
    return val;
}

static __always_inline __u64 offset_dport_fl6() {
     __u64 val = 0;
     LOAD_CONSTANT("offset_dport_fl6", val);
     return val;
}

static __always_inline __u64 offset_socket_sk() {
     __u64 val = 0;
     LOAD_CONSTANT("offset_socket_sk", val);
     return val;
}

static __always_inline __u64 offset_sk_buff_sock() {
    __u64 val = 0;
    LOAD_CONSTANT("offset_sk_buff_sock", val);
    return val;
}

#endif // __OFFSETS_H
