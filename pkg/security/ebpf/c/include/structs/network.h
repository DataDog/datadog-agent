#ifndef _STRUCTS_NETWORK_H_
#define _STRUCTS_NETWORK_H_

struct pid_route_t {
    u64 addr[2];
    u32 netns;
    u16 port;
    // TODO: wait for implementation on security_socket_bind to be ready first
    // u16 l4_protocol;
};

struct pid_route_entry_t {
    u32 pid;
    u32 type;
    char comm[16];
    u16 family;
    u16 dport;
};

struct flow_t {
    u64 saddr[2];
    u64 daddr[2];
    u16 sport;
    u16 dport;
    u16 l4_protocol;
    u16 l3_protocol;
};

struct network_counters_t {
    u64 data_size;
    u64 pkt_count;
};

struct network_stats_t {
    struct network_counters_t ingress;
    struct network_counters_t egress;
};

struct flow_stats_t {
    struct flow_t flow;
    struct network_stats_t stats;
};

struct namespaced_flow_t {
    struct flow_t flow;
    u32 netns;
};

struct flow_queue_msg_t {
    struct namespaced_flow_t ns_flow;

    u32 pid;
    u32 ifindex;
};

struct active_flows_t {
    struct flow_t flows[ACTIVE_FLOWS_MAX_SIZE];

    u64 last_sent;
    u32 netns;
    u32 ifindex;
    u32 cursor;
};

struct device_t {
    char name[16];
    u32 netns;
    u32 ifindex;
    u32 peer_netns;
    u32 peer_ifindex;
};

struct device_ifindex_t {
    u32 netns;
    u32 ifindex;
};

struct device_name_t {
    char name[16];
    u32 netns;
};

struct veth_state_t {
    struct device_ifindex_t peer_device_key;
    u32 state;
};

struct register_netdevice_cache_t {
    struct net_device *device;
    struct device_ifindex_t ifindex;
};

struct cursor {
    void *pos;
    void *end;
};

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
    u32 network_direction;
};

struct network_device_context_t {
    u32 netns;
    u32 ifindex;
};

struct network_context_t {
    struct network_device_context_t device;
    struct flow_t flow;

    u32 size;
    u32 network_direction;
};

#endif
