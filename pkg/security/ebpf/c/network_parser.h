#ifndef _NETWORK_PARSER_H_
#define _NETWORK_PARSER_H_

#define ACT_OK   TC_ACT_UNSPEC
#define ACT_SHOT TC_ACT_SHOT

static __attribute__((always_inline)) u32 get_netns() {
    u64 netns;
    LOAD_CONSTANT("netns", netns);
    return (u32) netns;
}

struct network_device_context_t {
    u32 netns;
    u32 ifindex;
};

struct cursor {
	void *pos;
	void *end;
};

__attribute__((always_inline)) void tc_cursor_init(struct cursor *c, struct __sk_buff *skb) {
	c->end = (void *)(long)skb->data_end;
	c->pos = (void *)(long)skb->data;
}

#define PARSE_FUNC(STRUCT)                                                                               \
__attribute__((always_inline)) struct STRUCT *parse_ ## STRUCT (struct cursor *c, struct STRUCT *dest) { \
	struct STRUCT *ret = c->pos;                                                                         \
	if (c->pos + sizeof(struct STRUCT) > c->end)                                                         \
		return 0;                                                                                        \
	c->pos += sizeof(struct STRUCT);                                                                     \
	*dest = *ret;                                                                                        \
	return ret;                                                                                          \
}

PARSE_FUNC(ethhdr)
PARSE_FUNC(iphdr)
PARSE_FUNC(ipv6hdr)
PARSE_FUNC(udphdr)
PARSE_FUNC(tcphdr)

#define PACKET_KEY 0

struct packet_t {
    struct ethhdr eth;
    struct iphdr ipv4;
    struct ipv6hdr ipv6;
    struct tcphdr tcp;
    struct udphdr udp;

    struct flow_t flow;
    struct flow_t translated_flow;

    u32 offset;
    u32 pid;
    u8 l4_protocol;
};

struct bpf_map_def SEC("maps/packets") packets = {
    .type = BPF_MAP_TYPE_PERCPU_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct packet_t),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

__attribute__((always_inline)) struct packet_t *get_packet() {
    u32 key = PACKET_KEY;
    return bpf_map_lookup_elem(&packets, &key);
}

__attribute__((always_inline)) struct packet_t *reset_packet() {
    u32 key = PACKET_KEY;
    struct packet_t new_pkt = {
        .flow = {
            .netns = get_netns(),
        },
    };
    bpf_map_update_elem(&packets, &key, &new_pkt, BPF_ANY);
    return get_packet();
}

#define DNS_REQUEST        1
#define DNS_REQUEST_PARSER 2

struct bpf_map_def SEC("maps/classifier_router") classifier_router = {
    .type = BPF_MAP_TYPE_PROG_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 100,
};

__attribute__((always_inline)) void tail_call_to_classifier(struct __sk_buff *skb, int classifier_id) {
    bpf_tail_call_compat(skb, &classifier_router, classifier_id);
}

__attribute__((always_inline)) int route_pkt(struct __sk_buff *skb, struct packet_t *pkt, int network_direction) {
    struct pid_route_t pid_route = {};
    struct flow_t tmp_flow = pkt->flow; // for compatibility with older kernels
    pkt->translated_flow = pkt->flow;

    // lookup flow in conntrack table
    #pragma unroll
    for (int i = 0; i < 10; i++) {
        struct flow_t *translated_flow = bpf_map_lookup_elem(&conntrack, &tmp_flow);
        if (translated_flow == NULL) {
            break;
        }

        pkt->translated_flow = *translated_flow;
        tmp_flow = *translated_flow;
    }

    // TODO: if nothing was found in the conntrack map, lookup ingress nat rules (nothing to do for egress though)

    // resolve pid
    switch (network_direction) {
        case EGRESS: {
            pid_route.addr[0] = pkt->translated_flow.saddr[0];
            pid_route.addr[1] = pkt->translated_flow.saddr[1];
            pid_route.netns = pkt->translated_flow.netns;
            pid_route.port = pkt->translated_flow.sport;
            break;
        }
        case INGRESS: {
            pid_route.addr[0] = pkt->translated_flow.daddr[0];
            pid_route.addr[1] = pkt->translated_flow.daddr[1];
            pid_route.netns = pkt->translated_flow.netns;
            pid_route.port = pkt->translated_flow.dport;
        }
    }
    pkt->pid = get_flow_pid(&pid_route);

    // TODO: l3 / l4 firewall

    // route l7 protocol
    if (pkt->translated_flow.dport == htons(53)) {
        tail_call_to_classifier(skb, DNS_REQUEST);
    }

    return ACT_OK;
}

#endif
