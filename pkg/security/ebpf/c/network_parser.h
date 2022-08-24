#ifndef _NETWORK_PARSER_H_
#define _NETWORK_PARSER_H_

#define ACT_OK   TC_ACT_UNSPEC
#define ACT_SHOT TC_ACT_SHOT

static __attribute__((always_inline)) u32 get_netns() {
    u64 netns;
    LOAD_CONSTANT("netns", netns);
    return (u32) netns;
}

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

    struct namespaced_flow_t ns_flow;
    struct namespaced_flow_t translated_ns_flow;

    u32 offset;
    u32 pid;
    u32 payload_len;
    u16 l4_protocol;
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
        .ns_flow = {
            .netns = get_netns(),
        },
    };
    bpf_map_update_elem(&packets, &key, &new_pkt, BPF_ANY);
    return get_packet();
}

__attribute__((always_inline)) void fill_network_process_context(struct process_context_t *process, struct packet_t *pkt) {
    process->pid = pkt->pid;
    process->tid = pkt->pid;
    process->netns = pkt->translated_ns_flow.netns;
}

struct network_device_context_t {
    u32 netns;
    u32 ifindex;
};

struct network_context_t {
    struct network_device_context_t device;
    struct flow_t flow;

    u32 size;
    u16 l3_protocol;
    u16 l4_protocol;
};

__attribute__((always_inline)) void fill_network_context(struct network_context_t *net_ctx, struct __sk_buff *skb, struct packet_t *pkt) {
    net_ctx->l3_protocol = htons(pkt->eth.h_proto);
    net_ctx->l4_protocol = pkt->l4_protocol;
    net_ctx->size = skb->len;
    net_ctx->flow = pkt->translated_ns_flow.flow;

    // network device context
    net_ctx->device.netns = pkt->translated_ns_flow.netns;
    net_ctx->device.ifindex = skb->ifindex;
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
    struct namespaced_flow_t tmp_ns_flow = pkt->ns_flow; // for compatibility with older kernels
    pkt->translated_ns_flow = pkt->ns_flow;

    // lookup flow in conntrack table
    #pragma unroll
    for (int i = 0; i < 10; i++) {
        struct namespaced_flow_t *translated_ns_flow = bpf_map_lookup_elem(&conntrack, &tmp_ns_flow);
        if (translated_ns_flow == NULL) {
            break;
        }

        pkt->translated_ns_flow = *translated_ns_flow;
        tmp_ns_flow = *translated_ns_flow;
    }

    // TODO: if nothing was found in the conntrack map, lookup ingress nat rules (nothing to do for egress though)

    // resolve pid
    switch (network_direction) {
        case EGRESS: {
            pid_route.addr[0] = pkt->translated_ns_flow.flow.saddr[0];
            pid_route.addr[1] = pkt->translated_ns_flow.flow.saddr[1];
            pid_route.port = pkt->translated_ns_flow.flow.sport;
            pid_route.netns = pkt->translated_ns_flow.netns;
            break;
        }
        case INGRESS: {
            pid_route.addr[0] = pkt->translated_ns_flow.flow.daddr[0];
            pid_route.addr[1] = pkt->translated_ns_flow.flow.daddr[1];
            pid_route.port = pkt->translated_ns_flow.flow.dport;
            pid_route.netns = pkt->translated_ns_flow.netns;
        }
    }
    pkt->pid = get_flow_pid(&pid_route);

    // TODO: l3 / l4 firewall

    // route l7 protocol
    if (pkt->translated_ns_flow.flow.dport == htons(53)) {
        tail_call_to_classifier(skb, DNS_REQUEST);
    }

    return ACT_OK;
}

#endif
