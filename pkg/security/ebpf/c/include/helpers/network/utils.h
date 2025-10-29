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

#endif
