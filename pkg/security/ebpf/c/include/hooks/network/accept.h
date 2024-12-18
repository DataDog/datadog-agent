#ifndef _HOOKS_ACCEPT_H_
#define _HOOKS_ACCEPT_H_

#include "constants/offsets/network.h"

int __attribute__((always_inline)) read_sock_and_send_event(ctx_t * ctx, struct sock * sock) {
    struct accept_event_t event = {0};

    // Extract family from the socket
    struct sock_common *sockcommon = (void *)sock;
    event.family = get_family_from_sock_common(sockcommon);
    // Only handle AF_INET and AF_INET6
    if (event.family != AF_INET && event.family != AF_INET6) {
        return 0;
    }

    // Read the listening port and source address
    if (event.family == AF_INET) {
        bpf_probe_read(&event.port, sizeof(event.port), &sockcommon->skc_num);
        bpf_probe_read(&event.addr[0], sizeof(event.addr[0]), &sockcommon->skc_daddr);
    } else if (event.family == AF_INET6) {
        bpf_probe_read(&event.port, sizeof(event.port), &sockcommon->skc_num);
        bpf_probe_read((void*)&event.addr, sizeof(sockcommon->skc_v6_daddr), &sockcommon->skc_v6_daddr);
    }

    event.port = htons(event.port);

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    // Check if we should sample this event for activity dumps
    struct activity_dump_config *config = lookup_or_delete_traced_pid(event.process.pid, bpf_ktime_get_ns(), NULL);
    if (config) {
      if (mask_has_event(config->event_mask, EVENT_ACCEPT)) {
          event.event.flags |= EVENT_FLAGS_ACTIVITY_DUMP_SAMPLE;
      }
    }

    send_event(ctx, EVENT_ACCEPT, event);

    return 0;
}

#ifdef USE_FENTRY

HOOK_EXIT("inet_accept")
int hook_accept(ctx_t *ctx) {
    struct file * f = (struct file *)CTX_PARMRET(ctx, 3);

    if(IS_ERR(f)) {
        return 0;
    }

    struct socket *sck = (struct socket*)CTX_PARM2(ctx);
    struct sock *sock = get_sock_from_socket(sck);
    return read_sock_and_send_event(ctx, sock);
}

#else

HOOK_EXIT("inet_csk_accept")
int hook_accept(ctx_t *ctx) {
    struct sock *sock = (struct sock*)PT_REGS_RC(ctx);
    return read_sock_and_send_event(ctx, sock);
}

#endif /* USE_FENTRY */
#endif /* _HOOKS_ACCEPT_H_ */
