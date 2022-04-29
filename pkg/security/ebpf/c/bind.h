#ifndef _BIND_H_
#define _BIND_H_

struct bind_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    int socket;
    uint16_t addr_family;
    uint16_t addr_port;
    union  {
        uint32_t addr_ip;
        uint8_t  addr_ip6[16];
    };
};

SYSCALL_KPROBE3(bind, int, socket, struct sockaddr*, addr, unsigned int, addr_len) {
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
        .bind = {
            .socket = socket,
            .addr = addr,
            .addr_len = addr_len,
        },
    };
    cache_syscall(&syscall);
    return 0;
}

SYSCALL_KRETPROBE(bind) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_BIND);
    if (!syscall) {
        return 0;
    }

    int retval = PT_REGS_RC(ctx); /* TODO: define which errors we want to discard */

    /* get address family */
    uint16_t addr_family;
    if (bpf_probe_read(&addr_family, sizeof(addr_family), &(syscall->bind.addr->sa_family))) {
        return 0;
    }

    /* pre-fill the event */
    struct bind_event_t event = {
        .syscall.retval = retval,
        .socket = syscall->bind.socket,
        .addr_family = addr_family,
        .addr_port = 0,
    };

    /* get additionnal information, depending on the addr family */
    struct sockaddr_in* sockin;
    struct sockaddr_in6* sockin6;
    switch (addr_family) {
    case AF_INET:
        /* get port */
        sockin = (struct sockaddr_in*)syscall->bind.addr;
        if (bpf_probe_read(&event.addr_port, sizeof(event.addr_port), &(sockin->sin_port))) {
            return 0;
        }
        event.addr_port = ntohs(event.addr_port);

        /* get ip */
        if (bpf_probe_read(&event.addr_ip, sizeof(event.addr_ip), &(sockin->sin_addr))) {
            return 0;
        }
        break;

    case AF_INET6:
        /* get port */
        sockin6 = (struct sockaddr_in6*)syscall->bind.addr;
        if (bpf_probe_read(&event.addr_port, sizeof(event.addr_port), &(sockin6->sin6_port))) {
            return 0;
        }
        event.addr_port = ntohs(event.addr_port);

        /* get addr ip */
        if (bpf_probe_read(&event.addr_ip6, sizeof(event.addr_ip6), &(sockin6->sin6_addr))) {
            return 0;
        }
        break;

    /* TODO: handle other addr family */
    }

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);
    send_event(ctx, EVENT_BIND, event);
    return 0;
}

#endif /* _BIND_H_ */
