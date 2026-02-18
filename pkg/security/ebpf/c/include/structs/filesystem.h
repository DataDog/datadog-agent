#ifndef _STRUCTS_FILESYSTEM_H_
#define _STRUCTS_FILESYSTEM_H_

#include "constants/custom.h"
#include "events_context.h"

struct mount_released_event_t {
    struct kevent_t event;
    u32 mount_id;
    u32 __pad;
    u64 mount_id_unique;
};

struct mount_fields_t {
    struct path_key_t root_key;
    struct path_key_t mountpoint_key;
    dev_t device;
    u32 bind_src_mount_id;
    u64 mount_id_unique;
    u64 parent_mount_id_unique;
    u64 bind_src_mount_id_unique;
    char fstype[FSTYPE_LEN];
    u16   visible;   // Is mount visible in the VFS?
    u16   detached;  // A detached mount is always not visible, but an invisible mount isn't always detached
    u32   ns_inum;
};

#endif
