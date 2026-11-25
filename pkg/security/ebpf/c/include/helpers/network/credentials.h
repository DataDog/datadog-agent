#ifndef _HELPERS_NETWORK_CREDENTIALS_H
#define _HELPERS_NETWORK_CREDENTIALS_H

#include "constants/custom.h"
#include "maps.h"
#include "structs/network.h"

// looks up the credential source for an address
static __always_inline u32 lookup_credential_source(u64 *addr) {
    struct credential_endpoint_t key = {};
    key.addr[0] = addr[0];
    key.addr[1] = addr[1];

    u32 *source = bpf_map_lookup_elem(&credential_endpoints, &key);
    if (source == NULL) {
        return CREDENTIAL_SOURCE_UNKNOWN;
    }
    return *source;
}

// credential source of a packet's observed or conntrack-restored src or dst address
static __always_inline u32 get_credential_source(struct packet_t *pkt) {
    u32 source = lookup_credential_source(pkt->ns_flow.flow.daddr);
    if (source != CREDENTIAL_SOURCE_UNKNOWN) {
        return source;
    }
    source = lookup_credential_source(pkt->ns_flow.flow.saddr);
    if (source != CREDENTIAL_SOURCE_UNKNOWN) {
        return source;
    }

    // DNAT can hide the credential endpoint at the TC hook. Fall back to the
    // original addresses restored by conntrack, without repeating lookups for
    // addresses that were not translated.
    if (pkt->translated_ns_flow.flow.daddr[0] != pkt->ns_flow.flow.daddr[0] ||
        pkt->translated_ns_flow.flow.daddr[1] != pkt->ns_flow.flow.daddr[1]) {
        source = lookup_credential_source(pkt->translated_ns_flow.flow.daddr);
        if (source != CREDENTIAL_SOURCE_UNKNOWN) {
            return source;
        }
    }
    if (pkt->translated_ns_flow.flow.saddr[0] != pkt->ns_flow.flow.saddr[0] ||
        pkt->translated_ns_flow.flow.saddr[1] != pkt->ns_flow.flow.saddr[1]) {
        return lookup_credential_source(pkt->translated_ns_flow.flow.saddr);
    }
    return CREDENTIAL_SOURCE_UNKNOWN;
}

#endif
