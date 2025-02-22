#ifndef _HELPERS_NETWORK_RAW_H_
#define _HELPERS_NETWORK_RAW_H_

#include "maps.h"

__attribute__((always_inline)) struct raw_packet_event_t *get_raw_packet_event() {
    u32 key = 0;
    return bpf_map_lookup_elem(&raw_packet_event, &key);
}

#endif
