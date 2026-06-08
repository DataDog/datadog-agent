#ifndef __SK_TRACER_MAPS_H
#define __SK_TRACER_MAPS_H

#include "map-defs.h"
#include "bpf_helpers.h"

#include "defs.h"
#include "tracer/tracer.h"

BPF_SK_STORAGE_MAP(sk_tcp_stats, sk_tcp_stats_t);
BPF_SK_STORAGE_MAP(sk_udp_stats, sk_udp_stats_t);
// will always be upgraded to ringbuffer
BPF_PERF_EVENT_ARRAY_MAP(conn_close_event, __u32);
BPF_HASH_MAP(port_bindings, port_binding_t, __u32, 0)
BPF_HASH_MAP(udp_port_bindings, port_binding_t, __u32, 0)

#endif
