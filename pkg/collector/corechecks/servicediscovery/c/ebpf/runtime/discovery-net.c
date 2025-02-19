#include "ktypes.h"
#include "bpf_metadata.h"

#ifdef COMPILE_RUNTIME
#include "kconfig.h"
#include <linux/ptrace.h>
#endif

#include "discovery-types.h"

#include "pid_tgid.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "map-defs.h"

BPF_HASH_MAP(network_stats, struct network_stats_key, struct network_stats, 1024)

static __always_inline struct network_stats *get_stats() {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = GET_USER_MODE_PID(pid_tgid);
    struct network_stats_key key = { .pid = pid };

    return bpf_map_lookup_elem(&network_stats, &key);
}

SEC("kretprobe/tcp_recvmsg")
int BPF_KRETPROBE(kretprobe__tcp_recvmsg, int bytes) {
    if (bytes <= 0) {
        return 0;
    }

    struct network_stats *stats = get_stats();
    if (!stats) {
        return 0;
    }

    __sync_fetch_and_add(&stats->rx, bytes);

    return 0;
}

static __always_inline void handle_send(int bytes) {
    if (bytes <= 0) {
        return;
    }

    struct network_stats *stats = get_stats();
    if (!stats) {
        return;
    }

    __sync_fetch_and_add(&stats->tx, bytes);
}

SEC("kretprobe/tcp_sendmsg")
int BPF_KRETPROBE(kretprobe__tcp_sendmsg, int bytes) {
    handle_send(bytes);
    return 0;
}

SEC("kretprobe/tcp_sendpage")
int BPF_KRETPROBE(kretprobe__tcp_sendpage, int bytes) {
    handle_send(bytes);
    return 0;
}

char _license[] SEC("license") = "GPL";
