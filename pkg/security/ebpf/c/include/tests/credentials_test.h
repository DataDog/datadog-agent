#ifndef _CREDENTIALS_TEST_H_
#define _CREDENTIALS_TEST_H_

#include "baloum.h"
#include "bpf_endian.h"
#include "helpers/network/credentials.h"
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

#endif
