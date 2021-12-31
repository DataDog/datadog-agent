#ifndef __SOCKFD_H
#define __SOCKFD_H

#include "tracer.h"
#include <linux/types.h>

// This map is used to to temporarily store function arguments (sockfd) for
// sockfd_lookup_light function calls, so they can be acessed by the corresponding kretprobe.
// * Key is the pid_tgid;
// * Value the socket FD;
struct bpf_map_def SEC("maps/sockfd_lookup_args") sockfd_lookup_args = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64),
    .value_size = sizeof(__u32),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/sock_by_pid_fd") sock_by_pid_fd = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(pid_fd_t),
    .value_size = sizeof(struct sock*),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/pid_fd_by_sock") pid_fd_by_sock = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(struct sock*),
    .value_size = sizeof(pid_fd_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

static __always_inline void clear_sockfd_maps(struct sock* sock) {
    if (sock == NULL) {
        return;
    }

    pid_fd_t* pid_fd = bpf_map_lookup_elem(&pid_fd_by_sock, &sock);
    if (pid_fd == NULL) {
        return;
    }

    // Copy map value to stack before re-using it (needed for Kernel 4.4)
    pid_fd_t pid_fd_copy = {};
    __builtin_memcpy(&pid_fd_copy, pid_fd, sizeof(pid_fd_t));
    pid_fd = &pid_fd_copy;

    bpf_map_delete_elem(&sock_by_pid_fd, pid_fd);
    bpf_map_delete_elem(&pid_fd_by_sock, &sock);
}

#endif
