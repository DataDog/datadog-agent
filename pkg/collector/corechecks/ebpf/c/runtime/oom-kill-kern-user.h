#ifndef OOM_KILL_KERN_USER_H
#define OOM_KILL_KERN_USER_H

#include <linux/types.h>

#ifndef TASK_COMM_LEN
#define TASK_COMM_LEN 16
#endif

struct oom_stats {
  char cgroup_name[129];
  // Pid of triggering process
  __u32 pid;
  // Pid of killed process
  __u32 tpid;
  // Name of triggering process
  char fcomm[TASK_COMM_LEN];
  // Name of killed process
  char tcomm[TASK_COMM_LEN];
  // Total number of pages
  __u64 pages;
  // Tracks if the OOM kill was triggered by a cgroup
  __u32 memcg_oom;
};

#endif /* defined(OOM_KILL_KERN_USER_H) */
