#include <linux/kconfig.h>
#define KBUILD_MODNAME "foo"
#include <linux/ptrace.h>
#include <linux/bpf.h>
#include <net/inet_sock.h>
#include <linux/tcp.h>

#include "pkg/ebpf/c/tcp-queue-length-kern-user.h"

/*
 * The `queue` map is used to share with the userland program system-probe
 * the statistics (max/min size of receive/send buffer) of every socket
 */
BPF_HASH(queue, struct sock *, struct stats);

/*
 * the `who_recvmsg` and `who_sendmsg` maps are used to remind the sock pointer
 * received as input parameter when we are in the kretprobe of tcp_recvmsg and tcp_sendmsg.
 */
BPF_HASH(who_recvmsg, u64, struct sock *);
BPF_HASH(who_sendmsg, u64, struct sock *);

// TODO: replace all `bpf_probe_read` by `bpf_probe_read_kernel` once we can assume that we have at least kernel 5.5
static inline int check_sock(struct sock *sk) {
  struct stats zero = {
    .rqueue = {
      .min = 2^32-1,
      .max = 0
    },
    .wqueue = {
      .min = 2^32-1,
      .max = 0
    }
  };

  struct stats *s = queue.lookup_or_init(&sk, &zero);
  if (s == NULL) return 0;

  /*
   * We assume here that only one thread will read and/or write to a given socket.
   * Indeed, having several unsynchronized threads attempting to read and/or write to a socket
   * would corrupt the stream.
   * If that assumption was wrong, we would need to make the following piece of code thread safe.
   * In that case, per-cpu hash would be a better solution than mutex.
   */
  if (s->pid == 0) {
    s->pid = bpf_get_current_pid_tgid() >> 32;

    struct task_struct *cur_tsk = (struct task_struct *)bpf_get_current_task();
    struct css_set *css_set;
    if (!bpf_probe_read(&css_set, sizeof(css_set), &cur_tsk->cgroups)) {
      struct cgroup_subsys_state *css;
      // TODO: Do not arbitrarily pick the first subsystem
      if (!bpf_probe_read(&css, sizeof(css), &css_set->subsys[0])) {
        struct cgroup *cgrp;
        if (!bpf_probe_read(&cgrp, sizeof(cgrp), &css->cgroup)) {
          struct kernfs_node *kn;
          if (!bpf_probe_read(&kn, sizeof(kn), &cgrp->kn)) {
            const char *name;
            if (!bpf_probe_read(&name, sizeof(name), &kn->name)) {
              bpf_probe_read_str(&s->cgroup_name, sizeof(s->cgroup_name), name);
            }
          }
        }
      }
    }

    const struct inet_sock *ip = inet_sk(sk);
    bpf_probe_read(&s->conn.saddr, sizeof(s->conn.saddr), &ip->inet_saddr);
    bpf_probe_read(&s->conn.daddr, sizeof(s->conn.daddr), &ip->inet_daddr);
    bpf_probe_read(&s->conn.sport, sizeof(s->conn.sport), &ip->inet_sport);
    bpf_probe_read(&s->conn.dport, sizeof(s->conn.dport), &ip->inet_dport);
  }

  const struct tcp_sock *tp = tcp_sk(sk);

  u32 rcv_nxt, copied_seq, write_seq, snd_una;
  bpf_probe_read(&rcv_nxt,    sizeof(rcv_nxt),    &tp->rcv_nxt   );  // What we want to receive next
  bpf_probe_read(&copied_seq, sizeof(copied_seq), &tp->copied_seq);  // Head of yet unread data
  bpf_probe_read(&write_seq,  sizeof(write_seq),  &tp->write_seq );  // Tail(+1) of data held in tcp send buffer
  bpf_probe_read(&snd_una,    sizeof(snd_una),    &tp->snd_una   );  // First byte we want an ack for

  int rqueue = rcv_nxt - copied_seq;
  if (rqueue < 0) rqueue = 0;
  int wqueue = write_seq - snd_una;

  bpf_probe_read(&s->rqueue.size, sizeof(s->rqueue.size), &sk->sk_rcvbuf);
  bpf_probe_read(&s->wqueue.size, sizeof(s->wqueue.size), &sk->sk_sndbuf);

  if (rqueue > s->rqueue.max)
    s->rqueue.max = rqueue;
  if (rqueue < s->rqueue.min)
    s->rqueue.min = rqueue;
  if (wqueue > s->wqueue.max)
    s->wqueue.max = wqueue;
  if (wqueue < s->wqueue.min)
    s->wqueue.min = wqueue;

  return 0;
}

// TODO: do not call the same check_sock() function in kretprobe.
// The retrieval of the conn quadruplet can be done once and cached in the map
int kprobe__tcp_recvmsg(struct pt_regs *ctx)
{
  struct sock *sk = (struct sock *)ctx->di;

  u64 pid_tgid = bpf_get_current_pid_tgid();
  who_recvmsg.insert(&pid_tgid, &sk);

  return check_sock(sk);
}

int kretprobe__tcp_recvmsg(struct pt_regs *ctx)
{
  u64 pid_tgid = bpf_get_current_pid_tgid();
  struct sock **sk = who_recvmsg.lookup(&pid_tgid);
  who_recvmsg.delete(&pid_tgid);

  if (sk)
    return check_sock(*sk);
  return 0;
}

int kprobe__tcp_sendmsg(struct pt_regs *ctx)
{
  struct sock *sk = (struct sock *)ctx->di;

  u64 pid_tgid = bpf_get_current_pid_tgid();
  who_sendmsg.insert(&pid_tgid, &sk);

  return check_sock(sk);
}

int kretprobe__tcp_sendmsg(struct pt_regs *ctx)
{
  u64 pid_tgid = bpf_get_current_pid_tgid();
  struct sock **sk = who_sendmsg.lookup(&pid_tgid);
  who_sendmsg.delete(&pid_tgid);

  if (sk)
    return check_sock(*sk);
  return 0;
}
