#ifndef _CREDENTIALS_TEST_H_
#define _CREDENTIALS_TEST_H_

#include "baloum.h"
#include "bpf_endian.h"
#include "helpers/network/credentials.h"
#include "helpers/network/pid_resolver.h"
#include "helpers/network/parser.h"

static __always_inline int test_credential_source(struct credential_endpoint_t *endpoint, struct credential_endpoint_t *other_endpoint, struct credential_endpoint_t *loopback, u32 source) {
    int ret = bpf_map_update_elem(&credential_endpoints, endpoint, &source, BPF_ANY);
    assert_zero(ret, "failed to register credential endpoint");
    u32 other_source = CREDENTIAL_SOURCE_IMDS;
    ret = bpf_map_update_elem(&credential_endpoints, other_endpoint, &other_source, BPF_ANY);
    assert_zero(ret, "failed to register other credential endpoint");

    struct packet_t *pkt = get_packet();
    assert_not_null(pkt, "unable to get packet");
    __builtin_memcpy(pkt->ns_flow.flow.saddr, loopback->addr, sizeof(loopback->addr));
    __builtin_memcpy(pkt->ns_flow.flow.daddr, loopback->addr, sizeof(loopback->addr));
    pkt->translated_ns_flow = pkt->ns_flow;
    assert_equals(get_credential_source(pkt), CREDENTIAL_SOURCE_UNKNOWN, "loopback alone is not a credential endpoint");

    // Direct requests and responses must retain their existing classification.
    __builtin_memcpy(pkt->ns_flow.flow.daddr, endpoint->addr, sizeof(endpoint->addr));
    pkt->translated_ns_flow = pkt->ns_flow;
    assert_equals(get_credential_source(pkt), source, "direct request credential source");

    __builtin_memcpy(pkt->ns_flow.flow.daddr, loopback->addr, sizeof(loopback->addr));
    __builtin_memcpy(pkt->ns_flow.flow.saddr, endpoint->addr, sizeof(endpoint->addr));
    pkt->translated_ns_flow = pkt->ns_flow;
    assert_equals(get_credential_source(pkt), source, "direct response credential source");

    // After local DNAT, only the conntrack-restored flow contains the endpoint.
    __builtin_memcpy(pkt->ns_flow.flow.saddr, loopback->addr, sizeof(loopback->addr));
    pkt->translated_ns_flow = pkt->ns_flow;
    __builtin_memcpy(pkt->translated_ns_flow.flow.daddr, endpoint->addr, sizeof(endpoint->addr));
    assert_equals(get_credential_source(pkt), source, "DNAT request credential source");

    pkt->translated_ns_flow = pkt->ns_flow;
    __builtin_memcpy(pkt->translated_ns_flow.flow.saddr, endpoint->addr, sizeof(endpoint->addr));
    assert_equals(get_credential_source(pkt), source, "DNAT response credential source");

    // Preserve the precedence of observed endpoints over restored endpoints.
    __builtin_memcpy(pkt->ns_flow.flow.daddr, other_endpoint->addr, sizeof(other_endpoint->addr));
    assert_equals(get_credential_source(pkt), other_source, "observed destination takes precedence");

    __builtin_memcpy(pkt->ns_flow.flow.daddr, loopback->addr, sizeof(loopback->addr));
    __builtin_memcpy(pkt->ns_flow.flow.saddr, other_endpoint->addr, sizeof(other_endpoint->addr));
    pkt->translated_ns_flow = pkt->ns_flow;
    __builtin_memcpy(pkt->translated_ns_flow.flow.saddr, loopback->addr, sizeof(loopback->addr));
    __builtin_memcpy(pkt->translated_ns_flow.flow.daddr, endpoint->addr, sizeof(endpoint->addr));
    assert_equals(get_credential_source(pkt), other_source, "observed source takes precedence");

    __builtin_memcpy(pkt->ns_flow.flow.saddr, loopback->addr, sizeof(loopback->addr));
    __builtin_memcpy(pkt->translated_ns_flow.flow.saddr, other_endpoint->addr, sizeof(other_endpoint->addr));
    assert_equals(get_credential_source(pkt), source, "restored destination takes precedence over restored source");

    // Match the full address, including translations that only change its second 64-bit word.
    __builtin_memcpy(pkt->ns_flow.flow.daddr, endpoint->addr, sizeof(endpoint->addr));
    pkt->ns_flow.flow.daddr[1] ^= 1;
    __builtin_memcpy(pkt->translated_ns_flow.flow.saddr, loopback->addr, sizeof(loopback->addr));
    assert_equals(get_credential_source(pkt), source, "restored destination differs in its second 64-bit word");

    __builtin_memcpy(pkt->ns_flow.flow.daddr, loopback->addr, sizeof(loopback->addr));
    __builtin_memcpy(pkt->ns_flow.flow.saddr, endpoint->addr, sizeof(endpoint->addr));
    pkt->ns_flow.flow.saddr[1] ^= 1;
    pkt->translated_ns_flow = pkt->ns_flow;
    __builtin_memcpy(pkt->translated_ns_flow.flow.saddr, endpoint->addr, sizeof(endpoint->addr));
    assert_equals(get_credential_source(pkt), source, "restored source differs in its second 64-bit word");

    // A disabled or unregistered endpoint must not be recognized via conntrack.
    ret = bpf_map_delete_elem(&credential_endpoints, endpoint);
    assert_zero(ret, "failed to unregister credential endpoint");
    assert_equals(get_credential_source(pkt), CREDENTIAL_SOURCE_UNKNOWN, "unregistered restored endpoint");

    return 1;
}

SEC("test/credential_source_ipv4")
int test_credential_source_ipv4() {
    struct credential_endpoint_t endpoint = {.addr = {htonl(0xa9feaa02), 0}}; // 169.254.170.2
    struct credential_endpoint_t other_endpoint = {.addr = {htonl(0xa9fea9fe), 0}}; // 169.254.169.254
    struct credential_endpoint_t loopback = {.addr = {htonl(0x7f000001), 0}}; // 127.0.0.1
    return test_credential_source(&endpoint, &other_endpoint, &loopback, CREDENTIAL_SOURCE_ECS);
}

SEC("test/credential_source_ipv6")
int test_credential_source_ipv6() {
    struct credential_endpoint_t endpoint = {.addr = {bpf_htonll(0xfd000ec200000000ULL), bpf_htonll(0x23)}}; // fd00:ec2::23
    struct credential_endpoint_t other_endpoint = {.addr = {bpf_htonll(0xfd000ec200000000ULL), bpf_htonll(0x254)}}; // fd00:ec2::254
    struct credential_endpoint_t loopback = {.addr = {0, bpf_htonll(1)}}; // ::1
    return test_credential_source(&endpoint, &other_endpoint, &loopback, CREDENTIAL_SOURCE_EKS_POD_IDENTITY);
}

SEC("test/credential_recipient_legacy")
int test_credential_recipient_legacy() {
    struct packet_t *pkt = get_packet();
    assert_not_null(pkt, "unable to get packet");
    pkt->ns_flow.netns = 42;
    pkt->translated_ns_flow = (struct namespaced_flow_t){
        .netns = 42,
        .flow = {
            .saddr = {htonl(0xa9feaa02), 0},
            .daddr = {htonl(0x0a000005), 0},
            .tcp_udp = {.sport = htons(80), .dport = htons(44000)},
            .l3_protocol = ETH_P_IP,
            .l4_protocol = IPPROTO_TCP,
        },
    };
    pkt->network_direction = EGRESS;
    pkt->pid = 123;
    pkt->cgroup_id = 999;
    resolve_pid(NULL, pkt, PID_RESOLUTION_STRICT_INGRESS);
    assert_equals(pkt->pid, 123, "egress event keeps the sender context");
    assert_equals(pkt->cgroup_id, 999, "egress event keeps the cgroup context");

    u64 cookie = 1;
    u32 sender_pid = 123;
    int ret = bpf_map_update_elem(&sock_cookie_pid, &cookie, &sender_pid, BPF_ANY);
    assert_zero(ret, "failed to register sender cookie");
    pkt->network_direction = INGRESS;
    resolve_pid(NULL, pkt, PID_RESOLUTION_DEFAULT);
    assert_equals(pkt->pid, sender_pid, "default resolution still trusts the socket cookie");
    assert_zero(pkt->cgroup_id, "default resolution resets the cgroup context");
    pkt->cgroup_id = 999;
    resolve_pid(NULL, pkt, PID_RESOLUTION_STRICT_INGRESS);
    assert_zero(pkt->pid, "missing local recipient must not retain sender pid");
    assert_equals(pkt->cgroup_id, 999, "strict resolution preserves the cgroup context");

    struct pid_route_t route = {
        .addr = {htonl(0x0a000005), 0},
        .netns = 42,
        .port = htons(44000),
        .l4_protocol = IPPROTO_TCP,
    };
    struct pid_route_entry_t entry = {.owner_sk = (struct sock *)1, .pid = 456, .type = FLOW_CLASSIFICATION_ENTRY};
    ret = bpf_map_update_elem(&flow_pid, &route, &entry, BPF_ANY);
    assert_zero(ret, "failed to register recipient route");
    pkt->pid = sender_pid;
    resolve_pid(NULL, pkt, PID_RESOLUTION_STRICT_INGRESS);
    assert_equals(pkt->pid, 456, "ingress event resolves the local recipient instead of the sender cookie");
    ret = bpf_map_delete_elem(&sock_cookie_pid, &cookie);
    assert_zero(ret, "failed to unregister sender cookie");
    resolve_pid(NULL, pkt, PID_RESOLUTION_DEFAULT);
    assert_equals(pkt->pid, 456, "default resolution still uses the exact route without a cookie");

    pkt->ns_flow.netns = 43;
    resolve_pid(NULL, pkt, PID_RESOLUTION_STRICT_INGRESS);
    assert_zero(pkt->pid, "a different namespace must not match");
    resolve_pid(NULL, pkt, PID_RESOLUTION_DEFAULT);
    assert_equals(pkt->pid, 456, "default resolution still uses the translated namespace");
    pkt->ns_flow.netns = 42;

    // A wildcard entry must not attribute traffic forwarded to a different IP.
    route.addr[0] = 0;
    ret = bpf_map_update_elem(&flow_pid, &route, &entry, BPF_ANY);
    assert_zero(ret, "failed to register wildcard route");
    pkt->translated_ns_flow.flow.daddr[0] = htonl(0x0a000006);
    resolve_pid(NULL, pkt, PID_RESOLUTION_STRICT_INGRESS);
    assert_zero(pkt->pid, "forwarded ingress must not match a wildcard route");
    resolve_pid(NULL, pkt, PID_RESOLUTION_DEFAULT);
    assert_equals(pkt->pid, 456, "default resolution still accepts wildcard routes");
    pkt->translated_ns_flow.flow.daddr[0] = htonl(0x0a000005);
    route.addr[0] = htonl(0x0a000005);

    entry.type = BIND_ENTRY;
    ret = bpf_map_update_elem(&flow_pid, &route, &entry, BPF_ANY);
    assert_zero(ret, "failed to register bind route");
    resolve_pid(NULL, pkt, PID_RESOLUTION_STRICT_INGRESS);
    assert_zero(pkt->pid, "a listener alone does not identify the requester");
    resolve_pid(NULL, pkt, PID_RESOLUTION_DEFAULT);
    assert_equals(pkt->pid, 456, "default resolution still accepts listeners");
    entry.type = PROCFS_ENTRY;
    ret = bpf_map_update_elem(&flow_pid, &route, &entry, BPF_ANY);
    assert_zero(ret, "failed to register procfs route");
    resolve_pid(NULL, pkt, PID_RESOLUTION_STRICT_INGRESS);
    assert_zero(pkt->pid, "a procfs entry does not prove socket ownership");
    resolve_pid(NULL, pkt, PID_RESOLUTION_DEFAULT);
    assert_equals(pkt->pid, 456, "default resolution still accepts procfs entries");
    entry.type = FLOW_CLASSIFICATION_ENTRY;
    entry.owner_sk = NULL;
    ret = bpf_map_update_elem(&flow_pid, &route, &entry, BPF_ANY);
    assert_zero(ret, "failed to register ownerless route");
    resolve_pid(NULL, pkt, PID_RESOLUTION_STRICT_INGRESS);
    assert_zero(pkt->pid, "an ownerless route must not match");
    resolve_pid(NULL, pkt, PID_RESOLUTION_DEFAULT);
    assert_equals(pkt->pid, 456, "default resolution still accepts ownerless routes");

    entry.owner_sk = (struct sock *)1;
    entry.pid = 2;
    ret = bpf_map_update_elem(&flow_pid, &route, &entry, BPF_ANY);
    assert_zero(ret, "failed to register kernel thread route");
    resolve_pid(NULL, pkt, PID_RESOLUTION_STRICT_INGRESS);
    assert_zero(pkt->pid, "a kernel thread cannot be a credential recipient");

    return 1;
}

SEC("test/credential_recipient_socket")
int test_credential_recipient_socket(struct baloum_ctx *ctx) {
    struct packet_t *pkt = get_packet();
    assert_not_null(pkt, "unable to get packet");
    pkt->ns_flow.netns = 42;
    pkt->translated_ns_flow = (struct namespaced_flow_t){
        .netns = 42,
        .flow = {
            .saddr = {htonl(0xa9feaa02), 0},
            .daddr = {htonl(0x0a000005), 0},
            .tcp_udp = {.sport = htons(80), .dport = htons(44000)},
            .l3_protocol = ETH_P_IP,
            .l4_protocol = IPPROTO_TCP,
        },
    };
    if (ctx->arg1) {
        pkt->ns_flow.netns = 43;
    }
    if (ctx->arg0) {
        pkt->translated_ns_flow.flow.l3_protocol = ETH_P_IPV6;
        pkt->translated_ns_flow.flow.saddr[0] = bpf_htonll(0xfd000ec200000000ULL);
        pkt->translated_ns_flow.flow.saddr[1] = bpf_htonll(0x23);
        pkt->translated_ns_flow.flow.daddr[0] = bpf_htonll(0xfd00000000000000ULL);
        pkt->translated_ns_flow.flow.daddr[1] = bpf_htonll(5);
    }
    struct pid_route_t route = {
        .addr = {pkt->translated_ns_flow.flow.daddr[0], pkt->translated_ns_flow.flow.daddr[1]},
        .netns = 42,
        .port = htons(44000),
        .l4_protocol = IPPROTO_TCP,
    };
    struct pid_route_entry_t entry = {.owner_sk = (struct sock *)1, .pid = 789, .type = FLOW_CLASSIFICATION_ENTRY};
    int ret = bpf_map_update_elem(&flow_pid, &route, &entry, BPF_ANY);
    assert_zero(ret, "failed to seed legacy fallback");
    pkt->network_direction = INGRESS;
    pkt->pid = 123;
    resolve_pid((struct __sk_buff *)ctx, pkt, PID_RESOLUTION_STRICT_INGRESS);
    return pkt->pid;
}

#endif
