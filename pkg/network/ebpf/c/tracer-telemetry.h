#ifndef __TRACER_TELEMETRY_H
#define __TRACER_TELEMETRY_H

#include "tracer-maps.h"

#include "bpf_endian.h"

#include <linux/kconfig.h>
#include <net/sock.h>

enum telemetry_counter {
    tcp_sent_miscounts,
    missed_tcp_close,
    missed_udp_close,
    udp_send_processed,
    udp_send_missed,
};

static __always_inline void increment_telemetry_count(enum telemetry_counter counter_name) {
    __u64 key = 0;
    telemetry_t* val = NULL;
    val = bpf_map_lookup_elem(&telemetry, &key);
    if (val == NULL) {
        return;
    }

    switch (counter_name) {
    case tcp_sent_miscounts:
        __sync_fetch_and_add(&val->tcp_sent_miscounts, 1);
        break;
    case missed_tcp_close:
        __sync_fetch_and_add(&val->missed_tcp_close, 1);
        break;
    case missed_udp_close:
        __sync_fetch_and_add(&val->missed_udp_close, 1);
    case udp_send_processed:
        __sync_fetch_and_add(&val->udp_sends_processed, 1);
        break;
    case udp_send_missed:
        __sync_fetch_and_add(&val->udp_sends_missed, 1);
        break;
    }
    return;
}


static __always_inline void sockaddr_to_addr(struct sockaddr * sa, u64 * addr_h, u64 * addr_l, u16 * port) {
    if (!sa) return;

    u16 family;
    bpf_probe_read(&family, sizeof(family), &sa->sa_family);

    struct sockaddr_in * sin;
    struct sockaddr_in6 * sin6;
    switch (family) {
    case AF_INET:
        sin = (struct sockaddr_in *) sa;
        if (addr_l) {
            bpf_probe_read(addr_l, sizeof(__be32), &(sin->sin_addr.s_addr));
        }
        if (port) {
            bpf_probe_read(port, sizeof(__be16), &sin->sin_port);
            *port = bpf_ntohs(*port);
        }
        break;
    case AF_INET6:
        sin6 = (struct sockaddr_in6 *) sa;
        if (addr_l && addr_h) {
            bpf_probe_read(addr_h, sizeof(u64), sin6->sin6_addr.s6_addr);
            bpf_probe_read(addr_l, sizeof(u64), &(sin6->sin6_addr.s6_addr[8]));
        }
        if (port) {
            bpf_probe_read(port, sizeof(u16), &sin6->sin6_port);
            *port = bpf_ntohs(*port);
        }
        break;
    }
}

#endif // __TRACER_TELEMETRY_H
