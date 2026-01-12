#ifndef _HOOKS_SOCKET_H_
#define _HOOKS_SOCKET_H_

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