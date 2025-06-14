#ifndef _HELPERS_NETWORK_ROUTER_H_
#define _HELPERS_NETWORK_ROUTER_H_

#include "stats.h"
#include "maps.h"

__attribute__((always_inline)) int route_pkt(struct __sk_buff *skb, struct packet_t *pkt, int direction) {
    if (is_network_flow_monitor_enabled()) {
        count_pkt(skb, pkt);
    }

    return ACT_OK;
}

#endif
