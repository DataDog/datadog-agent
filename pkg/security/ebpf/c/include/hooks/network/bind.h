#ifndef _HOOKS_BIND_H_
#define _HOOKS_BIND_H_

#include "constants/offsets/netns.h"
#include "constants/syscall_macro.h"
#include "helpers/discarders.h"
#include "helpers/syscalls.h"

HOOK_SYSCALL_ENTRY3(bind, int, socket, struct sockaddr *, addr, unsigned int, addr_len) {
    if (!addr) {
        return 0;
    }

    struct policy_t policy = fetch_policy(EVENT_BIND);
    if (is_discarded_by_process(policy.mode, EVENT_BIND)) {
        return 0;
    }

    /* cache the bind and wait to grab the retval to send it */
    struct syscall_cache_t syscall = {
        .type = EVENT_BIND,
    };
    cache_syscall(&syscall);
    return 0;
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
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    // should we sample this event for activity dumps ?
    struct activity_dump_config *config = lookup_or_delete_traced_pid(event.process.pid, bpf_ktime_get_ns(), NULL);
    if (config) {
        if (mask_has_event(config->event_mask, EVENT_BIND)) {
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

HOOK_ENTRY("security_socket_bind")
int hook_security_socket_bind(ctx_t *ctx) {
    struct socket *sk = (struct socket *)CTX_PARM1(ctx);
    struct sockaddr *address = (struct sockaddr *)CTX_PARM2(ctx);
    struct pid_route_t key = {};
    u16 family = 0;

    // Extract IP and port from the sockaddr structure
    bpf_probe_read(&family, sizeof(family), &address->sa_family);
    if (family == AF_INET) {
        struct sockaddr_in *addr_in = (struct sockaddr_in *)address;
        bpf_probe_read(&key.port, sizeof(addr_in->sin_port), &addr_in->sin_port);
        bpf_probe_read(&key.addr, sizeof(addr_in->sin_addr.s_addr), &addr_in->sin_addr.s_addr);
    } else if (family == AF_INET6) {
        struct sockaddr_in6 *addr_in6 = (struct sockaddr_in6 *)address;
        bpf_probe_read(&key.port, sizeof(addr_in6->sin6_port), &addr_in6->sin6_port);
        bpf_probe_read(&key.addr, sizeof(u64) * 2, (char *)addr_in6 + offsetof(struct sockaddr_in6, sin6_addr));
    }

    // fill syscall_cache if necessary
    struct syscall_cache_t *syscall = peek_syscall(EVENT_BIND);
    if (syscall) {
        syscall->bind.addr[0] = key.addr[0];
        syscall->bind.addr[1] = key.addr[1];
        syscall->bind.port = key.port;
        syscall->bind.family = family;
    }

    // past this point we care only about AF_INET and AF_INET6
    if (family != AF_INET && family != AF_INET6) {
        return 0;
    }

    // Register service PID
    if (key.port != 0) {
        u64 id = bpf_get_current_pid_tgid();
        u32 tid = (u32)id;

        // add netns information
        key.netns = get_netns_from_socket(sk);
        if (key.netns != 0) {
            bpf_map_update_elem(&netns_cache, &tid, &key.netns, BPF_ANY);
        }

#ifndef DO_NOT_USE_TC
        u32 pid = id >> 32;
        bpf_map_update_elem(&flow_pid, &key, &pid, BPF_ANY);
#endif

#ifdef DEBUG
        bpf_printk("# registered (bind) pid:%d", pid);
        bpf_printk("# p:%d a:%d a:%d", key.port, key.addr[0], key.addr[1]);
#endif
    }
    return 0;
}

SEC("tracepoint/handle_sys_bind_exit")
int tracepoint_handle_sys_bind_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_bind_ret(args, args->ret);
}

#endif /* _BIND_H_ */
