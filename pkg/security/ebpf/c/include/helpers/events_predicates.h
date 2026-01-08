#ifndef _HELPERS_EVENTS_PREDICATES_H
#define _HELPERS_EVENTS_PREDICATES_H

#include "constants/enums.h"

static int __attribute__((always_inline)) mnt_want_write_predicate(u64 type) {
    return type == EVENT_UTIME || type == EVENT_CHMOD || type == EVENT_CHOWN || type == EVENT_RENAME ||
           type == EVENT_RMDIR || type == EVENT_UNLINK || type == EVENT_SETXATTR || type == EVENT_REMOVEXATTR;
}

static int __attribute__((always_inline)) mnt_want_write_file_predicate(u64 type) {
    return type == EVENT_SETXATTR || type == EVENT_REMOVEXATTR || type == EVENT_CHOWN;
}

static int __attribute__((always_inline)) rmdir_predicate(u64 type) {
    return type == EVENT_RMDIR || type == EVENT_UNLINK;
}

static int __attribute__((always_inline)) security_inode_predicate(u64 type) {
    return type == EVENT_UTIME || type == EVENT_CHMOD || type == EVENT_CHOWN;
}

static int __attribute__((always_inline)) xattr_predicate(u64 type) {
    return type == EVENT_SETXATTR || type == EVENT_REMOVEXATTR;
}

static int __attribute__((always_inline)) credentials_predicate(u64 type) {
    return type == EVENT_SETUID || type == EVENT_SETGID || type == EVENT_CAPSET;
}

static int __attribute__((always_inline)) mountpoint_predicate(u64 type) {
    return type == EVENT_MOUNT || type == EVENT_UNSHARE_MNTNS || type == EVENT_OPEN_TREE || type == EVENT_MOVE_MOUNT;
}

static int __attribute__((always_inline)) mount_or_open_tree(u64 type) {
    return type == EVENT_MOUNT || type == EVENT_OPEN_TREE;
}

static int __attribute__((always_inline)) unshare_or_move_mount(u64 type) {
    return type == EVENT_UNSHARE_MNTNS || type == EVENT_MOVE_MOUNT;
}

static int __attribute__((always_inline)) mount_or_move_mount(u64 type) {
    return type == EVENT_MOUNT || type == EVENT_MOVE_MOUNT;
}

static int __attribute__((always_inline)) unshare_or_open_tree_or_move_mount(u64 type) {
    return type == EVENT_UNSHARE_MNTNS || type == EVENT_OPEN_TREE || type == EVENT_MOVE_MOUNT;
}

#endif
