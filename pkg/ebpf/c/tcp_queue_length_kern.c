#include <linux/kconfig.h>
#define KBUILD_MODNAME "foo"
#include <linux/ptrace.h>
#include <linux/bpf.h>
#include <net/inet_sock.h>
#include <linux/tcp.h>

struct queue_length {
  u32 min;
  u32 max;
};

struct stats {
  u32 pid;
  struct queue_length rqueue;
  struct queue_length wqueue;
};

struct conn {
  u32 saddr;
  u32 daddr;
  u16 sport;
  u16 dport;
};


BPF_HASH(queue, struct conn, struct stats);

BPF_HASH(who_recvmsg, u64, struct sock *);
BPF_HASH(who_sendmsg, u64, struct sock *);

static inline int check_sock(struct sock *sk) {
  const struct inet_sock *ip = inet_sk(sk);
  struct conn c;
  bpf_probe_read(&c.saddr, sizeof(c.saddr), &ip->inet_saddr);
  bpf_probe_read(&c.daddr, sizeof(c.daddr), &ip->inet_daddr);
  bpf_probe_read(&c.sport, sizeof(c.sport), &ip->inet_sport);
  bpf_probe_read(&c.dport, sizeof(c.dport), &ip->inet_dport);


  const struct tcp_sock *tp = tcp_sk(sk);

  u32 rcv_nxt, copied_seq, write_seq, snd_una;
  bpf_probe_read(&rcv_nxt,    sizeof(rcv_nxt),    &tp->rcv_nxt   );  // What we want to receive next
  bpf_probe_read(&copied_seq, sizeof(copied_seq), &tp->copied_seq);  // Head of yet unread data
  bpf_probe_read(&write_seq,  sizeof(write_seq),  &tp->write_seq );  // Tail(+1) of data held in tcp send buffer
  bpf_probe_read(&snd_una,    sizeof(snd_una),    &tp->snd_una   );  // First byte we want an ack for

  int rqueue = rcv_nxt - copied_seq;
  if (rqueue < 0) rqueue = 0;
  int wqueue = write_seq - snd_una;

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

  struct stats *s = queue.lookup_or_init(&c, &zero);

  if (s) {
    s->pid = bpf_get_current_pid_tgid() >> 32;
    if (rqueue > s->rqueue.max)
      s->rqueue.max = rqueue;
    if (rqueue < s->rqueue.min)
      s->rqueue.min = rqueue;
    if (wqueue > s->wqueue.max)
      s->wqueue.max = wqueue;
    if (wqueue < s->wqueue.min)
      s->wqueue.min = wqueue;
  }

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
