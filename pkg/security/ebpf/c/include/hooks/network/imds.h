#ifndef _HOOKS_NETWORK_IMDS_H_
#define _HOOKS_NETWORK_IMDS_H_

#include "helpers/imds.h"
#include "helpers/network.h"
#include "perf_ring.h"

SEC("classifier/imds_request")
int classifier_imds_request(struct __sk_buff *skb) {
    struct packet_t *pkt = get_packet();
    if (pkt == NULL) {
        // should never happen
        return ACT_OK;
    }

    struct imds_event_t *evt = reset_imds_event(skb, pkt);
    if (evt == NULL || skb == NULL) {
        // should never happen
        return ACT_OK;
    }

    pkt->payload_len = pkt->payload_len & (IMDS_MAX_LENGTH - 1);
    if (pkt->payload_len > 1) {
        // copy IMDS request
        if (bpf_skb_load_bytes(skb, pkt->offset, evt->body, pkt->payload_len) < 0) {
            return ACT_OK;
        }

        send_event_with_size_ptr(skb, EVENT_IMDS, evt, offsetof(struct imds_event_t, body) + (pkt->payload_len & (IMDS_MAX_LENGTH - 1)));
    }

    // done
    return ACT_OK;
}

#endif
