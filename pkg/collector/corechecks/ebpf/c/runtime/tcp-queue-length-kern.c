#include "kconfig.h"
#include "ktypes.h"
#include <linux/tcp.h>

#include "bpf_helpers.h"
#include "map-defs.h"
#include "cgroup.h"
#include "tcp-queue-length-kern-user.h"

#if LINUX_VERSION_CODE < KERNEL_VERSION(4, 8, 0)
// 4.8 is the first version where `bpf_get_current_task` is available
#error Versions of Linux previous to 4.8.0 are not supported by this probe
#endif

/*
 * The `tcp_queue_stats` map is used to share with the userland program system-probe
 * the statistics (max size of receive/send buffer)
 */
BPF_PERCPU_HASH_MAP(tcp_queue_stats, struct stats_key, struct stats_value, 1024)

/*
 * the `who_recvmsg` and `who_sendmsg` maps are used to remind the sock pointer
 * received as input parameter when we are in the kretprobe of tcp_recvmsg and tcp_sendmsg.
 */
BPF_HASH_MAP(who_recvmsg, u64, struct sock *, 100)

BPF_HASH_MAP(who_sendmsg, u64, struct sock *, 100)

// TODO: replace all `bpf_probe_read` by `bpf_probe_read_kernel` once we can assume that we have at least kernel 5.5
static __always_inline int check_sock(struct sock *sk) {
    struct stats_value zero = {
        .read_buffer_max_usage = 0,
        .write_buffer_max_usage = 0
    };

    struct stats_key k;
    get_cgroup_name(k.cgroup_name, sizeof(k.cgroup_name));

    bpf_map_update_elem(&tcp_queue_stats, &k, &zero, BPF_NOEXIST);
    struct stats_value *v = bpf_map_lookup_elem(&tcp_queue_stats, &k);
    if (!v) {
        return 0;
    }

    int rqueue_size, wqueue_size;
    bpf_probe_read(&rqueue_size, sizeof(rqueue_size), (void *)&sk->sk_rcvbuf);
    bpf_probe_read(&wqueue_size, sizeof(wqueue_size), (void *)&sk->sk_sndbuf);

    const struct tcp_sock *tp = tcp_sk(sk);
    u32 rcv_nxt, copied_seq, write_seq, snd_una;
    bpf_probe_read(&rcv_nxt, sizeof(rcv_nxt), (void *)&tp->rcv_nxt); // What we want to receive next
    bpf_probe_read(&copied_seq, sizeof(copied_seq), (void *)&tp->copied_seq); // Head of yet unread data
    bpf_probe_read(&write_seq, sizeof(write_seq), (void *)&tp->write_seq); // Tail(+1) of data held in tcp send buffer
    bpf_probe_read(&snd_una, sizeof(snd_una), (void *)&tp->snd_una); // First byte we want an ack for

    u32 rqueue = rcv_nxt < copied_seq ? 0 : rcv_nxt - copied_seq;
    if (rqueue < 0)
        rqueue = 0;
    u32 wqueue = write_seq - snd_una;

    u32 rqueue_usage = 1000 * rqueue / rqueue_size;
    u32 wqueue_usage = 1000 * wqueue / wqueue_size;

    if (rqueue_usage > v->read_buffer_max_usage)
        v->read_buffer_max_usage = rqueue_usage;
    if (wqueue_usage > v->write_buffer_max_usage)
        v->write_buffer_max_usage = wqueue_usage;

    return 0;
}

SEC("kprobe/tcp_recvmsg")
int kprobe__tcp_recvmsg(struct pt_regs *ctx) {
    struct sock *sk = (struct sock*)PT_REGS_PARM1(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    bpf_map_update_elem(&who_recvmsg, &pid_tgid, &sk, BPF_ANY);
    return check_sock(sk);
}

SEC("kretprobe/tcp_recvmsg")
int kretprobe__tcp_recvmsg(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock **sk = bpf_map_lookup_elem(&who_recvmsg, &pid_tgid);
    bpf_map_delete_elem(&who_recvmsg, &pid_tgid);

    if (sk)
        return check_sock(*sk);
    return 0;
}

SEC("kprobe/tcp_sendmsg")
int kprobe__tcp_sendmsg(struct pt_regs *ctx) {
    struct sock *sk = (struct sock*)PT_REGS_PARM1(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    bpf_map_update_elem(&who_sendmsg, &pid_tgid, &sk, BPF_ANY);

    return check_sock(sk);
}

SEC("kretprobe/tcp_sendmsg")
int kretprobe__tcp_sendmsg(struct pt_regs *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock **sk = bpf_map_lookup_elem(&who_sendmsg, &pid_tgid);
    bpf_map_delete_elem(&who_sendmsg, &pid_tgid);

    if (sk)
        return check_sock(*sk);
    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
