#ifndef _STRUCTS_DENTRY_RESOLVER_H_
#define _STRUCTS_DENTRY_RESOLVER_H_

#include "constants/custom.h"
#include "ring_buffer.h"

struct dentry_key_t {
    u64 ino;
    u32 mount_id;
    u32 path_id;
};

struct dentry_leaf_t {
    struct dentry_key_t parent;
};

// struct path_key_t {
//     u64 ino;
//     u32 mount_id;
//     u32 path_id;
// };

// struct path_leaf_t {
//   struct path_key_t parent;
//   char name[DR_MAX_SEGMENT_LENGTH + 1];
//   u16 len;
// };

enum dr_ring_buffer_reader_state_t {
    READ_FRONTWATERMARK,
    READ_PATHSEGMENT,
    READ_BACKWATERMARK,
};

struct dr_erpc_state_t {
    char *userspace_buffer;
    struct dentry_key_t key;
    struct ring_buffer_ref_t path_ref;
    u32 path_end_cursor;
    int ret;
    int iteration;
    u32 buffer_size;
    u32 challenge;
    u32 cursor;
    enum dr_ring_buffer_reader_state_t path_reader_state;
};

struct dr_erpc_stats_t {
    u64 count;
};

struct dentry_resolver_input_t {
    struct dentry_key_t key;
    struct dentry *dentry;
    u64 discarder_type;
    s64 sysretval;
    int callback;
    int ret;
    int iteration;
    u32 flags;
};

#endif
