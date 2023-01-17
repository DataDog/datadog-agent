#ifndef _DENTRY_H_
#define _DENTRY_H_

#include <linux/dcache.h>
#include <linux/types.h>
#include <linux/mount.h>
#include <linux/fs.h>
#include <linux/magic.h>

#include "defs.h"
#include "filters.h"

#define MNT_OFFSETOF_MNT 32 // offsetof(struct mount, mnt)

unsigned long __attribute__((always_inline)) get_inode_ino(struct inode *inode) {
    unsigned long ino;
    bpf_probe_read(&ino, sizeof(inode), &inode->i_ino);
    return ino;
}

void __attribute__((always_inline)) write_inode_ino(struct inode *inode, u64 *ino) {
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

u32 __attribute__((always_inline)) get_mount_offset_of_mount_id(void) {
    u64 offset;
    LOAD_CONSTANT("mount_id_offset", offset);
    return offset ? offset : 284; // offsetof(struct mount, mnt_id)
}

int __attribute__((always_inline)) get_vfsmount_mount_id(struct vfsmount *mnt) {
    int mount_id;
    // bpf_probe_read(&mount_id, sizeof(mount_id), (char *)mnt + offsetof(struct mount, mnt_id) - offsetof(struct mount, mnt));
    bpf_probe_read(&mount_id, sizeof(mount_id), (char *)mnt + get_mount_offset_of_mount_id() - MNT_OFFSETOF_MNT);
    return mount_id;
}

int __attribute__((always_inline)) get_path_mount_id(struct path *path) {
    struct vfsmount *mnt;
    bpf_probe_read(&mnt, sizeof(mnt), &path->mnt);
    return get_vfsmount_mount_id(mnt);
}

int __attribute__((always_inline)) get_file_mount_id(struct file *file) {
    struct vfsmount *mnt;
    bpf_probe_read(&mnt, sizeof(mnt), &file->f_path.mnt);
    return get_vfsmount_mount_id(mnt);
}

int __attribute__((always_inline)) get_vfsmount_mount_flags(struct vfsmount *mnt) {
    int mount_flags;
    bpf_probe_read(&mount_flags, sizeof(mount_flags), &mnt->mnt_flags);
    return mount_flags;
}

int __attribute__((always_inline)) get_path_mount_flags(struct path *path) {
    struct vfsmount *mnt;
    bpf_probe_read(&mnt, sizeof(mnt), &path->mnt);
    return get_vfsmount_mount_flags(mnt);
}

int __attribute__((always_inline)) get_mount_mount_id(void *mnt) {
    int mount_id;

    // bpf_probe_read(&mount_id, sizeof(mount_id), (char *)mnt + offsetof(struct mount, mnt_id));
    bpf_probe_read(&mount_id, sizeof(mount_id), (char *)mnt + get_mount_offset_of_mount_id());
    return mount_id;
}

int __attribute__((always_inline)) get_mount_peer_group_id(void *mnt) {
    int mount_id;

    // bpf_probe_read(&mount_id, sizeof(mount_id), (char *)mnt + offsetof(struct mount, mnt_group_id));
    bpf_probe_read(&mount_id, sizeof(mount_id), (char *)mnt + get_mount_offset_of_mount_id() + 4);
    return mount_id;
}

struct dentry * __attribute__((always_inline)) get_mount_mountpoint_dentry(struct mount *mnt) {
    struct dentry *dentry;
    bpf_probe_read(&dentry, sizeof(dentry), (char *)mnt + 24);
    return dentry;
}

struct vfsmount * __attribute__((always_inline)) get_mount_vfsmount(void *mnt) {
    return (struct vfsmount *)(mnt + 32);
}

struct dentry * __attribute__((always_inline)) get_vfsmount_dentry(struct vfsmount *mnt) {
    struct dentry *dentry;
    bpf_probe_read(&dentry, sizeof(dentry), &mnt->mnt_root);
    return dentry;
}

struct super_block *__attribute__((always_inline)) get_dentry_sb(struct dentry *dentry) {
    u64 offset;
    LOAD_CONSTANT("dentry_sb_offset", offset);

    struct super_block *sb;
    bpf_probe_read(&sb, sizeof(sb), (char *)dentry + offset);
    return sb;
}

struct file_system_type * __attribute__((always_inline)) get_super_block_fs(struct super_block *sb) {
    struct file_system_type *fs;
    bpf_probe_read(&fs, sizeof(fs), &sb->s_type);
    return fs;
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

    // bpf_probe_read(&dentry, sizeof(dentry), (char *)mntpoint + offsetof(struct mountpoint, m_dentry));
    bpf_probe_read(&dentry, sizeof(dentry), (char *)mntpoint + 16);
    return dentry;
}

dev_t __attribute__((always_inline)) get_vfsmount_dev(struct vfsmount *mnt) {
    return get_sb_dev(get_vfsmount_sb(mnt));
}

dev_t __attribute__((always_inline)) get_mount_dev(void *mnt) {
    return get_vfsmount_dev(get_mount_vfsmount(mnt));
}

struct inode* __attribute__((always_inline)) get_dentry_inode(struct dentry *dentry) {
    struct inode *d_inode;
    bpf_probe_read(&d_inode, sizeof(d_inode), &dentry->d_inode);
    return d_inode;
}

unsigned long __attribute__((always_inline)) get_dentry_ino(struct dentry *dentry) {
    return get_inode_ino(get_dentry_inode(dentry));
}

void __attribute__((always_inline)) fill_file_metadata(struct dentry* dentry, struct file_metadata_t* file) {
    struct inode *d_inode;
    bpf_probe_read(&d_inode, sizeof(d_inode), &dentry->d_inode);

    bpf_probe_read(&file->nlink, sizeof(file->nlink), (void *)&d_inode->i_nlink);
    bpf_probe_read(&file->mode, sizeof(file->mode), &d_inode->i_mode);
    bpf_probe_read(&file->uid, sizeof(file->uid), &d_inode->i_uid);
    bpf_probe_read(&file->gid, sizeof(file->gid), &d_inode->i_gid);

    bpf_probe_read(&file->ctime, sizeof(file->ctime), &d_inode->i_ctime);
    bpf_probe_read(&file->mtime, sizeof(file->mtime), &d_inode->i_mtime);
}

void __attribute__((always_inline)) write_dentry_inode(struct dentry *dentry, struct inode **d_inode) {
    bpf_probe_read(d_inode, sizeof(d_inode), &dentry->d_inode);
}

struct dentry* __attribute__((always_inline)) get_file_dentry(struct file *file) {
    struct dentry *file_dentry;
    bpf_probe_read(&file_dentry, sizeof(file_dentry), &file->f_path.dentry);
    return file_dentry;
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

int __attribute__((always_inline)) get_sizeof_inode() {
    u64 sizeof_inode;
    LOAD_CONSTANT("sizeof_inode", sizeof_inode);

    return sizeof_inode;
}

int __attribute__((always_inline)) get_sb_magic_offset() {
    u64 offset;
    LOAD_CONSTANT("sb_magic_offset", offset);

    return offset;
}

#define get_dentry_key_path(dentry, path) (struct path_key_t) { .ino = get_dentry_ino(dentry), .mount_id = get_path_mount_id(path) }
#define get_inode_key_path(inode, path) (struct path_key_t) { .ino = get_inode_ino(inode), .mount_id = get_path_mount_id(path) }

static int is_overlayfs(struct dentry *dentry);
static void set_overlayfs_ino(struct dentry *dentry, u64 *ino, u32 *flags);

static __attribute__((always_inline)) void set_file_inode(struct dentry *dentry, struct file_t *file, int invalidate) {
    file->path_key.path_id = get_path_id(invalidate);
    if (!file->path_key.ino) {
        file->path_key.ino = get_dentry_ino(dentry);
    }

    if (is_overlayfs(dentry)) {
        set_overlayfs_ino(dentry, &file->path_key.ino, &file->flags);
    }
}

// get_sb_magic returns the magic number of a superblock, which can be used to identify the format of the filesystem
static __attribute__((always_inline)) int get_sb_magic(struct super_block *sb) {
    u64 magic;
    bpf_probe_read(&magic, sizeof(magic), (char *)sb + get_sb_magic_offset());

    return magic;
}

static __attribute__((always_inline)) int is_tmpfs(struct dentry *dentry) {
    struct super_block *sb = get_dentry_sb(dentry);
    return get_sb_magic(sb) == TMPFS_MAGIC;
}

#endif
