#ifndef __SOCKET_CONTENTION_KERN_USER_H
#define __SOCKET_CONTENTION_KERN_USER_H

#include "ktypes.h"

#define SOCKET_CONTENTION_OBJECT_KIND_UNKNOWN 0
#define SOCKET_CONTENTION_OBJECT_KIND_SOCKET  1

#define SOCKET_CONTENTION_LOCK_SUBTYPE_UNKNOWN            0
#define SOCKET_CONTENTION_LOCK_SUBTYPE_SK_LOCK            1
#define SOCKET_CONTENTION_LOCK_SUBTYPE_SK_WAIT_QUEUE      2
#define SOCKET_CONTENTION_LOCK_SUBTYPE_CALLBACK_LOCK      3
#define SOCKET_CONTENTION_LOCK_SUBTYPE_ERROR_QUEUE_LOCK   4
#define SOCKET_CONTENTION_LOCK_SUBTYPE_RECEIVE_QUEUE_LOCK 5
#define SOCKET_CONTENTION_LOCK_SUBTYPE_WRITE_QUEUE_LOCK   6

struct socket_lock_identity {
    __u64 sock_ptr;
    __u64 socket_cookie;
    __u64 cgroup_id;
    __u16 family;
    __u16 protocol;
    __u16 socket_type;
    __u8 lock_subtype;
    __u8 reserved;
};

struct socket_contention_key {
    __u64 cgroup_id;
    __u32 flags;
    __u16 family;
    __u16 protocol;
    __u16 socket_type;
    __u8 object_kind;
    __u8 lock_subtype;
};

struct socket_contention_stats {
    __u64 total_time_ns;
    __u64 min_time_ns;
    __u64 max_time_ns;
    __u64 count;
};

#endif
