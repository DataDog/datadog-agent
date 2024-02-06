#ifndef __SOCKFD_H
#define __SOCKFD_H

#include "ktypes.h"
#include "bpf_builtins.h"
#include "map-defs.h"

#include "pid_fd.h"

BPF_HASH_MAP(sock_by_pid_fd, pid_fd_t, struct sock *, 1024)

BPF_HASH_MAP(pid_fd_by_sock, struct sock *, pid_fd_t, 1024)

// On older kernels, clang can generate Wunused-function warnings on static inline functions defined in
// header files, even if they are later used in source files. __maybe_unused prevents that issue
__maybe_unused static __always_inline void clear_sockfd_maps(struct sock* sock) {
    if (sock == NULL) {
        return;
    }

    pid_fd_t* pid_fd = bpf_map_lookup_elem(&pid_fd_by_sock, &sock);
    if (pid_fd == NULL) {
        return;
    }

    // Copy map value to stack before re-using it (needed for Kernel 4.4)
    pid_fd_t pid_fd_copy = {};
    bpf_memcpy(&pid_fd_copy, pid_fd, sizeof(pid_fd_t));
    pid_fd = &pid_fd_copy;

    bpf_map_delete_elem(&sock_by_pid_fd, pid_fd);
    bpf_map_delete_elem(&pid_fd_by_sock, &sock);
}

#endif
