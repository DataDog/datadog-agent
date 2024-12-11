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

typedef struct {
    __u64 registers;
} conntrack_telemetry_t;


#endif
