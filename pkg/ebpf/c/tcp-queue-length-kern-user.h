#ifndef TCP_QUEUE_LENGTH_KERN_USER_H
#define TCP_QUEUE_LENGTH_KERN_USER_H

#include <linux/types.h>

struct queue_length {
  int size;
  __u32 min;
  __u32 max;
};

struct conn {
  __u32 saddr;
  __u32 daddr;
  __u16 sport;
  __u16 dport;
};

struct stats {
  __u32 pid;
  char cgroup_name[64];
  struct conn conn;
  struct queue_length rqueue;
  struct queue_length wqueue;
};

#endif /* defined(TCP_QUEUE_LENGTH_KERN_USER_H) */
