#ifndef _HOOKS_NETWORK_RAW_H_
#define _HOOKS_NETWORK_RAW_H_

#include "helpers/network/parser.h"
#include "helpers/network/raw.h"
#include "perf_ring.h"

TAIL_CALL_CLASSIFIER_FNC(raw_packet_sender, struct __sk_buff *skb) {
    u64 rate = 10;
    LOAD_CONSTANT("raw_packet_limiter_rate", rate);

    if (!global_limiter_allow(RAW_PACKET_LIMITER, rate, 1)) {
        return TC_ACT_UNSPEC;
    }

    struct packet_t *pkt = get_packet();
    if (pkt == NULL) {
        // should never happen
        return TC_ACT_UNSPEC;
    }

    struct raw_packet_event_t *evt = get_raw_packet_event();
    if (evt == NULL || skb == NULL || evt->len == 0) {
        // should never happen
        return TC_ACT_UNSPEC;
    }

    // process context
    fill_network_process_context_from_pkt(&evt->process, pkt);

    u64 sched_cls_has_current_pid_tgid_helper = 0;
    LOAD_CONSTANT("sched_cls_has_current_pid_tgid_helper", sched_cls_has_current_pid_tgid_helper);
    if (sched_cls_has_current_pid_tgid_helper) {
        // reset and fill span context
        reset_span_context(&evt->span);
        fill_span_context(&evt->span);
    }

    struct proc_cache_t *entry = get_proc_cache(evt->process.pid);
    if (entry == NULL) {
        evt->container.container_id[0] = 0;
    } else {
        copy_container_id_no_tracing(entry->container.container_id, &evt->container.container_id);
    }

    fill_network_device_context_from_pkt(&evt->device, skb, pkt);

    u32 len = evt->len;
    if (len > sizeof(evt->data)) {
        len = sizeof(evt->data);
    }

    send_event_with_size_ptr(skb, EVENT_RAW_PACKET, evt, offsetof(struct raw_packet_event_t, data) + len);

    return TC_ACT_UNSPEC;
}

#endif
