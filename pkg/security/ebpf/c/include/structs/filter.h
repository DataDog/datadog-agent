#ifndef _STRUCTS_FILTER_H_
#define _STRUCTS_FILTER_H_

#include "constants/custom.h"
#include "constants/enums.h"
#include "dentry_resolver.h"

struct policy_t {
    char mode;
    char flags;
};

// Approvers

struct approver_stats_t {
    u64 event_approved_by_basename;
    u64 event_approved_by_flag;
};

struct basename_t {
    char value[BASENAME_FILTER_SIZE];
};

struct basename_filter_t {
    u64 event_mask;
};

// Discarders

struct discarder_stats_t {
    u64 discarders_added;
    u64 event_discarded;
};

struct discarder_params_t {
    u64 event_mask;
    u64 timestamps[EVENT_LAST_DISCARDER+1-EVENT_FIRST_DISCARDER];
    u64 expire_at;
    u32 is_retained;
    u32 revision;
};

struct inode_discarder_params_t {
    struct discarder_params_t params;
    u32 mount_revision;
};

struct pid_discarder_params_t {
    struct discarder_params_t params;
};

struct pid_discarder_t {
    u32 tgid;
};

struct inode_discarder_t {
    struct path_key_t path_key;
    u32 is_leaf;
    u32 padding;
};

struct is_discarded_by_inode_t {
    u64 discarder_type;
    struct inode_discarder_t discarder;
    u64 now;
};


#endif
