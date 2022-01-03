#ifndef _DNS_H_
#define _DNS_H_

#define DNS_PORT 53
#define DNS_MAX_LENGTH 256

struct dnshdr {
    uint16_t id;
    union {
        struct {
            uint8_t  rd     : 1;
            uint8_t  tc     : 1;
            uint8_t  aa     : 1;
            uint8_t  opcode : 4;
            uint8_t  qr     : 1;

            uint8_t  rcode  : 4;
            uint8_t  cd     : 1;
            uint8_t  ad     : 1;
            uint8_t  z      : 1;
            uint8_t  ra     : 1;
        }        as_bits_and_pieces;
        uint16_t as_value;
    } flags;
    uint16_t qdcount;
    uint16_t ancount;
    uint16_t nscount;
    uint16_t arcount;
};

struct dns_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct network_device_context_t device;

    u16 id;
    u16 qdcount;
    u16 qtype;
    u16 qclass;
    u64 dns_server_ip_family;
    u64 dns_server_ip[2];
    char name[DNS_MAX_LENGTH];
};

#define DNS_EVENT_KEY 0

struct bpf_map_def SEC("maps/dns_event") dns_event = {
    .type = BPF_MAP_TYPE_PERCPU_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct dns_event_t),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

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

    evt->name[0] = 0;
    evt->process.pid = pkt->pid;
    evt->process.netns = pkt->netns;
    evt->device.netns = pkt->netns;
    evt->device.ifindex = skb->ifindex;

    struct proc_cache_t *entry = get_proc_cache(evt->process.pid);
    if (entry == NULL) {
        evt->container.container_id[0] = 0;
    } else {
        fill_container_context(entry, &evt->container);
    }

    return evt;
}

__attribute__((always_inline)) int parse_dns_request(struct __sk_buff *skb, struct packet_t *pkt, struct dns_event_t *evt) {
    u16 qname_length = 0;
    u8 end_of_name = 0;

    // Handle DNS request
    #pragma unroll
    for (int i = 0; i < DNS_MAX_LENGTH; i++) {
        if (end_of_name) {
            evt->name[i] = 0;
            continue;
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

__attribute__((always_inline)) int handle_dns_req(struct __sk_buff *skb, struct packet_t *pkt) {
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
    evt->dns_server_ip_family = htons(pkt->eth.h_proto);
    if (evt->dns_server_ip_family == ETH_P_IP) {
        evt->dns_server_ip[0] = pkt->ipv4.daddr;
    } else if (evt->dns_server_ip_family == ETH_P_IPV6) {
        evt->dns_server_ip[0] = *(u64*)&pkt->ipv6.saddr;
        evt->dns_server_ip[1] = *((u64*)(&pkt->ipv6.saddr) + 1);
    }

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

    // send DNS event
    send_event_with_size_ptr(skb, EVENT_DNS, evt, offsetof(struct dns_event_t, name) + qname_length);

    if (!is_dns_request_parsing_done(skb, pkt)) {
        tail_call_to_classifier(skb, DNS_REQUEST_PARSER);
    }

    return ACT_OK;
}

// => add DNS server IP
// => detect new interfaces & attach TC probe
// => loop for IPv6
// => tests
// => parse DNS answer (number of answers, resolved IPs, resolution time)

#endif
