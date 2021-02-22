#include "offset-guess.h"
#include "bpf_helpers.h"
#include <linux/kconfig.h>
#include <net/net_namespace.h>
#include <net/sock.h>
#include <uapi/linux/ptrace.h>
#include <uapi/linux/tcp.h>

/* These maps are used to match the kprobe & kretprobe of connect for IPv6 */
/* This is a key/value store with the keys being a pid
 * and the values being a struct sock *.
 */
struct bpf_map_def SEC("maps/connectsock_ipv6") connectsock_ipv6 = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64),
    .value_size = sizeof(void*),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/tracer_status") tracer_status = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64),
    .value_size = sizeof(tracer_status_t),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

static __always_inline bool proc_t_comm_equals(proc_t a, proc_t b) {
    for (int i = 0; i < TASK_COMM_LEN; i++) {
        if (a.comm[i] != b.comm[i]) {
            return false;
        }
        // if chars equal but a NUL terminator, both strings equal
        if (!a.comm[i]) {
            break;
        }
    }
    return true;
}

static __always_inline bool check_family(struct sock* sk, tracer_status_t* status, u16 expected_family) {
    u16 family = 0;
    bpf_probe_read(&family, sizeof(u16), ((char*)sk) + status->offset_family);
    return family == expected_family;
}

static __always_inline int guess_offsets(tracer_status_t* status, struct sock* skp, struct flowi4 *fl4) {
    u64 zero = 0;

    if (status->state != TRACER_STATE_CHECKING) {
        return 1;
    }

    // Only traffic for the expected process name. Extraneous connections from other processes must be ignored here.
    // Userland must take care to generate connections from the correct thread. In Golang, this can be achieved
    // with runtime.LockOSThread.
    proc_t proc = {};
    bpf_get_current_comm(&proc.comm, sizeof(proc.comm));

    if (!proc_t_comm_equals(status->proc, proc)) {
        return 0;
    }

    tracer_status_t new_status = {};
    // Copy values from status to new_status
    bpf_probe_read(&new_status, sizeof(tracer_status_t), status);
    new_status.state = TRACER_STATE_CHECKED;
    new_status.err = 0;
    bpf_probe_read(&new_status.proc.comm, sizeof(proc.comm), proc.comm);

    possible_net_t* possible_skc_net = NULL;
    u32 possible_netns = 0;
    long ret;

    switch (status->what) {
    case GUESS_SADDR:
        bpf_probe_read(&new_status.saddr, sizeof(new_status.saddr), ((char*)skp) + status->offset_saddr);
        break;
    case GUESS_DADDR:
        bpf_probe_read(&new_status.daddr, sizeof(new_status.daddr), ((char*)skp) + status->offset_daddr);
        break;
    case GUESS_FAMILY:
        bpf_probe_read(&new_status.family, sizeof(new_status.family), ((char*)skp) + status->offset_family);
        break;
    case GUESS_SPORT:
        bpf_probe_read(&new_status.sport, sizeof(new_status.sport), ((char*)skp) + status->offset_sport);
        break;
    case GUESS_DPORT:
        bpf_probe_read(&new_status.dport, sizeof(new_status.dport), ((char*)skp) + status->offset_dport);
        break;
    case GUESS_SADDR_FL4:
        bpf_probe_read(&new_status.saddr_fl4, sizeof(new_status.saddr_fl4), ((char*)fl4) + status->offset_saddr_fl4);
        break;
    case GUESS_DADDR_FL4:
        bpf_probe_read(&new_status.daddr_fl4, sizeof(new_status.daddr_fl4), ((char*)fl4) + status->offset_daddr_fl4);
        break;
    case GUESS_SPORT_FL4:
        bpf_probe_read(&new_status.sport_fl4, sizeof(new_status.sport_fl4), ((char*)fl4) + status->offset_sport_fl4);
        break;
    case GUESS_DPORT_FL4:
        bpf_probe_read(&new_status.dport_fl4, sizeof(new_status.dport_fl4), ((char*)fl4) + status->offset_dport_fl4);
        break;
    case GUESS_NETNS:
        bpf_probe_read(&possible_skc_net, sizeof(possible_net_t*), ((char*)skp) + status->offset_netns);
        // if we get a kernel fault, it means possible_skc_net
        // is an invalid pointer, signal an error so we can go
        // to the next offset_netns
        ret = bpf_probe_read(&possible_netns, sizeof(possible_netns), ((char*)possible_skc_net) + status->offset_ino);
        if (ret == -EFAULT) {
            new_status.err = 1;
            break;
        }
        new_status.netns = possible_netns;
        break;
    case GUESS_RTT:
        bpf_probe_read(&new_status.rtt, sizeof(new_status.rtt), ((char*)skp) + status->offset_rtt);
        bpf_probe_read(&new_status.rtt_var, sizeof(new_status.rtt_var), ((char*)skp) + status->offset_rtt_var);
        break;
    case GUESS_DADDR_IPV6:
        if (!check_family(skp, status, AF_INET6))
            break;

        bpf_probe_read(new_status.daddr_ipv6, sizeof(u32) * 4, ((char*)skp) + status->offset_daddr_ipv6);
        break;
    default:
        // not for us
        return 0;
    }

    bpf_map_update_elem(&tracer_status, &zero, &new_status, BPF_ANY);

    return 0;
}

SEC("kprobe/ip_make_skb")
int kprobe__ip_make_skb(struct pt_regs* ctx) {
    u64 zero = 0;
    tracer_status_t* status = bpf_map_lookup_elem(&tracer_status, &zero);

    if (status == NULL) {
        return 0;
    }

    struct flowi4* fl4 = (struct flowi4*)PT_REGS_PARM2(ctx);
    guess_offsets(status, NULL, fl4);
    return 0;
}

/* Used exclusively for offset guessing */
SEC("kprobe/tcp_getsockopt")
int kprobe__tcp_getsockopt(struct pt_regs* ctx) {
    int level = (int)PT_REGS_PARM2(ctx);
    int optname = (int)PT_REGS_PARM3(ctx);
    if (level != SOL_TCP || optname != TCP_INFO) {
        return 0;
    }

    u64 zero = 0;
    tracer_status_t* status = bpf_map_lookup_elem(&tracer_status, &zero);
    if (status == NULL) {
        return 0;
    }

    struct sock* sk = (struct sock*)PT_REGS_PARM1(ctx);
    status->tcp_info_kprobe_status = 1;
    guess_offsets(status, sk, NULL);

    return 0;
}

// Used for offset guessing (see: pkg/ebpf/offsetguess.go)
SEC("kprobe/tcp_v6_connect")
int kprobe__tcp_v6_connect(struct pt_regs* ctx) {
    struct sock* sk;
    u64 pid = bpf_get_current_pid_tgid();

    sk = (struct sock*)PT_REGS_PARM1(ctx);

    bpf_map_update_elem(&connectsock_ipv6, &pid, &sk, BPF_ANY);

    return 0;
}

// Used for offset guessing (see: pkg/ebpf/offsetguess.go)
SEC("kretprobe/tcp_v6_connect")
int kretprobe__tcp_v6_connect(struct pt_regs* __attribute__((unused)) ctx) {
    u64 pid = bpf_get_current_pid_tgid();
    u64 zero = 0;
    struct sock** skpp;
    tracer_status_t* status;
    skpp = bpf_map_lookup_elem(&connectsock_ipv6, &pid);
    if (skpp == 0) {
        return 0; // missed entry
    }

    bpf_map_delete_elem(&connectsock_ipv6, &pid);

    struct sock* skp = *skpp;

    status = bpf_map_lookup_elem(&tracer_status, &zero);
    if (status == NULL) {
        return 0;
    }

    // We should figure out offsets if they're not already figured out
    guess_offsets(status, skp, NULL);

    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
