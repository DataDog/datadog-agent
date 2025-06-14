#ifndef _HELPERS_NETWORK_IMDS_H
#define _HELPERS_NETWORK_IMDS_H

#include "constants/enums.h"
#include "helpers/container.h"
#include "helpers/network/context.h"
#include "helpers/process.h"
#include "maps.h"


__attribute__((always_inline)) struct imds_event_t *get_imds_event() {
    u32 key = IMDS_EVENT_KEY;
    return bpf_map_lookup_elem(&imds_event, &key);
}

__attribute__((always_inline)) struct imds_event_t *reset_imds_event(struct __sk_buff *skb, struct packet_t *pkt) {
    struct imds_event_t *evt = get_imds_event();
    if (evt == NULL) {
        // should never happen
        return NULL;
    }

    // reset event flags
    evt->event.flags = 0;

    // process context
    fill_network_process_context_from_pkt(&evt->process, pkt);

    // network context
    fill_network_context(&evt->network, skb, pkt);

    // should we sample this event for activity dumps ?
    struct activity_dump_config *config = lookup_or_delete_traced_pid(evt->process.pid, bpf_ktime_get_ns(), NULL);
    if (config) {
        if (mask_has_event(config->event_mask, EVENT_IMDS)) {
            evt->event.flags |= EVENT_FLAGS_ACTIVITY_DUMP_SAMPLE;
        }
    }

    return evt;
}

#endif
