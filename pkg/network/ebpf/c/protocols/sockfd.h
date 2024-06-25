#ifndef __SOCKFD_H
#define __SOCKFD_H

#include "ktypes.h"
#include "bpf_builtins.h"
#include "map-defs.h"

#include "pid_fd.h"

// This map is used to to temporarily store function arguments (sockfd) for
// sockfd_lookup_light function calls, so they can be accessed by the corresponding kretprobe.
// * Key is the pid_tgid;
// * Value the socket FD;
BPF_HASH_MAP(sockfd_lookup_args, __u64, __u32, 1024)

BPF_HASH_MAP(tuple_by_pid_fd, pid_fd_t, conn_tuple_t, 1024)

BPF_HASH_MAP(pid_fd_by_tuple, conn_tuple_t, pid_fd_t, 1024)

#endif
