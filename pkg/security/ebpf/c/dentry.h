#ifndef _DENTRY_H_
#define _DENTRY_H_

#include <linux/dcache.h>
#include <linux/types.h>
#include <linux/mount.h>
#include <linux/fs.h>

#include "defs.h"
#include "filters.h"

#define DENTRY_MAX_DEPTH 16

#define MNT_OFFSETOF_MNT 32 // offsetof(struct mount, mnt)
#if USE_SYSCALL_WRAPPER == 1
# define MNT_OFFSETOF_MNTID 284 // offsetof(struct mount, mnt_id)
#else
# define MNT_OFFSETOF_MNTID 268 // offsetof(struct mount, mnt_id)
#endif

struct path_key_t {
    unsigned long ino;
    int mount_id;
    u32 padding;
};

struct path_leaf_t {
  struct path_key_t parent;
  // TODO: reduce the amount of allocated structs during the resolution so that we can take this buffer to its max
  // theoretical value (256), without reaching the eBPF stack max size.
  char name[128];
};

struct bpf_map_def SEC("maps/pathnames") pathnames = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct path_key_t),
    .value_size = sizeof(struct path_leaf_t),
    .max_entries = 32000,
    .pinning = 0,
    .namespace = "",
};

unsigned long __attribute__((always_inline)) get_inode_ino(struct inode *inode) {
    unsigned long ino;
    bpf_probe_read(&ino, sizeof(inode), &inode->i_ino);
    return ino;
}

void __attribute__((always_inline)) write_inode_ino(struct inode *inode, unsigned long *ino) {
    bpf_probe_read(ino, sizeof(inode), &inode->i_ino);
}

dev_t __attribute__((always_inline)) get_inode_dev(struct inode *inode) {
    dev_t dev;
    struct super_block *sb;
    bpf_probe_read(&sb, sizeof(sb), &inode->i_sb);
    bpf_probe_read(&dev, sizeof(dev), &sb->s_dev);
    return dev;
}

dev_t __attribute__((always_inline)) get_dentry_dev(struct dentry *dentry) {
    dev_t dev;
    struct super_block *sb;
    bpf_probe_read(&sb, sizeof(sb), &dentry->d_sb);
    bpf_probe_read(&dev, sizeof(dev), &sb->s_dev);
    return dev;
}

int __attribute__((always_inline)) get_vfsmount_mount_id(struct vfsmount *mnt) {
    int mount_id;
    // bpf_probe_read(&mount_id, sizeof(mount_id), (void *)mnt + offsetof(struct mount, mnt_id) - offsetof(struct mount, mnt));
    bpf_probe_read(&mount_id, sizeof(mount_id), (void *)mnt + MNT_OFFSETOF_MNTID - MNT_OFFSETOF_MNT);
    return mount_id;
}

int __attribute__((always_inline)) get_path_mount_id(struct path *path) {
    struct vfsmount *mnt;
    bpf_probe_read(&mnt, sizeof(mnt), &path->mnt);
    return get_vfsmount_mount_id(mnt);
}

int __attribute__((always_inline)) get_mount_mount_id(void *mnt) {
    int mount_id;

    // bpf_probe_read(&mount_id, sizeof(mount_id), (void *)mnt + offsetof(struct mount, mnt_id));
    bpf_probe_read(&mount_id, sizeof(mount_id), (void *)mnt + MNT_OFFSETOF_MNTID);
    return mount_id;
}

int __attribute__((always_inline)) get_mount_peer_group_id(void *mnt) {
    int mount_id;

    // bpf_probe_read(&mount_id, sizeof(mount_id), (void *)mnt + offsetof(struct mount, mnt_group_id));
    bpf_probe_read(&mount_id, sizeof(mount_id), (void *)mnt + MNT_OFFSETOF_MNTID + 4);
    return mount_id;
}

struct vfsmount * __attribute__((always_inline)) get_mount_vfsmount(void *mnt) {
    return (struct vfsmount *)((void *)mnt + 32);
}

struct dentry * __attribute__((always_inline)) get_vfsmount_dentry(struct vfsmount *mnt) {
    struct dentry *dentry;
    bpf_probe_read(&dentry, sizeof(dentry), &mnt->mnt_root);
    return dentry;
}

struct super_block * __attribute__((always_inline)) get_vfsmount_sb(struct vfsmount *mnt) {
    struct super_block *sb;
    bpf_probe_read(&sb, sizeof(sb), &mnt->mnt_sb);
    return sb;
}

dev_t __attribute__((always_inline)) get_sb_dev(struct super_block *sb) {
    dev_t dev;
    bpf_probe_read(&dev, sizeof(dev), &sb->s_dev);
    return dev;
}

struct dentry * __attribute__((always_inline)) get_mountpoint_dentry(void *mntpoint) {
    struct dentry *dentry;

    // bpf_probe_read(&dentry, sizeof(dentry), (void *)mntpoint + offsetof(struct mountpoint, m_dentry));
    bpf_probe_read(&dentry, sizeof(dentry), (void *)mntpoint + 16);
    return dentry;
}

dev_t __attribute__((always_inline)) get_vfsmount_dev(struct vfsmount *mnt) {
    return get_sb_dev(get_vfsmount_sb(mnt));
}

dev_t __attribute__((always_inline)) get_mount_dev(void *mnt) {
    return get_vfsmount_dev(get_mount_vfsmount(mnt));
}

int __attribute__((always_inline)) get_overlay_numlower(struct dentry *dentry) {
    int numlower;
    void *fsdata;
    bpf_probe_read(&fsdata, sizeof(void *), &dentry->d_fsdata);

    // bpf_probe_read(&numlower, sizeof(int), fsdata + offsetof(struct ovl_entry, numlower));
    // TODO: make it a constant and change its value based on the current kernel version. 16 is only good for kernels 4.13+
    bpf_probe_read(&numlower, sizeof(int), fsdata + 16);
    return numlower;
}

unsigned long __attribute__((always_inline)) get_dentry_ino(struct dentry *dentry) {
    struct inode *d_inode;
    bpf_probe_read(&d_inode, sizeof(d_inode), &dentry->d_inode);
    return get_inode_ino(d_inode);
}

void __attribute__((always_inline)) write_dentry_inode(struct dentry *dentry, struct inode **d_inode) {
    bpf_probe_read(d_inode, sizeof(d_inode), &dentry->d_inode);
}

struct dentry* __attribute__((always_inline)) get_file_dentry(struct file *file) {
    struct dentry *f_dentry;
    bpf_probe_read(&f_dentry, sizeof(f_dentry), &file->f_path.dentry);
    return f_dentry;
}

struct dentry* __attribute__((always_inline)) get_path_dentry(struct path *path) {
    struct dentry *dentry;
    bpf_probe_read(&dentry, sizeof(dentry), &path->dentry);
    return dentry;
}

unsigned long  __attribute__((always_inline)) get_path_ino(struct path *path) {
    struct dentry *dentry;
    bpf_probe_read(&dentry, sizeof(dentry), &path->dentry);

    if (dentry) {
        return get_dentry_ino(dentry);
    }
    return 0;
}

void __attribute__((always_inline)) get_dentry_name(struct dentry *dentry, void *buffer, size_t n) {
    struct qstr qstr;
    bpf_probe_read(&qstr, sizeof(qstr), &dentry->d_name);
    bpf_probe_read_str(buffer, n, (void *)qstr.name);
}

#define get_key(dentry, path) (struct path_key_t) { .ino = get_dentry_ino(dentry), .mount_id = get_path_mount_id(path) }

static __attribute__((always_inline)) int resolve_dentry(struct dentry *dentry, struct path_key_t key, struct bpf_map_def *discarders_table) {
    struct path_leaf_t map_value = {};
    struct path_key_t next_key = key;
    struct qstr qstr;
    struct dentry *d_parent;
    struct inode *d_inode = NULL;

#pragma unroll
    for (int i = 0; i < DENTRY_MAX_DEPTH; i++)
    {
        d_parent = NULL;
        bpf_probe_read(&d_parent, sizeof(d_parent), &dentry->d_parent);

        key = next_key;
        if (dentry != d_parent) {
            write_dentry_inode(d_parent, &d_inode);
            write_inode_ino(d_inode, &next_key.ino);
        }

        // discard filename and its parent only in order to limit the number of lookup
        if (discarders_table && i < 2) {
            struct filter_t *filter = bpf_map_lookup_elem(discarders_table, &key);
            if (filter) {
                return -1;
            }
        }

        bpf_probe_read(&qstr, sizeof(qstr), &dentry->d_name);
        bpf_probe_read_str(&map_value.name, sizeof(map_value.name), (void *)qstr.name);

        if (map_value.name[0] == '/' || map_value.name[0] == 0) {
            next_key.ino = 0;
            next_key.mount_id = 0;
        }

        map_value.parent = next_key;

        bpf_map_update_elem(&pathnames, &key, &map_value, BPF_ANY);

        dentry = d_parent;
        if (next_key.ino == 0)
            return i + 1;
    }

    // If the last next_id isn't null, this means that there are still other parents to fetch.
    // TODO: use BPF_PROG_ARRAY to recursively fetch 32 more times. For now, add a fake parent to notify
    // that we couldn't fetch everything.

    map_value.name[0] = 0;
    map_value.parent.mount_id = 0;
    map_value.parent.ino = 0;
    bpf_map_update_elem(&pathnames, &next_key, &map_value, BPF_ANY);

    return DENTRY_MAX_DEPTH;
}

#endif
