#ifndef _HOOKS_SETSOCKOPT_H_
#define _HOOKS_SETSOCKOPT_H_

#include "constants/syscall_macro.h"
#include "helpers/syscalls.h"

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
    // // BEGIN
    // bpf_printk("EXIT SYSCALL | Fprog ptr received from cache: %p", syscall->setsockopt.fprog);
    // struct sock_fprog prog;
    // int ret = bpf_probe_read(&prog, sizeof(struct sock_fprog), syscall->setsockopt.fprog);
    // bpf_printk("EXIT SYSCALL | Fprog: %d", prog);

    // if (ret < 0) {
    //     bpf_printk("EXIT SYSCALL | Failed to read sock_fprog: %d", ret);
    //     return 0;
    // }
    // struct sock_filter filter;
    // bpf_printk("EXIT SYSCALL | Filter ptr: %p", prog.filter);
    // ret = bpf_probe_read(&filter, sizeof(struct sock_filter),prog.filter);
    // if (ret < 0) {
    //     bpf_printk("EXIT SYSCALL | Failed to read sock_filter: %d", ret);
    //     return 0;
    // }
    // unsigned short code = filter.code;
    // bpf_printk("EXIT SYSCALL | Filter code: %u", code);
    // // END

    int key = 0;
    struct setsockopt_event_t *event = bpf_map_lookup_elem(&setsockopt_event,&key);

    if (!event) {
    return 0;  
}
    
    event->syscall.retval = retval;
    event->event.flags = syscall->async ? EVENT_FLAGS_ASYNC : 0;
    event->socket_type = syscall->setsockopt.socket_type;
    event->sk_protocol = syscall->setsockopt.sk_protocol;
    event->level = syscall->setsockopt.level;
    event->optname = syscall->setsockopt.optname;
    event->filter_len = syscall->setsockopt.filter_len;
    struct proc_cache_t *entry = fill_process_context(&event->process);
    fill_container_context(entry, &event->container);
    fill_span_context(&event->span);
    
    if (syscall->setsockopt.filter_len > MAX_BPF_FILTER_LEN){
        return 0;
    }
    // & (MAX_BPF_FILTER_LEN - 1
    send_event_with_size_ptr(ctx, EVENT_SETSOCKOPT, event, (offsetof(struct setsockopt_event_t, bpf_filters_buffer) + sizeof(struct sock_filter) * syscall->setsockopt.filter_len) );

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_SETSOCKOPT);
    

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
    // We assume that optname is always SO_ATTACH_FILTER
    struct sock_fprog *fprog = (struct sock_fprog *)CTX_PARM1(ctx);
    bpf_printk("ENTRY ATTACH_FILTER | Fprog ptr sent in cache: %p", fprog);
    syscall->setsockopt.fprog = fprog;
    // // BEGIN
    // struct sock_fprog prog;
    // int ret = bpf_probe_read(&prog, sizeof(struct sock_fprog), syscall->setsockopt.fprog);
    // bpf_printk("ENTRY ATTACH_FILTER | Fprog: %d", prog);

    // if (ret < 0) {
    //     bpf_printk("ENTRY ATTACH_FILTER | Failed to read sock_fprog: %d", ret);
    //     return 0;
    // }
    // struct sock_filter filter;
    // bpf_printk("ENTRY ATTACH_FILTER | Filter ptr: %p", prog.filter);
    // ret = bpf_probe_read(&filter, sizeof(struct sock_filter),prog.filter);
    // if (ret < 0) {
    //     bpf_printk("ENTRY ATTACH_FILTER | Failed to read sock_filter: %d", ret);
    //     return 0;
    // }
    // unsigned short code = filter.code;
    // bpf_printk("ENTRY ATTACH_FILTER | Filter code: %u", code);
    // // END
    return 0;
}


HOOK_EXIT("sk_attach_filter")
int rethook_sk_attach_filter(ctx_t *ctx) {
    // bpf_printk("EXIT ATTACH_FILTER | retval: %d", CTX_PARMRET(ctx)); 
    // struct syscall_cache_t *syscall = peek_syscall(EVENT_SETSOCKOPT);
    // if (!syscall) {
    //     return 0;
    // }
    // // We assume that optname is always SO_ATTACH_FILTER
    // bpf_printk("EXIT ATTACH_FILTER | Fprog ptr sent in cache: %p", syscall->setsockopt.fprog);
    
    // // BEGIN
    // struct sock_fprog prog;
    // int ret = bpf_probe_read(&prog, sizeof(struct sock_fprog), syscall->setsockopt.fprog);
    // bpf_printk("EXIT ATTACH_FILTER | Fprog: %d", prog);

    // if (ret < 0) {
    //     bpf_printk("EXIT ATTACH_FILTER | Failed to read sock_fprog: %d", ret);
    //     return 0;
    // }
    // struct sock_filter filter;
    // bpf_printk("EXIT ATTACH_FILTER | Filter ptr: %p", prog.filter);
    // ret = bpf_probe_read(&filter, sizeof(struct sock_filter),prog.filter);
    // if (ret < 0) {
    //     bpf_printk("EXIT ATTACH_FILTER | Failed to read sock_filter: %d", ret);
    //     return 0;
    // }
    // unsigned short code = filter.code;
    // bpf_printk("EXIT ATTACH_FILTER | Filter code: %u", code);
    // // END
    return 0;}

HOOK_ENTRY("security_socket_setsockopt")
int hook_security_socket_setsockopt(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_SETSOCKOPT);

    if (!syscall) {
        return 0;
    }
    // We assume that optname is always SO_ATTACH_FILTER
    struct socket *sock = (struct socket *)CTX_PARM1(ctx);
    short socket_type = 0;
    bpf_probe_read(&socket_type, sizeof(socket_type), &sock->type);
    if (socket_type) {
        syscall->setsockopt.socket_type = socket_type;
    }
    return 0;
}

HOOK_ENTRY("bpf_prog_put")
int hook_bpf_prog_put(ctx_t *ctx){
    return 0;
}

HOOK_ENTRY("bpf_prog_free")
int hook_bpf_prog_free(ctx_t *ctx){
    return 0;
}

HOOK_ENTRY("sock_setsockopt")
int hook_sock_setsockopt(ctx_t *ctx) {
    // struct syscall_cache_t *syscall = peek_syscall(EVENT_SETSOCKOPT);
    // if (!syscall) {
    //     return 0;
    // }
    // // We assume that optname is always SO_ATTACH_FILTER
    // bpf_printk("ENTRY SOCK_SETSOCKOPT | Fprog ptr sent in cache: %p", syscall->setsockopt.fprog);
    // // BEGIN
    // struct sock_fprog prog;
    // int ret = bpf_probe_read(&prog, sizeof(struct sock_fprog), syscall->setsockopt.fprog);
    // bpf_printk("ENTRY SOCK_SETSOCKOPT | Fprog: %d", prog);

    // if (ret < 0) {
    //     bpf_printk("ENTRY SOCK_SETSOCKOPT | Failed to read sock_fprog: %d", ret);
    //     return 0;
    // }
    // struct sock_filter filter;
    // bpf_printk("ENTRY SOCK_SETSOCKOPT | Filter ptr: %p", prog.filter);
    // ret = bpf_probe_read(&filter, sizeof(struct sock_filter),prog.filter);
    // if (ret < 0) {
    //     bpf_printk("ENTRY SOCK_SETSOCKOPT | Failed to read sock_filter: %d", ret);
    //     return 0;
    // }
    // unsigned short code = filter.code;
    // bpf_printk("ENTRY SOCK_SETSOCKOPT | Filter code: %u", code);
    // // END


    return 0;
}
HOOK_EXIT("sock_setsockopt")
int rethook_sock_setsockopt(ctx_t *ctx) {
    // bpf_printk("EXIT SOCK_SETSOCKOPT | retval: %d", CTX_PARMRET(ctx)); 
    // struct syscall_cache_t *syscall = peek_syscall(EVENT_SETSOCKOPT);
    // if (!syscall) {
    //     return 0;
    // }
    // // We assume that optname is always SO_ATTACH_FILTER
    // bpf_printk("EXIT SOCK_SETSOCKOPT | Fprog ptr sent in cache: %p", syscall->setsockopt.fprog);    // BEGIN
    // struct sock_fprog prog;
    // int ret = bpf_probe_read(&prog, sizeof(struct sock_fprog), syscall->setsockopt.fprog);
    // bpf_printk("EXIT SOCK_SETSOCKOPT | Fprog: %d", prog);

    // if (ret < 0) {
    //     bpf_printk("EXIT SOCK_SETSOCKOPT | Failed to read sock_fprog: %d", ret);
    //     return 0;
    // }
    // struct sock_filter filter;
    // bpf_printk("EXIT SOCK_SETSOCKOPT | Filter ptr: %p", prog.filter);
    // ret = bpf_probe_read(&filter, sizeof(struct sock_filter),prog.filter);
    // if (ret < 0) {
    //     bpf_printk("EXIT SOCK_SETSOCKOPT | Failed to read sock_filter: %d", ret);
    //     return 0;
    // }
    // unsigned short code = filter.code;
    // bpf_printk("EXIT SOCK_SETSOCKOPT | Filter code: %u", code);
    // // END
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
    } // checkez ça pour sk

    u16 sk_protocol;
    bpf_probe_read(&sk_protocol, sizeof(u16), (struct sock *)CTX_PARM1(ctx) + offsetof(struct sock, sk_protocol));
    syscall->setsockopt.sk_protocol = sk_protocol;


    // bpf_printk("ENTRY RELEASE_SOCK | Fprog ptr sent in cache: %p", syscall->setsockopt.fprog);
    // // BEGIN
    // struct sock_fprog prog;
    // int ret = bpf_probe_read(&prog, sizeof(struct sock_fprog), syscall->setsockopt.fprog);
    // bpf_printk("ENTRY RELEASE_SOCK | Fprog: %d", prog);

    // if (ret < 0) {
    //     bpf_printk("ENTRY RELEASE_SOCK | Failed to read sock_fprog: %d", ret);
    //     return 0;
    // }
    // struct sock_filter filter;
    // bpf_printk("ENTRY RELEASE_SOCK | Filter ptr: %p", prog.filter);
    // ret = bpf_probe_read(&filter, sizeof(struct sock_filter),prog.filter);
    // if (ret < 0) {
    //     bpf_printk("ENTRY RELEASE_SOCK | Failed to read sock_filter: %d", ret);
    //     return 0;
    // }
    // unsigned short code = filter.code;
    // bpf_printk("ENTRY RELEASE_SOCK | Filter code: %u", code);
    // // END
    return 0;
}
HOOK_EXIT("release_sock")
int rethook_release_sock(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_SETSOCKOPT);
    if (!syscall) {
        return 0;
    }
    
    bpf_printk("EXIT RELEASE_SOCK | Fprog ptr sent in cache: %p", syscall->setsockopt.fprog);
    // BEGIN
    struct sock_fprog prog;
    int ret = bpf_probe_read(&prog, sizeof(struct sock_fprog), syscall->setsockopt.fprog);
    bpf_printk("EXIT RELEASE_SOCK | Fprog: %d", prog);

    if (ret < 0) {
        bpf_printk("EXIT RELEASE_SOCK | Failed to read sock_fprog: %d", ret);
        return 0;
    }
    unsigned int prog_len = prog.len;
    syscall->setsockopt.filter_len = prog_len;
    // Iterate over each filter and do something (example: print code)
    int key = 0;
    struct setsockopt_event_t *event = bpf_map_lookup_elem(&setsockopt_event, &key);
    if (!event) {
    return 0; 
        }
    if (prog.len > MAX_BPF_FILTER_LEN){
        return 0;
    }
    ret = bpf_probe_read(&event->bpf_filters_buffer, sizeof(struct sock_filter) * prog.len , prog.filter);

    if (ret < 0) {
        bpf_printk("EXIT RELEASE_SOCK | Failed to read sock_filter: %d", ret);
        return 0;
    }


    // END


    return 0;
}

#endif
