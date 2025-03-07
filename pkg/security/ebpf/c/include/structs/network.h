#ifndef _STRUCTS_NETWORK_H_
#define _STRUCTS_NETWORK_H_

struct pid_route_t {
    u64 addr[2];
    u32 netns;
    u16 port;
    // TODO: wait for implementation on security_socket_bind to be ready first
    // u16 l4_protocol;
};

struct sock_meta_t {
    struct pid_route_t existing_route;
};

struct pid_route_entry_t {
    struct sock* owner_sk; // stores which struct sock* was responsible for adding this entry
    u32 pid;
    u16 type;
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

struct active_flows_t {
    struct flow_t flows[ACTIVE_FLOWS_MAX_SIZE];

    u64 last_sent;
    u32 netns;
    u32 ifindex;
    u32 cursor;
};

struct active_flows_spin_lock_t {
    struct bpf_spin_lock lock;
};

struct inet_bind_args_t {
    struct socket *sock;
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
    s64 pid;
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
