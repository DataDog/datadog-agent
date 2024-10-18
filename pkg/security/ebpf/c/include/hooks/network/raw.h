#ifndef _HOOKS_NETWORK_RAW_H_
#define _HOOKS_NETWORK_RAW_H_

#include "helpers/network.h"
#include "perf_ring.h"

__attribute__((always_inline)) struct raw_packet_event_t *get_raw_packet_event() {
    u32 key = 0;
    return bpf_map_lookup_elem(&raw_packet_event, &key);
}

SEC("classifier/raw_packet")
int classifier_raw_packet(struct __sk_buff *skb) {
    struct packet_t *pkt = get_packet();
    if (pkt == NULL) {
        // should never happen
        return ACT_OK;
    }

    struct raw_packet_event_t *evt = get_raw_packet_event();
    if (evt == NULL || skb == NULL || evt->len == 0) {
        // should never happen
        return ACT_OK;
    }

    bpf_printk("DATA: %d", evt->data[12]);

    // process context
    fill_network_process_context(&evt->process, pkt);

    struct proc_cache_t *entry = get_proc_cache(evt->process.pid);
    if (entry == NULL) {
        evt->container.container_id[0] = 0;
    } else {
        copy_container_id_no_tracing(entry->container.container_id, &evt->container.container_id);
    }

    fill_network_device_context(&evt->device, skb, pkt);

    u32 len = evt->len;
    if (len > sizeof(evt->data)) {
        len = sizeof(evt->data);
    }

    send_event_with_size_ptr(skb, EVENT_RAW_PACKET, evt, offsetof(struct raw_packet_event_t, data) + len);

    return ACT_OK;
}

#endif
