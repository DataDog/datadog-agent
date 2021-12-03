#ifndef __SOCKFD_SHARED_MAPS_H
#define __SOCKFD_SHARED_MAPS_H

#include "tracer.h"
#include "bpf_helpers.h"
#include <linux/types.h>

struct bpf_map_def SEC("maps/sock_by_pid_fd") sock_by_pid_fd = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(pid_fd_t),
    .value_size = sizeof(struct sock*),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

#endif //__SOCKFD_SHARED_MAPS_H
