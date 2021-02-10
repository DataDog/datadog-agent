#include <linux/compiler.h>

#include <linux/kconfig.h>
#include <linux/ptrace.h>
#include <linux/types.h>
#include <linux/version.h>

#include "defs.h"
#include "buffer_selector.h"
#include "process.h"
#include "container.h"
#include "dentry.h"
#include "overlayfs.h"
#include "exec.h"
#include "setattr.h"
#include "mnt.h"
#include "filename.h"
#include "chmod.h"
#include "chown.h"
#include "mkdir.h"
#include "rmdir.h"
#include "unlink.h"
#include "rename.h"
#include "cgroup.h"
#include "open.h"
#include "utimes.h"
#include "mount.h"
#include "umount.h"
#include "link.h"
#include "raw_syscalls.h"
#include "procfs.h"
#include "setxattr.h"

struct invalidate_dentry_event_t {
    struct kevent_t event;
    u64 inode;
    u32 mount_id;
    u32 revision;
};

void __attribute__((always_inline)) invalidate_inode(struct pt_regs *ctx, u32 mount_id, u64 inode, int send_invalidate_event) {
    if (!inode || !mount_id)
        return;

    if (!is_flushing_discarders()) {
        remove_inode_discarder(mount_id, inode);
    }

    if (send_invalidate_event) {
        // invalidate dentry
        struct invalidate_dentry_event_t event = {
            .inode = inode,
            .mount_id = mount_id,
            .revision = bump_discarder_revision(mount_id),
        };

        send_event(ctx, EVENT_INVALIDATE_DENTRY, event);
    }
}

__u32 _version SEC("version") = 0xFFFFFFFE;

char LICENSE[] SEC("license") = "GPL";
