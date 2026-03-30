#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "map-defs.h"
#include "socket-contention-kern-user.h"
#include "bpf_metadata.h"

BPF_HASH_MAP(socket_contention_stats, __u32, struct socket_contention_stats, 1)

SEC("kprobe/sock_init_data")
int BPF_KPROBE(kprobe__sock_init_data, struct socket *sock, struct sock *sk)
{
    __u32 key = 0;
    struct socket_contention_stats *stats = bpf_map_lookup_elem(&socket_contention_stats, &key);
    if (!stats) {
        struct socket_contention_stats zero = {};

        bpf_map_update_elem(&socket_contention_stats, &key, &zero, BPF_NOEXIST);
        stats = bpf_map_lookup_elem(&socket_contention_stats, &key);
        if (!stats) {
            return 0;
        }
    }

    __sync_fetch_and_add(&stats->socket_inits, 1);
    return 0;
}

char _license[] SEC("license") = "GPL";
