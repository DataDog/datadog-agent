#ifndef _HOOKS_NETWORK_IMDS_H_
#define _HOOKS_NETWORK_IMDS_H_

#include "helpers/network/imds.h"
#include "helpers/network/parser.h"
#include "perf_ring.h"

TAIL_CALL_CLASSIFIER_FNC(imds_request, struct __sk_buff *skb) {
    struct packet_t *pkt = get_packet();
    if (pkt == NULL) {
        // should never happen
        return TC_ACT_UNSPEC;
    }

    struct imds_event_t *evt = reset_imds_event(skb, pkt);
    if (evt == NULL || skb == NULL) {
        // should never happen
        return TC_ACT_UNSPEC;
    }

    // record the TCP sequence number so userspace can reassemble responses that span
    // multiple TCP segments
    evt->tcp_seq = pkt->l4.tcp.seq;

    // clamp (do NOT wrap with a bitmask) the payload length to the body buffer size: a
    // bitwise AND would truncate or zero out payloads larger than IMDS_MAX_LENGTH
    u32 len = pkt->payload_len;
    if (len > IMDS_MAX_LENGTH) {
        len = IMDS_MAX_LENGTH;
    }
    if (len > 1) {
        // copy IMDS request
        if (bpf_skb_load_bytes(skb, pkt->offset, evt->body, len) < 0) {
            return TC_ACT_UNSPEC;
        }

        send_event_with_size_ptr(skb, EVENT_IMDS, evt, offsetof(struct imds_event_t, body) + len);
    }

    // done
    return TC_ACT_UNSPEC;
}

#endif
