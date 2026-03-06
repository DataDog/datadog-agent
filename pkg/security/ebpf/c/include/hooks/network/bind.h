#ifndef _HOOKS_BIND_H_
#define _HOOKS_BIND_H_

#include "constants/offsets/netns.h"
#include "constants/syscall_macro.h"
#include "helpers/discarders.h"
#include "helpers/syscalls.h"

int __attribute__((always_inline)) sys_bind(u64 pid_tgid) {
    struct syscall_cache_t syscall = {
        .type = EVENT_BIND,
        .async = pid_tgid ? 1: 0,
        .bind = {
            .pid_tgid = pid_tgid,
        }
    };
    cache_syscall(&syscall);
    return 0;
}

HOOK_SYSCALL_ENTRY3(bind, int, socket, struct sockaddr *, addr, unsigned int, addr_len) {
    if (!addr) {
        return 0;
    }

    return sys_bind(0);
}

int __attribute__((always_inline)) sys_bind_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_BIND);
    if (!syscall) {
        return 0;
    }

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    /* pre-fill the event */
    struct bind_event_t event = {
        .syscall.retval = retval,
        .addr[0] = syscall->bind.addr[0],
        .addr[1] = syscall->bind.addr[1],
        .family = syscall->bind.family,
        .port = syscall->bind.port,
        .protocol = syscall->bind.protocol,
    };

    struct proc_cache_t *entry;
    if (syscall->bind.pid_tgid != 0) {
        entry = fill_process_context_with_pid_tgid(&event.process, syscall->bind.pid_tgid);
    } else {
        entry = fill_process_context(&event.process);
    }
    fill_cgroup_context(entry, &event.cgroup);
    fill_span_context(&event.span);

    // should we sample this event for activity dumps ?
    struct activity_dump_config *config = lookup_or_delete_traced_pid(event.process.pid, bpf_ktime_get_ns(), NULL);
    if (config) {
        if (mask_has_event(config->event_mask, EVENT_BIND)) {
            event.event.flags |= EVENT_FLAGS_ACTIVITY_DUMP_SAMPLE;
        }
    }

    if (!(event.event.flags & EVENT_FLAGS_ACTIVITY_DUMP_SAMPLE)) {
        if (approve_bind_sample(event.process.pid, syscall->bind.family, syscall->bind.port, syscall->bind.protocol)) {
            event.event.flags |= EVENT_FLAGS_ACTIVITY_DUMP_SAMPLE;
        }
    }

    send_event(ctx, EVENT_BIND, event);
    return 0;
}

HOOK_SYSCALL_EXIT(bind) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_bind_ret(ctx, retval);
}

HOOK_ENTRY("io_bind")
int hook_io_bind(ctx_t *ctx) {
    void *raw_req = (void *)CTX_PARM1(ctx);
    u64 pid_tgid = get_pid_tgid_from_iouring(raw_req);
    return sys_bind(pid_tgid);
}

HOOK_EXIT("io_bind")
int rethook_io_bind(ctx_t *ctx) {
    return sys_bind_ret(ctx, CTX_PARMRET(ctx));
}

HOOK_ENTRY("security_socket_bind")
int hook_security_socket_bind(ctx_t *ctx) {
    struct socket *sock = (struct socket *)CTX_PARM1(ctx);
    struct sockaddr *address = (struct sockaddr *)CTX_PARM2(ctx);

    // fill syscall_cache if necessary
    struct syscall_cache_t *syscall = peek_syscall(EVENT_BIND);
    if (!syscall) {
        return 0;
    }

    // Extract IP and port from the sockaddr structure
    bpf_probe_read(&syscall->bind.family, sizeof(syscall->bind.family), &address->sa_family);
    if (syscall->bind.family == AF_INET) {
        struct sockaddr_in *addr_in = (struct sockaddr_in *)address;
        bpf_probe_read(&syscall->bind.port, sizeof(addr_in->sin_port), &addr_in->sin_port);
        bpf_probe_read(&syscall->bind.addr, sizeof(addr_in->sin_addr.s_addr), &addr_in->sin_addr.s_addr);
    } else if (syscall->bind.family == AF_INET6) {
        struct sockaddr_in6 *addr_in6 = (struct sockaddr_in6 *)address;
        bpf_probe_read(&syscall->bind.port, sizeof(addr_in6->sin6_port), &addr_in6->sin6_port);
        bpf_probe_read(&syscall->bind.addr, sizeof(u64) * 2, (char *)addr_in6 + offsetof(struct sockaddr_in6, sin6_addr));
    }
    struct sock *sk = get_sock_from_socket(sock);
    syscall->bind.protocol = get_protocol_from_sock(sk);
    return 0;
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_bind_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_bind_ret(args, args->ret);
}

#endif /* _BIND_H_ */
