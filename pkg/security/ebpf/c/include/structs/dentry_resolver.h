#ifndef _STRUCTS_DENTRY_RESOLVER_H_
#define _STRUCTS_DENTRY_RESOLVER_H_

#include "constants/custom.h"

struct path_key_t {
    u64 ino;
    u32 mount_id;
    u32 path_id;
};

struct path_leaf_t {
  struct path_key_t parent;
  char name[DR_MAX_SEGMENT_LENGTH + 1];
  u16 len;
};

struct dr_erpc_state_t {
    char *userspace_buffer;
    struct path_key_t key;
    int ret;
    int iteration;
    u32 buffer_size;
    u32 challenge;
    u16 cursor;
};

struct dr_erpc_stats_t {
    u64 count;
};

struct dentry_resolver_input_t {
    struct path_key_t key;
    struct dentry *dentry;
    u64 discarder_type;
    s64 sysretval;
    int callback;
    int ret;
    int iteration;
    u32 flags;
};

#endif
