#ifndef _HELPERS_NETWORK_CREDENTIALS_H
#define _HELPERS_NETWORK_CREDENTIALS_H

#include "constants/custom.h"
#include "maps.h"
#include "structs/network.h"

// looks up the credential source for an address
__attribute__((always_inline)) u32 lookup_credential_source(u64 *addr) {
    struct credential_endpoint_t key = {};
    key.addr[0] = addr[0];
    key.addr[1] = addr[1];

    u32 *source = bpf_map_lookup_elem(&credential_endpoints, &key);
    if (source == NULL) {
        return CREDENTIAL_SOURCE_UNKNOWN;
    }
    return *source;
}

// credential source of a packet's src or dst address
__attribute__((always_inline)) u32 get_credential_source(struct packet_t *pkt) {
    u32 source = lookup_credential_source(pkt->ns_flow.flow.daddr);
    if (source != CREDENTIAL_SOURCE_UNKNOWN) {
        return source;
    }
    return lookup_credential_source(pkt->ns_flow.flow.saddr);
}

#endif
