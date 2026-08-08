#ifndef _HOOKS_ACCEPT_H_
#define _HOOKS_ACCEPT_H_

#include "constants/offsets/network.h"
#include "helpers/span_fill.h"

int __attribute__((always_inline)) read_sock_and_send_event(ctx_t * ctx, struct sock * sock) {
    if(sock == NULL) {
        return 0;
    }

    struct accept_event_t *event = SPAN_FILL_EVENT(struct accept_event_t, EVENT_ACCEPT);
    if (!event) {
        return 0;
    }

    // Extract family from the socket
    struct sock_common *sockcommon = (void *)sock;
    event->family = get_family_from_sock_common(sockcommon);
    // Only handle AF_INET and AF_INET6
    if (event->family != AF_INET && event->family != AF_INET6) {
        return 0;
    }

    // Read the listening port and source address
    bpf_probe_read(&event->port, sizeof(event->port), &sockcommon->skc_num);
    event->port = htons(event->port);

    if (event->family == AF_INET) {
        bpf_probe_read(&event->addr[0], sizeof(event->addr[0]), &sockcommon->skc_daddr);
    } else if (event->family == AF_INET6) {
        bpf_probe_read((void*)&event->addr, sizeof(sockcommon->skc_v6_daddr), &sockcommon->skc_v6_daddr);
    }

    struct proc_cache_t *entry = fill_process_context(&event->process);
    fill_cgroup_context(entry, &event->cgroup);
    bpf_tail_call_compat(ctx, &span_fill_progs, 0);

    return 0;
}

HOOK_EXIT("inet_csk_accept")
int hook_accept(ctx_t *ctx) {
    struct sock *sock = (struct sock*)CTX_PARMRET(ctx);
    return read_sock_and_send_event(ctx, sock);
}

#endif /* _HOOKS_ACCEPT_H_ */
