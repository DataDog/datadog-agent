#ifndef _HOOKS_NETWORK_RAW_H_
#define _HOOKS_NETWORK_RAW_H_

#include "helpers/network/parser.h"
#include "helpers/network/raw.h"
#include "perf_ring.h"

SEC("classifier/raw_packet_sender")
int classifier_raw_packet_sender(struct __sk_buff *skb) {
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

    // process context
    fill_network_process_context_from_pkt(&evt->process, pkt);

    fill_network_device_context_from_pkt(&evt->device, skb, pkt);

    u32 len = evt->len;
    if (len > sizeof(evt->data)) {
        len = sizeof(evt->data);
    }

    send_event_with_size_ptr(skb, EVENT_RAW_PACKET, evt, offsetof(struct raw_packet_event_t, data) + len);

    return ACT_OK;
}

#endif
