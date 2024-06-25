#ifndef _HOOKS_NETWORK_DNS_H_
#define _HOOKS_NETWORK_DNS_H_

#include "helpers/dns.h"
#include "helpers/network.h"
#include "perf_ring.h"

__attribute__((always_inline)) int parse_dns_request(struct __sk_buff *skb, struct packet_t *pkt, struct dns_event_t *evt) {
    u16 qname_length = 0;
    u8 end_of_name = 0;

// Handle DNS request
#pragma unroll
    for (int i = 0; i < DNS_MAX_LENGTH; i++) {
        if (end_of_name) {
            evt->name[i] = 0;
            break;
        }

        if (bpf_skb_load_bytes(skb, pkt->offset, &evt->name[i], sizeof(u8)) < 0) {
            return -1;
        }

        qname_length += 1;
        pkt->offset += 1;

        if (evt->name[i] == 0) {
            end_of_name = 1;
        }
    }

    // Handle qtype
    if (bpf_skb_load_bytes(skb, pkt->offset, &evt->qtype, sizeof(u16)) < 0) {
        return -1;
    }
    evt->qtype = htons(evt->qtype);
    pkt->offset += sizeof(u16);

    // Handle qclass
    if (bpf_skb_load_bytes(skb, pkt->offset, &evt->qclass, sizeof(u16)) < 0) {
        return -1;
    }
    evt->qclass = htons(evt->qclass);
    pkt->offset += sizeof(u16);

    return qname_length;
}

__attribute__((always_inline)) int is_dns_request_parsing_done(struct __sk_buff *skb, struct packet_t *pkt) {
    // if there is another DNS name left to parse, the next byte would be the length of its first label
    u8 next_char = 0;
    if (bpf_skb_load_bytes(skb, pkt->offset, &next_char, sizeof(u8)) < 0) {
        return 1;
    }
    if (next_char > 0) {
        return 0;
    }
    return 1;
}

SEC("classifier/dns_request")
int classifier_dns_request(struct __sk_buff *skb) {
    struct packet_t *pkt = get_packet();
    if (pkt == NULL) {
        // should never happen
        return ACT_OK;
    }

    struct dnshdr header = {};
    if (bpf_skb_load_bytes(skb, pkt->offset, &header, sizeof(header)) < 0) {
        return ACT_OK;
    }
    pkt->offset += sizeof(header);

    struct dns_event_t *evt = reset_dns_event(skb, pkt);
    if (evt == NULL) {
        return ACT_OK;
    }
    evt->qdcount = htons(header.qdcount);
    evt->id = htons(header.id);

    // tail call to the dns request parser
    tail_call_to_classifier(skb, DNS_REQUEST_PARSER);

    // tail call failed, ignore packet
    return ACT_OK;
}

SEC("classifier/dns_request_parser")
int classifier_dns_request_parser(struct __sk_buff *skb) {
    struct packet_t *pkt = get_packet();
    if (pkt == NULL) {
        // should never happen
        return ACT_OK;
    }

    struct dns_event_t *evt = get_dns_event();
    if (evt == NULL) {
        // should never happen
        return ACT_OK;
    }

    int qname_length = parse_dns_request(skb, pkt, evt);
    if (qname_length < 0) {
        // couldn't parse DNS request
        return ACT_OK;
    }

    // really should not happen, the loop in parse_dns_request only ever
    // reads DNS_MAX_LENGTH bytes
    if (qname_length > DNS_MAX_LENGTH) {
        return ACT_OK;
    }

    // send DNS event
    send_event_with_size_ptr(skb, EVENT_DNS, evt, offsetof(struct dns_event_t, name) + qname_length);

    if (!is_dns_request_parsing_done(skb, pkt)) {
        tail_call_to_classifier(skb, DNS_REQUEST_PARSER);
    }

    return ACT_OK;
}

#endif
