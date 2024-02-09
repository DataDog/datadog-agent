#ifndef _STRUCTS_EVENTS_CONTEXT_H_
#define _STRUCTS_EVENTS_CONTEXT_H_

#include "constants/custom.h"
#include "dentry_resolver.h"

struct kevent_t {
    u64 cpu;
    u64 timestamp;
    u32 type;
    u32 flags;
};

struct syscall_t {
    s64 retval;
};

struct span_context_t {
   u64 span_id;
   u64 trace_id;
};

struct process_context_t {
    u32 pid;
    u32 tid;
    u32 netns;
    u32 is_kworker;
    u64 inode;
};

struct container_context_t {
    char container_id[CONTAINER_ID_LEN];
};

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

#endif
