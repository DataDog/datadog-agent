#ifndef TCP_QUEUE_LENGTH_KERN_USER_H
#define TCP_QUEUE_LENGTH_KERN_USER_H

#include <linux/types.h>

struct stats_key {
  char cgroup_name[129];
};

struct stats_value {
  __u32 read_buffer_max_fill_rate;
  __u32 write_buffer_max_fill_rate;
};

#endif /* defined(TCP_QUEUE_LENGTH_KERN_USER_H) */
