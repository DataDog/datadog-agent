#ifndef __PORT_RANGE_H
#define __PORT_RANGE_H

// TODO: Replace those by injected constants based on system configuration
// once we have port range detection merged into the codebase.
#define EPHEMERAL_RANGE_BEG 32768
#define EPHEMERAL_RANGE_END 60999

static __always_inline int is_ephemeral_port(u16 port) {
    return port >= EPHEMERAL_RANGE_BEG && port <= EPHEMERAL_RANGE_END;
}

static __always_inline void flip_tuple(conn_tuple_t *t) {
    __u16 tmp_port = t->sport;
    t->sport = t->dport;
    t->dport = tmp_port;

    __u64 tmp_ip_part = t->saddr_l;
    t->saddr_l = t->daddr_l;
    t->daddr_l = tmp_ip_part;

    tmp_ip_part = t->saddr_h;
    t->saddr_h = t->daddr_h;
    t->daddr_h = tmp_ip_part;
}

// ensure that the given tuple is in the (src: client, dst: server) format based
// on the port range heuristic
static __always_inline void normalize_tuple(conn_tuple_t *t) {
    if (is_ephemeral_port(t->sport) && !is_ephemeral_port(t->dport)) {
        return;
    }

    if ((!is_ephemeral_port(t->sport) && is_ephemeral_port(t->dport)) || t->sport > t->dport) {
        // flip the tuple if:
        // 1) the tuple is currently in the (server, client) format;
        // 2) unlikely: if both ports are in the same range we ensure that sport < dport to make
        // this function return a deterministic result for a given pair of ports;
        flip_tuple(t);
    }
}

#endif
