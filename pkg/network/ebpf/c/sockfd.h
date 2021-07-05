#ifndef __SOCKFD_H
#define __SOCKFD_H

#include "tracer.h"
#include <linux/types.h>

typedef struct {
    __u32 pid;
    __u32 fd;
} pid_fd_t;

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

struct bpf_map_def SEC("maps/tup_by_pid_fd") tup_by_pid_fd = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(pid_fd_t),
    .value_size = sizeof(conn_tuple_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/pid_fd_by_tup") pid_fd_by_tup = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(conn_tuple_t),
    .value_size = sizeof(pid_fd_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

static __always_inline int socket_to_tuple(struct socket* socket, conn_tuple_t* tuple, u64 pid_tgid, u64 offset_sk, u64 offset_type) {
    struct sock *sock = NULL;
    bpf_probe_read(&sock, sizeof(sock), (char*)socket + offset_sk);

    enum sock_type sock_type = 0;
    bpf_probe_read(&sock_type, sizeof(short), (char*)socket + offset_type);

    metadata_mask_t metadata = 0;
    switch(sock_type) {
    case SOCK_STREAM:
        metadata |= CONN_TYPE_TCP;
        break;
    case SOCK_DGRAM:
        metadata |= CONN_TYPE_UDP;
        break;
    default:
        return 0;
    }

    return read_conn_tuple(tuple, sock, pid_tgid, metadata);
}

#endif
