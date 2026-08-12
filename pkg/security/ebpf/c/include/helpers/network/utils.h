#ifndef _HELPERS_NETWORK_UTILS_H_
#define _HELPERS_NETWORK_UTILS_H_

__attribute__((always_inline)) void flip(struct flow_t *flow) {
    u64 tmp = 0;
    tmp = flow->tcp_udp.sport;
    flow->tcp_udp.sport = flow->tcp_udp.dport;
    flow->tcp_udp.dport = tmp;

    tmp = flow->saddr[0];
    flow->saddr[0] = flow->daddr[0];
    flow->daddr[0] = tmp;

    tmp = flow->saddr[1];
    flow->saddr[1] = flow->daddr[1];
    flow->daddr[1] = tmp;
}

// addr holds an in6_addr read as two host-order u64: ::ffff:0:0/96 puts the 0xffff marker in the
// low half of the second one
__attribute__((always_inline)) u8 is_ipv4_mapped_ipv6_addr(u64 *addr) {
    return addr[0] == 0 && (addr[1] & 0xffffffff) == 0xffff0000;
}

#endif
