#include <linux/compiler.h>

#include <linux/kconfig.h>
#include <linux/ptrace.h>
#include <linux/types.h>
#include <linux/version.h>
#include <linux/bpf.h>
#include <linux/filter.h>
#include <uapi/asm-generic/mman-common.h>

#include "defs.h"
#include "buffer_selector.h"
#include "filters.h"
#include "approvers.h"
#include "discarders.h"
#include "dentry.h"
#include "dentry_resolver.h"
#include "exec.h"
#include "process.h"
#include "container.h"
#include "commit_creds.h"
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
#include "procfs.h"
#include "setxattr.h"
#include "erpc.h"
#include "ioctl.h"
#include "selinux.h"
#include "bpf.h"
#include "ptrace.h"
#include "mmap.h"
#include "mprotect.h"
#include "raw_syscalls.h"

struct invalidate_dentry_event_t {
    struct kevent_t event;
    u64 inode;
    u32 mount_id;
    u32 padding;
};

void __attribute__((always_inline)) invalidate_inode(struct pt_regs *ctx, u32 mount_id, u64 inode, int send_invalidate_event) {
    if (!inode || !mount_id)
        return;

    if (!is_flushing_discarders()) {
        // remove both regular and parent discarders
        remove_inode_discarders(mount_id, inode);
    }

    if (send_invalidate_event) {
        // invalidate dentry
        struct invalidate_dentry_event_t event = {
            .inode = inode,
            .mount_id = mount_id,
        };

        send_event(ctx, EVENT_INVALIDATE_DENTRY, event);
    }
}

__u32 _version SEC("version") = 0xFFFFFFFE;

char LICENSE[] SEC("license") = "GPL";
