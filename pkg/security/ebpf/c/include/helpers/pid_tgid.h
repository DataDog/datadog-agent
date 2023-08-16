#ifndef _HELPERS_PIDTGID_H_
#define _HELPERS_PIDTGID_H_

#include "constants/macros.h"

u64 __attribute__((always_inline)) __attribute__((pure)) get_ns_current_pid_tgid() {
    u64 dev, ino;
    LOAD_CONSTANT("pid_namespace_device", dev);
    LOAD_CONSTANT("pid_namespace_inode", ino);

    struct bpf_pidns_info info;
    if (bpf_get_ns_current_pid_tgid(dev, ino, &info, sizeof(info)) != 0) {
        return -1;
    }

    return (u64)info.tgid << 32 | (u64)info.pid;
}

#endif // _HELPERS_PIDTGID_H_
