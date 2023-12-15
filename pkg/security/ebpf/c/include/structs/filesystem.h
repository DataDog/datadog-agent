#ifndef _STRUCTS_FILESYSTEM_H_
#define _STRUCTS_FILESYSTEM_H_

#include "constants/custom.h"
#include "events_context.h"

struct mount_released_event_t {
    struct kevent_t event;
    u32 mount_id;
};

struct mount_ref_t {
    u32 umounted;
    s32 counter;
};

 struct mount_fields_t {
    struct path_key_t root_key;
    struct path_key_t mountpoint_key;
    dev_t device;
    u32 bind_src_mount_id;
    char fstype[FSTYPE_LEN];
};

#endif
