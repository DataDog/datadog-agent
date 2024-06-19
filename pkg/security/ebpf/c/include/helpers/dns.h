#ifndef _HELPERS_DNS_H
#define _HELPERS_DNS_H

#include "constants/enums.h"
#include "maps.h"

#include "activity_dump.h"
#include "container.h"
#include "network.h"
#include "process.h"

__attribute__((always_inline)) struct dns_event_t *get_dns_event() {
    u32 key = DNS_EVENT_KEY;
    return bpf_map_lookup_elem(&dns_event, &key);
}

__attribute__((always_inline)) struct dns_event_t *reset_dns_event(struct __sk_buff *skb, struct packet_t *pkt) {
    struct dns_event_t *evt = get_dns_event();
    if (evt == NULL) {
        // should never happen
        return NULL;
    }

    // reset DNS name
    evt->name[0] = 0;
    evt->size = pkt->payload_len;
    evt->event.flags = 0;

    // process context
    fill_network_process_context(&evt->process, pkt);

    // network context
    fill_network_context(&evt->network, skb, pkt);

    struct proc_cache_t *entry = get_proc_cache(evt->process.pid);
    if (entry == NULL) {
        evt->container.container_id[0] = 0;
    } else {
        copy_container_id_no_tracing(entry->container.container_id, &evt->container.container_id);
        evt->container.flags = entry->container.flags;
    }

    // should we sample this event for activity dumps ?
    struct activity_dump_config *config = lookup_or_delete_traced_pid(evt->process.pid, bpf_ktime_get_ns(), NULL);
    if (config) {
        if (mask_has_event(config->event_mask, EVENT_DNS)) {
            evt->event.flags |= EVENT_FLAGS_ACTIVITY_DUMP_SAMPLE;
        }
    }

    return evt;
}

#endif
