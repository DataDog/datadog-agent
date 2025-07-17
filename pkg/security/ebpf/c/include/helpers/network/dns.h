#ifndef _HELPERS_NETWORK_DNS_H
#define _HELPERS_NETWORK_DNS_H

#include "constants/enums.h"
#include "helpers/activity_dump.h"
#include "helpers/container.h"
#include "helpers/process.h"

#include "context.h"

#include "maps.h"

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
    fill_network_process_context_from_pkt(&evt->process, pkt);

    u64 sched_cls_has_current_pid_tgid_helper = 0;
    LOAD_CONSTANT("sched_cls_has_current_pid_tgid_helper", sched_cls_has_current_pid_tgid_helper);
    if (sched_cls_has_current_pid_tgid_helper) {
        // reset and fill span context
        reset_span_context(&evt->span);
        fill_span_context(&evt->span);
    }

    // network context
    fill_network_context(&evt->network, skb, pkt);

    struct proc_cache_t *entry = get_proc_cache(evt->process.pid);
    if (entry == NULL) {
        evt->container.container_id[0] = 0;
    } else {
        copy_container_id_no_tracing(entry->container.container_id, &evt->container.container_id);
        evt->container.cgroup_context = entry->container.cgroup_context;
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

__attribute__((always_inline)) union dns_responses_t *get_dns_response_event() {
    const u32 key = DNS_EVENT_KEY;
    return bpf_map_lookup_elem(&dns_response_event, &key);
}

__attribute__((always_inline)) union dns_responses_t *reset_dns_response_event(struct __sk_buff *skb, struct packet_t *pkt) {
    union dns_responses_t *evt = get_dns_response_event();

    if (evt == NULL) {
        // should never happen
        return NULL;
    }

    __builtin_memset(evt, 0, sizeof(*evt));

    return evt;
}

#endif
