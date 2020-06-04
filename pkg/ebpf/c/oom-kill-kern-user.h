#ifndef OOM_KILL_KERN_USER_H
#define OOM_KILL_KERN_USER_H

#include <linux/types.h>

#ifndef TASK_COMM_LEN
#define TASK_COMM_LEN 16
#endif

struct oom_stats {
  char cgroup_name[64];
  __u32 pid;
  __u32 tpid;
  char fcomm[TASK_COMM_LEN];
  char tcomm[TASK_COMM_LEN];
  __u64 pages;
  __u32 memcg_oom;
};

#endif /* defined(OOM_KILL_KERN_USER_H) */
