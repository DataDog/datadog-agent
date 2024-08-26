#ifndef _STRUCTS_NETWORK_H_
#define _STRUCTS_NETWORK_H_

struct pid_route_t {
    u64 addr[2];
    u32 netns;
    u16 port;
};

struct flow_t {
    u64 saddr[2];
    u64 daddr[2];
    u16 sport;
    u16 dport;
    u32 padding;
};

struct namespaced_flow_t {
    struct flow_t flow;
    u32 netns;
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
    u16 l4_protocol;
};

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

#endif
