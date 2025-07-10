#ifndef _HOOKS_SETSOCKOPT_H_
#define _HOOKS_SETSOCKOPT_H_

#include "constants/syscall_macro.h"
#include "helpers/syscalls.h"
#include <uapi/linux/filter.h>
long __attribute__((always_inline)) trace__sys_setsock_opt(u8 async, int socket_fd, int level, int optname) {
    if (is_discarded_by_pid()) {
        return 0;
    }
    switch (optname) {
        case SO_ATTACH_FILTER: {
            struct policy_t policy = fetch_policy(EVENT_SETSOCKOPT);
            struct syscall_cache_t syscall = {
                .type = EVENT_SETSOCKOPT,
                .policy = policy,
                .async = async,
                .setsockopt = {
                    .level = level,
                    .optname = optname,
                }
            };

            cache_syscall(&syscall);
            return 0;
        }
        default:
            return 0; // unsupported optname
    }
}

int __attribute__((always_inline)) sys_set_sock_opt_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_SETSOCKOPT);
    if (!syscall) {
        return 0;
    }
    int key = 0;
    struct setsockopt_event_t *event = bpf_map_lookup_elem(&setsockopt_event,&key);

    if (!event) {
    return 0;  
}
    event->syscall.retval = retval;
    event->event.flags = syscall->async ? EVENT_FLAGS_ASYNC : 0;
    event->socket_type = syscall->setsockopt.socket_type;
    event->socket_protocol = syscall->setsockopt.socket_protocol;
    event->socket_family = syscall->setsockopt.socket_family;
    event->level = syscall->setsockopt.level;
    event->optname = syscall->setsockopt.optname;
    event->filter_len = syscall->setsockopt.filter_len;
    event->truncated = syscall->setsockopt.truncated;
    struct proc_cache_t *entry = fill_process_context(&event->process);
    fill_container_context(entry, &event->container);
    fill_span_context(&event->span);
    int size_to_sent = (syscall->setsockopt.filter_size_to_send >= MAX_BPF_FILTER_SIZE )
        ? MAX_BPF_FILTER_SIZE
        : syscall->setsockopt.filter_size_to_send;
    event->sent_size = size_to_sent;
    send_event_with_size_ptr(ctx, EVENT_SETSOCKOPT, event, (offsetof(struct setsockopt_event_t, bpf_filters_buffer) + size_to_sent));
    

    return 0;
}

HOOK_SYSCALL_ENTRY3(setsockopt, int, socket, int, level, int, optname) {
    return trace__sys_setsock_opt(SYNC_SYSCALL, socket, level, optname);
}

HOOK_SYSCALL_EXIT(setsockopt) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_set_sock_opt_ret(ctx, retval);
}
HOOK_ENTRY("sk_attach_filter")
int hook_sk_attach_filter(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_SETSOCKOPT);
    if (!syscall) {
        return 0;
    }
    // We assume that optname is always SO_ATTACH_FILTER here
    struct sock_fprog *fprog = (struct sock_fprog *)CTX_PARM1(ctx);
    syscall->setsockopt.fprog = fprog;
    return 0;
}

HOOK_ENTRY("security_socket_setsockopt")
int hook_security_socket_setsockopt(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_SETSOCKOPT);

    if (!syscall) {
        return 0;
    }
    // We assume that optname is always SO_ATTACH_FILTER
    struct socket *sock = (struct socket *)CTX_PARM1(ctx);
    short socket_type;
    bpf_probe_read(&socket_type, sizeof(socket_type), &sock->type);
    if (socket_type) {
        syscall->setsockopt.socket_type = socket_type;
    }
    return 0;
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_setsockopt_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_set_sock_opt_ret(args, args->ret);
}
HOOK_ENTRY("release_sock")
int hook_release_sock(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_SETSOCKOPT);
    if (!syscall) {
        return 0;
    }
    struct sock *sk = (struct sock *)CTX_PARM1(ctx);
    struct sock_common *sockcommon = (void *)sk;
    u16 socket_family = get_family_from_sock_common(sockcommon);
    u16 socket_protocol = get_protocol_from_sock(sk);

    syscall->setsockopt.socket_protocol = socket_protocol;
    syscall->setsockopt.socket_family = socket_family;

    return 0;
}
HOOK_EXIT("release_sock")
int rethook_release_sock(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_SETSOCKOPT);
    if (!syscall) {
        return 0;
    }
    struct sock_fprog prog;
    int ret = bpf_probe_read(&prog, sizeof(struct sock_fprog), syscall->setsockopt.fprog);

    if (ret < 0) {
        return 0;
    }
    syscall->setsockopt.filter_len = (unsigned short) prog.len;

    syscall->setsockopt.filter_size_to_send = (sizeof(struct sock_filter) * prog.len);
    int key = 0;
    struct setsockopt_event_t *event = bpf_map_lookup_elem(&setsockopt_event, &key);

    if (!event) {
        return 0;
    }
    if (syscall->setsockopt.filter_size_to_send >= MAX_BPF_FILTER_SIZE) {
        bpf_probe_read(&event->bpf_filters_buffer, MAX_BPF_FILTER_SIZE, prog.filter); 
        syscall->setsockopt.truncated = 1;
        syscall->setsockopt.filter_size_to_send = MAX_BPF_FILTER_SIZE;
    }
    else if (syscall->setsockopt.filter_size_to_send >= sizeof(struct sock_filter)) {
        bpf_probe_read(&event->bpf_filters_buffer, syscall->setsockopt.filter_size_to_send, prog.filter); 
        syscall->setsockopt.truncated = 0;
    }
    else {
        syscall->setsockopt.truncated = 0;
        syscall->setsockopt.filter_size_to_send = 0;
    }

    return 0;
}
#endif
