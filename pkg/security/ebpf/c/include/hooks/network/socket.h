#ifndef _HOOKS_SOCKET_H_
#define _HOOKS_SOCKET_H_

#include "constants/offsets/network.h"
#include "helpers/iouring.h"

static long __attribute__((always_inline)) trace__sys_socket(void *ctx, u16 domain, u16 type, u16 protocol, u64 pid_tgid) {
    if (is_discarded_by_pid()) {
        return 0;
    }

    struct policy_t policy = fetch_policy(EVENT_SOCKET);
    struct syscall_cache_t syscall = {
        .type = EVENT_SOCKET,
        .policy = policy,
        .async = pid_tgid ? ASYNC_SYSCALL : SYNC_SYSCALL,
        .socket = {
            .domain = domain,
            .type = type,
            .protocol = protocol,
            .pid_tgid = pid_tgid,
        }
    };

    cache_syscall(ctx, &syscall);

    return 0;
}

static int __attribute__((always_inline)) sys_socket_ret(void *ctx, int retval) {
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
        .event.flags = syscall->async ? EVENT_FLAGS_ASYNC : 0,
        .domain = syscall->socket.domain,
        .type = syscall->socket.type,
        .protocol = syscall->socket.protocol,
    };

    struct proc_cache_t *entry;
    if (syscall->socket.pid_tgid != 0) {
        entry = fill_process_context_with_pid_tgid(&event.process, syscall->socket.pid_tgid);
    } else {
        entry = fill_process_context(&event.process);
    }
    fill_cgroup_context(entry, &event.cgroup);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_SOCKET, event);
    return 0;
}

HOOK_SYSCALL_ENTRY3(socket, int, domain, int, type, int, protocol) {
    // Mask out SOCK_CLOEXEC / SOCK_NONBLOCK flags to get just the socket type
    // (SOCK_STREAM=1, SOCK_DGRAM=2, ... fit in 4 bits)
    u16 socket_type = (u16)type & 0x0F;
    return trace__sys_socket(ctx, (u16)domain, socket_type, (u16)protocol, 0);
}

HOOK_SYSCALL_EXIT(socket) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_socket_ret(ctx, retval);
}

// io_socket (IORING_OP_SOCKET, kernel 5.19+).
HOOK_ENTRY("io_socket")
int hook_io_socket(ctx_t *ctx) {
    void *raw_req = (void *)CTX_PARM1(ctx);

    int domain = 0, type = 0, protocol = 0;
    bpf_probe_read(&domain, sizeof(domain), raw_req + get_io_socket_domain_offset());
    bpf_probe_read(&type, sizeof(type), raw_req + get_io_socket_type_offset());
    bpf_probe_read(&protocol, sizeof(protocol), raw_req + get_io_socket_protocol_offset());

    u64 pid_tgid = get_pid_tgid_from_iouring(raw_req);
    // Mask out SOCK_CLOEXEC / SOCK_NONBLOCK (same as above)
    u16 socket_type = (u16)type & 0x0F;
    return trace__sys_socket(ctx, (u16)domain, socket_type, (u16)protocol, pid_tgid);
}

HOOK_EXIT("io_socket")
int rethook_io_socket(ctx_t *ctx) {
    return sys_socket_ret(ctx, (int)CTX_PARMRET(ctx));
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_socket_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_socket_ret(args, args->ret);
}

SEC("cgroup/sock_create")
static int hook_sock_create(struct bpf_sock *ctx) {
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
static int hook_sock_release(struct bpf_sock *ctx)
{
    u64 cookie = bpf_get_socket_cookie(ctx);
    bpf_map_delete_elem(&sock_cookie_pid, &cookie);
    return 1;
}

#endif
