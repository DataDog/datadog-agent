#ifndef _STRUCTS_EVENTS_CONTEXT_H_
#define _STRUCTS_EVENTS_CONTEXT_H_

#include "constants/custom.h"
#include "dentry_resolver.h"

struct kevent_t {
    u64 timestamp;
    u32 type;
    u32 flags;
};

struct syscall_t {
    s64 retval;
};

struct syscall_context_t {
    u32 id;
    u32 padding;
};

struct span_context_t {
    u64 span_id;
    u64 trace_id[2];
};

struct process_context_t {
    u32 pid;
    u32 tid;
    u32 netns;
    u32 is_kworker;
    u64 inode;
};

typedef char container_id_t[CONTAINER_ID_LEN];

typedef char cgroup_prefix_t[256];

struct ktimeval {
    long tv_sec;
    long tv_nsec;
};

struct file_metadata_t {
    u32 uid;
    u32 gid;
    u32 nlink;
    u16 mode;
    char padding[2];

    struct ktimeval ctime;
    struct ktimeval mtime;
};

struct file_t {
    struct path_key_t path_key;
    u32 dev;
    u32 flags;
    struct file_metadata_t metadata;
};

struct cgroup_context_t {
    u64 cgroup_flags;
    struct path_key_t cgroup_file;
};

struct container_context_t {
    container_id_t container_id;
    struct cgroup_context_t cgroup_context;
};

#endif
