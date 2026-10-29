#ifndef OOM_KILL_KERN_USER_H
#define OOM_KILL_KERN_USER_H

#include "ktypes.h"

#ifndef TASK_COMM_LEN
#define TASK_COMM_LEN 16
#endif

typedef struct {} oom_stats_v2;

// struct cgroup_memory_stats holds the memory usage of the cgroup and the value of some of its stat
// items, in pages. Untracked stat items are -1.
struct cgroup_memory_stats {
    __u64 usage;
    __u64 active_anon;
    __u64 inactive_anon;
    __u64 active_file;
    __u64 inactive_file;
    __u64 unevictable;
    __u64 kernel;
    // CGROUP_MEM_STATS_* flags
    __u64 flags;
};

// The stat items could not be read without overlapping a memcg stats flush step, and can be off by
// the updates that step moved.
#define CGROUP_MEM_STATS_UNSTABLE (1 << 0)

struct stats_key {
    __u64 ts;
    __u64 pid_tgid;
    __u32 cpu;
};

struct oom_stats {
    // version marker to ensure compatibility
    // between a new oom kill map vs the pinned map.
    // Bump it whenever this struct changes, see
    // pkg/collector/corechecks/ebpf/probe/oomkill/oom_stats_layout_linux.go
    oom_stats_v2 oom_stats_version;
    // cgroup memory statistics at the point of OOM
    struct cgroup_memory_stats mem;
    // Pid of killed process
    __u32 victim_pid;
    // Pid of triggering process
    __u32 trigger_pid;
    // Name of killed process
    char victim_comm[TASK_COMM_LEN];
    // Name of triggering process
    char trigger_comm[TASK_COMM_LEN];
    // OOM score of killed process
    __s64 score;
    // Total number of pages
    __u64 pages;
    // Tracks if the OOM kill was triggered by a cgroup
    __u32 memcg_oom;
    // OOM score adjustment of killed process
    __s16 score_adj;

    // 2 bytes of padding to force cgroup_name to be 8 byte aligned for bpf_memset
    __s16 pad;

    char cgroup_name[129];
};

#endif /* defined(OOM_KILL_KERN_USER_H) */
