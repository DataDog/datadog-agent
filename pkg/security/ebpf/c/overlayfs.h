#ifndef _OVERLAYFS_H_
#define _OVERLAYFS_H_

#include <linux/magic.h>

#include "syscalls.h"

static __attribute__((always_inline)) int is_overlayfs(struct dentry *dentry) {
    struct super_block *sb = get_dentry_sb(dentry);
    return get_sb_magic(sb) == OVERLAYFS_SUPER_MAGIC;
}

int __attribute__((always_inline)) get_ovl_lower_ino(struct dentry *dentry) {
    struct inode *d_inode;
    bpf_probe_read(&d_inode, sizeof(d_inode), &dentry->d_inode);

    // escape from the embedded vfs_inode to reach ovl_inode
    struct inode *lower;
    bpf_probe_read(&lower, sizeof(lower), (char *)d_inode + get_sizeof_inode() + 8);

    return get_inode_ino(lower);
}

int __attribute__((always_inline)) get_ovl_upper_ino(struct dentry *dentry) {
    struct inode *d_inode;
    bpf_probe_read(&d_inode, sizeof(d_inode), &dentry->d_inode);

    // escape from the embedded vfs_inode to reach ovl_inode
    struct dentry *upper;
    bpf_probe_read(&upper, sizeof(upper), (char *)d_inode + get_sizeof_inode());

    return get_dentry_ino(upper);
}

void __always_inline set_overlayfs_ino(struct dentry *dentry, u64 *ino, u32 *flags) {
    u64 lower_inode = get_ovl_lower_ino(dentry);
    u64 upper_inode = get_ovl_upper_ino(dentry);

#ifdef DEBUG
    bpf_printk("get_overlayfs_ino lower: %d upper: %d\n", lower_inode, upper_inode);
#endif

    if (upper_inode) {
        *flags |= UPPER_LAYER;
    } else if (lower_inode) {
        *flags |= LOWER_LAYER;
    }

    if (lower_inode) {
        *ino = lower_inode;
    } else if (upper_inode) {
        *ino = upper_inode;
    }
}

#endif