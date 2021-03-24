#include <linux/compiler.h>

#include <linux/kconfig.h>
#include <linux/ptrace.h>
#include <linux/types.h>
#include <linux/version.h>
#include <linux/tcp.h>

#define bpf_printk(fmt, ...)                       \
	({                                             \
		char ____fmt[] = fmt;                      \
		bpf_trace_printk(____fmt, sizeof(____fmt), \
						 ##__VA_ARGS__);           \
	})

#include "bpf_helpers.h"
#include "bpf-common.h"
#include "tcp-queue-length-kern-user.h"

/*
 * The `tcp_queue_stats` map is used to share with the userland program system-probe
 * the statistics (max size of receive/send buffer)
 */

struct bpf_map_def SEC("maps/tcp_queue_stats") tcp_queue_stats = {
    .type = BPF_MAP_TYPE_PERCPU_HASH,
    .key_size = sizeof(struct stats_key),
    .value_size = sizeof(struct stats_value),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

/*
 * the `who_recvmsg` and `who_sendmsg` maps are used to remind the sock pointer
 * received as input parameter when we are in the kretprobe of tcp_recvmsg and tcp_sendmsg.
 */
struct bpf_map_def SEC("maps/who_recvmsg") who_recvmsg = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(struct sock*),
    .max_entries = 100,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/who_sendmsg") who_sendmsg = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(struct sock*),
    .max_entries = 100,
    .pinning = 0,
    .namespace = "",
};

// TODO: replace all `bpf_probe_read` by `bpf_probe_read_kernel` once we can assume that we have at least kernel 5.5
static __always_inline int check_sock(struct sock* sk) {
    struct stats_value zero = {
        .read_buffer_max_usage = 0,
        .write_buffer_max_usage = 0
    };

    struct stats_key k;
    get_cgroup_name(k.cgroup_name, sizeof(k.cgroup_name));

    bpf_map_update_elem(&tcp_queue_stats, &k, &zero, BPF_NOEXIST);
    struct stats_value* v = bpf_map_lookup_elem(&tcp_queue_stats, &k);
    if (!v) {
        return 0;
    }

    int rqueue_size, wqueue_size;
    bpf_probe_read(&rqueue_size, sizeof(rqueue_size), (void*) &sk->sk_rcvbuf);
    bpf_probe_read(&wqueue_size, sizeof(wqueue_size), (void*) &sk->sk_sndbuf);

    const struct tcp_sock* tp = tcp_sk(sk);
    u32 rcv_nxt, copied_seq, write_seq, snd_una;
    bpf_probe_read(&rcv_nxt, sizeof(rcv_nxt), (void*) &tp->rcv_nxt); // What we want to receive next
    bpf_probe_read(&copied_seq, sizeof(copied_seq), (void*) &tp->copied_seq); // Head of yet unread data
    bpf_probe_read(&write_seq, sizeof(write_seq), (void*) &tp->write_seq); // Tail(+1) of data held in tcp send buffer
    bpf_probe_read(&snd_una, sizeof(snd_una), (void*) &tp->snd_una); // First byte we want an ack for

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
int kprobe__tcp_recvmsg(struct pt_regs* ctx) {
    struct sock* sk;
    bpf_probe_read(&sk, sizeof(sk),(void*) ctx->di);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    bpf_map_update_elem(&who_recvmsg, &pid_tgid, &sk, BPF_ANY);
    return check_sock(sk);
}

SEC("kretprobe/tcp_recvmsg")
int kretprobe__tcp_recvmsg(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock** sk = bpf_map_lookup_elem(&who_recvmsg, &pid_tgid);
    bpf_map_delete_elem(&who_recvmsg, &pid_tgid);

    if (sk)
        return check_sock(*sk);
    return 0;
}

SEC("kprobe/tcp_sendmsg")
int kprobe__tcp_sendmsg(struct pt_regs* ctx) {
    struct sock* sk;
    bpf_probe_read(&sk, sizeof(sk), (void*) ctx->di);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    bpf_map_update_elem(&who_sendmsg, &pid_tgid, &sk, BPF_ANY);

    return check_sock(sk);
}

SEC("kretprobe/tcp_sendmsg")
int kretprobe__tcp_sendmsg(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock** sk = bpf_map_lookup_elem(&who_sendmsg, &pid_tgid);
    bpf_map_delete_elem(&who_sendmsg, &pid_tgid);

    if (sk)
        return check_sock(*sk);
    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
