#include <linux/kconfig.h>
#define KBUILD_MODNAME "ddsysprobe"
#include <linux/bpf.h>
#include <linux/ptrace.h>
#include <linux/tcp.h>
#include <net/inet_sock.h>

#include "bpf-common.h"
#include "tcp-queue-length-kern-user.h"

/*
 * The `tcp_queue_stats` map is used to share with the userland program system-probe
 * the statistics (max size of receive/send buffer)
 */
BPF_TABLE("percpu_hash", struct stats_key, struct stats_value, tcp_queue_stats, 1024);

/*
 * the `who_recvmsg` and `who_sendmsg` maps are used to remind the sock pointer
 * received as input parameter when we are in the kretprobe of tcp_recvmsg and tcp_sendmsg.
 */
BPF_HASH(who_recvmsg, u64, struct sock*, 100);
BPF_HASH(who_sendmsg, u64, struct sock*, 100);

// TODO: replace all `bpf_probe_read` by `bpf_probe_read_kernel` once we can assume that we have at least kernel 5.5
static inline int check_sock(struct sock* sk) {
    struct stats_value zero = {
        .read_buffer_max_usage = 0,
        .write_buffer_max_usage = 0
    };

    struct stats_key k;
    get_cgroup_name(k.cgroup_name, sizeof(k.cgroup_name));

    struct stats_value* v = tcp_queue_stats.lookup_or_init(&k, &zero);
    if (v == NULL)
        return 0;

    int rqueue_size, wqueue_size;
    bpf_probe_read(&rqueue_size, sizeof(rqueue_size), &sk->sk_rcvbuf);
    bpf_probe_read(&wqueue_size, sizeof(wqueue_size), &sk->sk_sndbuf);

    const struct tcp_sock* tp = tcp_sk(sk);
    u32 rcv_nxt, copied_seq, write_seq, snd_una;
    bpf_probe_read(&rcv_nxt, sizeof(rcv_nxt), &tp->rcv_nxt); // What we want to receive next
    bpf_probe_read(&copied_seq, sizeof(copied_seq), &tp->copied_seq); // Head of yet unread data
    bpf_probe_read(&write_seq, sizeof(write_seq), &tp->write_seq); // Tail(+1) of data held in tcp send buffer
    bpf_probe_read(&snd_una, sizeof(snd_una), &tp->snd_una); // First byte we want an ack for

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

int kprobe__tcp_recvmsg(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)ctx->di;

    u64 pid_tgid = bpf_get_current_pid_tgid();
    who_recvmsg.insert(&pid_tgid, &sk);

    return check_sock(sk);
}

int kretprobe__tcp_recvmsg(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock** sk = who_recvmsg.lookup(&pid_tgid);
    who_recvmsg.delete(&pid_tgid);

    if (sk)
        return check_sock(*sk);
    return 0;
}

int kprobe__tcp_sendmsg(struct pt_regs* ctx) {
    struct sock* sk = (struct sock*)ctx->di;

    u64 pid_tgid = bpf_get_current_pid_tgid();
    who_sendmsg.insert(&pid_tgid, &sk);

    return check_sock(sk);
}

int kretprobe__tcp_sendmsg(struct pt_regs* ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock** sk = who_sendmsg.lookup(&pid_tgid);
    who_sendmsg.delete(&pid_tgid);

    if (sk)
        return check_sock(*sk);
    return 0;
}
