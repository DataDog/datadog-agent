#ifndef _HELPERS_EVENTS_PREDICATES_H
#define _HELPERS_EVENTS_PREDICATES_H

#include "constants/enums.h"

int __attribute__((always_inline)) mnt_want_write_predicate(u64 type) {
    return type == EVENT_UTIME || type == EVENT_CHMOD || type == EVENT_CHOWN || type == EVENT_RENAME ||
        type == EVENT_RMDIR || type == EVENT_UNLINK || type == EVENT_SETXATTR || type == EVENT_REMOVEXATTR;
}

int __attribute__((always_inline)) mnt_want_write_file_predicate(u64 type) {
    return type == EVENT_SETXATTR || type == EVENT_REMOVEXATTR || type == EVENT_CHOWN;
}

int __attribute__((always_inline)) rmdir_predicate(u64 type) {
    return type == EVENT_RMDIR || type == EVENT_UNLINK;
}

int __attribute__((always_inline)) security_inode_predicate(u64 type) {
    return type == EVENT_UTIME || type == EVENT_CHMOD || type == EVENT_CHOWN;
}

int __attribute__((always_inline)) xattr_predicate(u64 type) {
    return type == EVENT_SETXATTR || type == EVENT_REMOVEXATTR;
}

int __attribute__((always_inline)) credentials_predicate(u64 type) {
    return type == EVENT_SETUID || type == EVENT_SETGID || type == EVENT_CAPSET;
}

int __attribute__((always_inline)) mountpoint_predicate(u64 type) {
    return type == EVENT_MOUNT || type == EVENT_UNSHARE_MNTNS;
}

#endif
