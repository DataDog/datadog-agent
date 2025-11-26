#ifndef _HOOKS_SOCKET_H_
#define _HOOKS_SOCKET_H_

long __attribute__((always_inline)) trace__sys_socket(u8 async, u16 domain, u16 type, u16 protocol) {
    if (is_discarded_by_pid()) {
        return 0;
    }

    struct policy_t policy = fetch_policy(EVENT_SOCKET);
    struct syscall_cache_t syscall = {
        .type = EVENT_SOCKET,
        .policy = policy,
        .async = async,
        .socket = {
            .domain = domain,
            .type = type,
            .protocol = protocol,
        }
    };

    cache_syscall(&syscall);

    return 0;
}

int __attribute__((always_inline)) sys_socket_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_SOCKET);
    if (!syscall) {
        return 0;
    }

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    if (approve_syscall(syscall, socket_approvers) == DISCARDED) {
        return 0;
    }

    struct socket_event_t event = {
        .syscall.retval = retval,
        .domain = syscall->socket.domain,
        .type = syscall->socket.type,
        .protocol = syscall->socket.protocol,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_cgroup_context(entry, &event.cgroup);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_SOCKET, event);
    return 0;
}

HOOK_SYSCALL_ENTRY3(socket, int, domain, int, type, int, protocol) {
    // Mask out SOCK_CLOEXEC / SOCK_NONBLOCK flags to get just the socket type
    // (SOCK_STREAM=1, SOCK_DGRAM=2, ... fit in 4 bits)
    u16 socket_type = (u16)type & 0x0F;
    return trace__sys_socket(SYNC_SYSCALL, (u16)domain, socket_type, (u16)protocol);
}

HOOK_SYSCALL_EXIT(socket) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_socket_ret(ctx, retval);
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_socket_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_socket_ret(args, args->ret);
}

SEC("cgroup/sock_create")
int hook_sock_create(struct bpf_sock *ctx) {
    if (ctx->family != AF_INET && ctx->family != AF_INET6) {
        return 1;
    }

    u64 cookie = bpf_get_socket_cookie(ctx);
    if (cookie == 0) {
        return 1;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    if (pid_tgid == 0) {
        return 1;
    }
    u32 tgid = pid_tgid >> 32;

    bpf_map_update_elem(&sock_cookie_pid, &cookie, &tgid, BPF_ANY);

    return 1;
}

SEC("cgroup/sock_release")
int hook_sock_release(struct bpf_sock *ctx)
{
    u64 cookie = bpf_get_socket_cookie(ctx);
    bpf_map_delete_elem(&sock_cookie_pid, &cookie);
    return 1;
}

#endif