#ifndef __TRACER_PORT_H
#define __TRACER_PORT_H

#include "tracer/tracer.h"

#define add_port_bind(pb, pb_map)                                           \
    do {                                                                    \
        int err;                                                            \
        __u32 *port_count = bpf_map_lookup_elem(&pb_map, pb);               \
        if (!port_count) {                                                  \
            __u32 tmpport = 0;                                              \
            err = bpf_map_update_elem(&pb_map, pb, &tmpport, BPF_NOEXIST);  \
            if (err < 0 && err != EEXIST) {                                 \
                record_map_telemetry(pb_map, err);                          \
            }                                                               \
        }                                                                   \
        if (port_count) {                                                   \
            __sync_fetch_and_add(port_count, 1);                            \
        }                                                                   \
    } while (0)

static __always_inline void remove_port_bind(port_binding_t *pb, void *pb_map) {
    __u32 *port_count = bpf_map_lookup_elem(pb_map, pb);
    if (!port_count) {
        return;
    }
    __sync_fetch_and_add(port_count, -1);
    if (*port_count == 0) {
        bpf_map_delete_elem(pb_map, pb);
        log_debug("remove_port_bind: netns=%u port=%u marked as closed", pb->netns, pb->port);
    }
}

#endif
