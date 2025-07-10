#ifndef _HOOKS_NETWORK_DNS_H_
#define _HOOKS_NETWORK_DNS_H_

#include "helpers/network/dns.h"
#include "helpers/network/parser.h"
#include "helpers/network/router.h"
#include "perf_ring.h"

#define DNS_ENTRY_TIMEOUT_NS SEC_TO_NS(1)

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

TAIL_CALL_CLASSIFIER_FNC(dns_request, struct __sk_buff *skb) {
    struct packet_t *pkt = get_packet();
    if (pkt == NULL) {
        // should never happen
        return TC_ACT_UNSPEC;
    }

    struct dnshdr header = {};
    if (bpf_skb_load_bytes(skb, pkt->offset, &header, sizeof(header)) < 0) {
        return TC_ACT_UNSPEC;
    }
    pkt->offset += sizeof(header);

    struct dns_event_t *evt = reset_dns_event(skb, pkt);
    if (evt == NULL) {
        return TC_ACT_UNSPEC;
    }
    evt->qdcount = htons(header.qdcount);
    evt->id = htons(header.id);

    // tail call to the dns request parser
    bpf_tail_call_compat(skb, &classifier_router, DNS_REQUEST_PARSER);

    // tail call failed, ignore packet
    return TC_ACT_UNSPEC;
}

TAIL_CALL_CLASSIFIER_FNC(dns_request_parser, struct __sk_buff *skb) {
    struct packet_t *pkt = get_packet();
    if (pkt == NULL) {
        // should never happen
        return TC_ACT_UNSPEC;
    }

    struct dns_event_t *evt = get_dns_event();
    if (evt == NULL) {
        // should never happen
        return TC_ACT_UNSPEC;
    }

    int qname_length = parse_dns_request(skb, pkt, evt);
    if (qname_length < 0) {
        // couldn't parse DNS request
        return TC_ACT_UNSPEC;
    }

    // really should not happen, the loop in parse_dns_request only ever
    // reads DNS_MAX_LENGTH bytes
    if (qname_length > DNS_MAX_LENGTH) {
        return TC_ACT_UNSPEC;
    }

    // send DNS event
    send_event_with_size_ptr(skb, EVENT_DNS, evt, offsetof(struct dns_event_t, name) + qname_length);

    if (!is_dns_request_parsing_done(skb, pkt)) {
        bpf_tail_call_compat(skb, &classifier_router, DNS_REQUEST_PARSER);
    }

    return TC_ACT_UNSPEC;
}

TAIL_CALL_CLASSIFIER_FNC(dns_response, struct __sk_buff *skb) {
    struct packet_t *pkt = get_packet();
    if (pkt == NULL) {
        // should never happen
        return TC_ACT_UNSPEC;
    }

    union dns_responses_t * map_elem = reset_dns_response_event(skb, pkt);
    if (map_elem == NULL) {
        // should never happen
        return TC_ACT_UNSPEC;
    }

    int len = pkt->payload_len;

    if (len > DNS_RECEIVE_MAX_LENGTH) {
        return TC_ACT_UNSPEC;
    }

    if(len <= sizeof(struct dnshdr)) {
        // Reject if less than the minimum size
        return TC_ACT_UNSPEC;
    }

    struct dns_flags_as_bits_and_pieces_t flags;
    if (bpf_skb_load_bytes(skb, pkt->offset + 2, &flags, sizeof(flags)) < 0) {
        return TC_ACT_UNSPEC;
    }

    if(!flags.qr || flags.tc) {
        // Stop processing if it's not a query response or if the message is truncated
        return TC_ACT_UNSPEC;
    }

    uint16_t header_id;
    bool send_packet_with_context = false;
    long err;

    struct bpf_map_def *buffer = select_buffer(&fb_dns_stats, &bb_dns_stats, DNS_FILTERED_KEY);
    if (buffer == NULL) {
        // Should never happen
        return TC_ACT_UNSPEC;
    }

    const u32 key = 0;
    struct dns_receiver_stats_t *stats = bpf_map_lookup_elem(buffer, &key);
    if (stats == NULL) {
        // Should never happen
        return TC_ACT_UNSPEC;
    }

    u16 should_discard = (get_dns_rcode_discarder_mask() >> flags.rcode) & 1;
    if(should_discard) {
        __sync_fetch_and_add(&stats->discarded_dns_packets, 1);
        if (flags.rcode != 0) {
            return TC_ACT_UNSPEC;
        }
        // Even if there's a discarder, we still send packets with rcode=0 without context information for the DNS resolver
        err = bpf_skb_load_bytes(skb, pkt->offset, &map_elem->short_dns_response.header, sizeof(struct dnshdr));
        header_id = map_elem->short_dns_response.header.id;
    } else {
        send_packet_with_context = true;
        fill_network_process_context_from_pkt(&map_elem->full_dns_response.process, pkt);
        u64 sched_cls_has_current_pid_tgid_helper = 0;
        LOAD_CONSTANT("sched_cls_has_current_pid_tgid_helper", sched_cls_has_current_pid_tgid_helper);
        if (sched_cls_has_current_pid_tgid_helper) {
            // fill span context (that was previously reset by reset_dns_response_event)
            fill_span_context(&map_elem->full_dns_response.span);
        }
        fill_network_context(&map_elem->full_dns_response.network, skb, pkt);
        err = bpf_skb_load_bytes(skb, pkt->offset, &map_elem->full_dns_response.header, sizeof(struct dnshdr));
        header_id = map_elem->full_dns_response.header.id;
    }

    if (err < 0) {
        return TC_ACT_UNSPEC;
    }

    pkt->offset += sizeof(struct dnshdr);

    u64 current_timestamp = bpf_ktime_get_ns();
    struct dns_responses_sent_to_userspace_lru_entry_t* lru_entry = bpf_map_lookup_elem(&dns_responses_sent_to_userspace, &header_id);

    if (lru_entry != NULL && lru_entry->timestamp + DNS_ENTRY_TIMEOUT_NS > current_timestamp) {
        if (len == lru_entry->packet_size) {
            __sync_fetch_and_add(&stats->filtered_dns_packets, 1);
            return TC_ACT_UNSPEC;
        }

        __sync_fetch_and_add(&stats->same_id_different_size, 1);
    }

    struct dns_responses_sent_to_userspace_lru_entry_t entry;
    entry.timestamp = current_timestamp;
    entry.packet_size = (u64)len;
    bpf_map_update_elem(&dns_responses_sent_to_userspace, &header_id, &entry, BPF_ANY);

    int remaining_bytes = len - sizeof(struct dnshdr);

    if (remaining_bytes <= 0 || pkt->offset <= 0 || remaining_bytes >= DNS_RECEIVE_MAX_LENGTH) {
        return TC_ACT_UNSPEC;
    }

    if (send_packet_with_context) {
        err = bpf_skb_load_bytes(skb, pkt->offset, (void*)map_elem->full_dns_response.data, remaining_bytes);
    } else {
        err = bpf_skb_load_bytes(skb, pkt->offset, (void*)map_elem->short_dns_response.data, remaining_bytes);
    }

    if (err < 0) {
        return TC_ACT_UNSPEC;
    }

    if (send_packet_with_context) {
        send_event_with_size_ptr(skb, EVENT_DNS_RESPONSE_FULL, &map_elem->full_dns_response, offsetof(struct full_dns_response_event_t, data) + remaining_bytes);
    } else {
        send_event_with_size_ptr(skb, EVENT_DNS_RESPONSE_SHORT, &map_elem->short_dns_response, offsetof(struct short_dns_response_event_t, data) + remaining_bytes);
    }

    return TC_ACT_UNSPEC;
}

#endif
