#ifndef _STRUCTS_ERPC_H_
#define _STRUCTS_ERPC_H_

struct discard_request_t {
    u64 event_type;
    u64 timeout;
};

struct discard_inode_t {
    struct discard_request_t req;
    u64 inode;
    u32 mount_id;
    u32 is_leaf;
};

struct expire_inode_discarder_t {
    u64 inode;
    u32 mount_id;
};

struct discard_pid_t {
    struct discard_request_t req;
    u32 pid;
};

#endif
