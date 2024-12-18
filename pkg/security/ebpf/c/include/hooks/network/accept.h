#ifndef _HOOKS_ACCEPT_H_
#define _HOOKS_ACCEPT_H_

#include "constants/offsets/network.h"

HOOK_EXIT("do_accept")
int hook_do_accept(ctx_t *ctx) {
    struct syscall_cache_t syscall = {
        .type = EVENT_ACCEPT,
    };

    struct file * f = (struct file *)CTX_PARMRET(ctx, 1);

    if(IS_ERR(f)) {
        return 0;
    }

    struct socket * sck;
    bpf_probe_read(&sck, sizeof(sck), &f->private_data);
    struct sock *sock = get_sock_from_socket(sck);

    // Extract family from the socket
    struct sock_common *sockcommon = (void *)sock;
    syscall.accept.family = get_family_from_sock_common(sockcommon);

    // Only handle AF_INET and AF_INET6
    if (syscall.accept.family != AF_INET && syscall.accept.family != AF_INET6) {
        return 0;
    }

    // Read the listening port and source address
    if (syscall.accept.family == AF_INET) {
        bpf_probe_read(&syscall.accept.port, sizeof(syscall.accept.port), &sockcommon->skc_num);
        bpf_probe_read(&syscall.accept.addr[0], sizeof(syscall.accept.addr[0]), &sockcommon->skc_daddr);
    } else if (syscall.accept.family == AF_INET6) {
        bpf_probe_read(&syscall.accept.port, sizeof(syscall.accept.port), &sockcommon->skc_num);
        bpf_probe_read((void*)&syscall.accept.addr, sizeof(sockcommon->skc_v6_daddr), &sockcommon->skc_v6_daddr);
    }

    syscall.accept.port = htons(syscall.accept.port);

    cache_syscall(&syscall);
    return 0;
}

int __attribute__((always_inline)) sys_accept_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_ACCEPT);
    if (!syscall) {
        return 0;
    }

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

   /* pre-fill the event */
   struct accept_event_t event = {
        .syscall.retval = retval,
        .addr[0] = syscall->accept.addr[0],
        .addr[1] = syscall->accept.addr[1],
        .family = syscall->accept.family,
        .port = syscall->accept.port,
    };

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

HOOK_SYSCALL_EXIT(accept) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_accept_ret(ctx, retval);
}

HOOK_SYSCALL_EXIT(accept4) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_accept_ret(ctx, retval);
}

SEC("tracepoint/handle_sys_accept_exit")
int tracepoint_handle_sys_accept_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_accept_ret(args, args->ret);
}

SEC("tracepoint/handle_sys_accept4_exit")
int tracepoint_handle_sys_accept4_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_accept_ret(args, args->ret);
}

#endif /* _HOOKS_ACCEPT_H_ */
