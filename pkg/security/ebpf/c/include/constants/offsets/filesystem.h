#ifndef _CONSTANTS_OFFSETS_DENTRY_H_
#define _CONSTANTS_OFFSETS_DENTRY_H_

#include "constants/macros.h"
#include "constants/enums.h"

#define MNT_OFFSETOF_MNT 32 // offsetof(struct mount, mnt)

struct mount;

unsigned long __attribute__((always_inline)) get_inode_ino(struct inode *inode) {
    u64 inode_ino_offset;
    LOAD_CONSTANT("inode_ino_offset", inode_ino_offset);

    unsigned long ino;
    bpf_probe_read(&ino, sizeof(inode), (void *)inode + inode_ino_offset);
    return ino;
}

struct inode* get_dentry_inode(struct dentry *dentry) {
    u64 offset;
    LOAD_CONSTANT("dentry_d_inode_offset", offset);

    struct inode *inode;
    bpf_probe_read(&inode, sizeof(inode), (void *)dentry + offset);
    return inode;
}

dev_t __attribute__((always_inline)) get_sb_dev(struct super_block *sb) {
    u64 sb_dev_offset;
    LOAD_CONSTANT("sb_dev_offset", sb_dev_offset);

    dev_t dev;
    bpf_probe_read(&dev, sizeof(dev), (void *)sb + sb_dev_offset);
    return dev;
}

dev_t __attribute__((always_inline)) get_inode_dev(struct inode *inode) {
    u64 offset;
    LOAD_CONSTANT("inode_sb_offset", offset);

    struct super_block *sb;
    bpf_probe_read(&sb, sizeof(sb), (void *)inode + offset);
    return get_sb_dev(sb);
}

dev_t __attribute__((always_inline)) get_dentry_dev(struct dentry *dentry) {
    u64 offset;
    LOAD_CONSTANT("dentry_d_sb_offset", offset);

    struct super_block *sb;
    bpf_probe_read(&sb, sizeof(sb), (char *)dentry + offset);
    return get_sb_dev(sb);
}

void *__attribute__((always_inline)) get_file_f_inode_addr(struct file *file) {
    u64 offset;
    LOAD_CONSTANT("file_f_inode_offset", offset);
    return (char *)file + offset;
}

struct path *__attribute__((always_inline)) get_file_f_path_addr(struct file *file) {
    u64 offset;
    LOAD_CONSTANT("file_f_path_offset", offset);
    return (struct path *)((char *)file + offset);
}

u64 __attribute__((always_inline)) security_have_usernamespace_first_arg(void) {
    u64 flag;
    LOAD_CONSTANT("has_usernamespace_first_arg", flag);
    return flag;
}

u32 __attribute__((always_inline)) get_mount_offset_of_mount_id(void) {
    u64 offset;
    LOAD_CONSTANT("mount_id_offset", offset);
    return offset; // offsetof(struct mount, mnt_id)
}

int __attribute__((always_inline)) get_vfsmount_mount_id(struct vfsmount *mnt) {
    int mount_id;
    // bpf_probe_read(&mount_id, sizeof(mount_id), (char *)mnt + offsetof(struct mount, mnt_id) - offsetof(struct mount, mnt));
    bpf_probe_read(&mount_id, sizeof(mount_id), (char *)mnt + get_mount_offset_of_mount_id() - MNT_OFFSETOF_MNT);
    return mount_id;
}

struct vfsmount* __attribute__((always_inline)) get_path_vfsmount(struct path *path) {
    u64 offset;
    LOAD_CONSTANT("path_mnt_offset", offset);

    struct vfsmount *mnt;
    bpf_probe_read(&mnt, sizeof(mnt), (void *)path + offset);
    return mnt;
}

int __attribute__((always_inline)) get_path_mount_id(struct path *path) {
    return get_vfsmount_mount_id(get_path_vfsmount(path));
}

int __attribute__((always_inline)) get_file_mount_id(struct file *file) {
    return get_path_mount_id(get_file_f_path_addr(file));
}

int __attribute__((always_inline)) get_vfsmount_mount_flags(struct vfsmount *mnt) {
    u64 offset;
    LOAD_CONSTANT("vfsmount_mnt_flags_offset", offset);

    int mount_flags;
    bpf_probe_read(&mount_flags, sizeof(mount_flags), (void *)mnt + offset);
    return mount_flags;
}

int __attribute__((always_inline)) get_path_mount_flags(struct path *path) {
    return get_vfsmount_mount_flags(get_path_vfsmount(path));
}

int __attribute__((always_inline)) get_mount_mount_id(void *mnt) {
    int mount_id;

    bpf_probe_read(&mount_id, sizeof(mount_id), (char *)mnt + get_mount_offset_of_mount_id());
    return mount_id;
}

struct dentry *__attribute__((always_inline)) get_mount_mountpoint_dentry(struct mount *mnt) {
	u64 mount_mnt_mountpoint_offset;
	LOAD_CONSTANT("mount_mnt_mountpoint_offset", mount_mnt_mountpoint_offset);

    struct dentry *dentry;
    bpf_probe_read(&dentry, sizeof(dentry), (void *)mnt + mount_mnt_mountpoint_offset);
    return dentry;
}

struct vfsmount *__attribute__((always_inline)) get_mount_vfsmount(void *mnt) {
    return (struct vfsmount *)(mnt + MNT_OFFSETOF_MNT);
}

struct dentry *__attribute__((always_inline)) get_vfsmount_dentry(struct vfsmount *mnt) {
	u64 offset;
	LOAD_CONSTANT("vfsmount_mnt_root_offset", offset);

    struct dentry *dentry;
    bpf_probe_read(&dentry, sizeof(dentry), (void *)mnt + offset);
    return dentry;
}

struct super_block *__attribute__((always_inline)) get_dentry_sb(struct dentry *dentry) {
    u64 offset;
    LOAD_CONSTANT("dentry_sb_offset", offset);

    struct super_block *sb;
    bpf_probe_read(&sb, sizeof(sb), (char *)dentry + offset);
    return sb;
}

struct file_system_type *__attribute__((always_inline)) get_super_block_fs(struct super_block *sb) {
	u64 offset;
	LOAD_CONSTANT("super_block_s_type_offset", offset);

    struct file_system_type *fs;
    bpf_probe_read(&fs, sizeof(fs), (void *)sb + offset);
    return fs;
}

struct super_block *__attribute__((always_inline)) get_vfsmount_sb(struct vfsmount *mnt) {
	u64 offset;
	LOAD_CONSTANT("vfsmount_mnt_sb_offset", offset);

    struct super_block *sb;
    bpf_probe_read(&sb, sizeof(sb), (void *)mnt + offset);
    return sb;
}

struct dentry *__attribute__((always_inline)) get_mountpoint_dentry(void *mntpoint) {
	u64 offset;
	LOAD_CONSTANT("mountpoint_dentry_offset", offset);

    struct dentry *dentry;
    bpf_probe_read(&dentry, sizeof(dentry), (void *)mntpoint + offset);
    return dentry;
}

dev_t __attribute__((always_inline)) get_vfsmount_dev(struct vfsmount *mnt) {
    return get_sb_dev(get_vfsmount_sb(mnt));
}

dev_t __attribute__((always_inline)) get_mount_dev(void *mnt) {
    return get_vfsmount_dev(get_mount_vfsmount(mnt));
}

unsigned long __attribute__((always_inline)) get_dentry_ino(struct dentry *dentry) {
    if (!dentry) {
        return 0;
    }
    return get_inode_ino(get_dentry_inode(dentry));
}

struct dentry *__attribute__((always_inline)) get_path_dentry(struct path *path) {
    u64 offset;
    LOAD_CONSTANT("path_dentry_offset", offset);

    struct dentry *dentry;
    bpf_probe_read(&dentry, sizeof(dentry), (void *)path + offset);
    return dentry;
}

u32  __attribute__((always_inline)) get_dentry_nlink(struct dentry* dentry) {
    struct inode *d_inode = get_dentry_inode(dentry);

    u64 inode_nlink_offset;
    LOAD_CONSTANT("inode_nlink_offset", inode_nlink_offset);

    int nlink = 0;
    bpf_probe_read(&nlink, sizeof(nlink), (void *)d_inode + inode_nlink_offset);

    return nlink;
}

struct dentry *__attribute__((always_inline)) get_file_dentry(struct file *file) {
    return get_path_dentry(get_file_f_path_addr(file));
}

unsigned long __attribute__((always_inline)) get_path_ino(struct path *path) {
    struct dentry *dentry = get_path_dentry(path);
    return get_dentry_ino(dentry);
}

void __attribute__((always_inline)) get_dentry_name(struct dentry *dentry, void *buffer, size_t n) {
	u64 dentry_d_name_offset;
	LOAD_CONSTANT("dentry_d_name_offset", dentry_d_name_offset);

    struct qstr qstr;
    bpf_probe_read(&qstr, sizeof(qstr), (void *)dentry + dentry_d_name_offset);
    bpf_probe_read_str(buffer, n, (void *)qstr.name);
}

int __attribute__((always_inline)) get_sizeof_inode() {
    u64 sizeof_inode;
    LOAD_CONSTANT("sizeof_inode", sizeof_inode);
    return sizeof_inode;
}

u64 __attribute__((always_inline)) get_ovl_path_in_inode() {
    u64 ovl_path_in_ovl_inode;
    LOAD_CONSTANT("ovl_path_in_ovl_inode", ovl_path_in_ovl_inode);
    return ovl_path_in_ovl_inode;
}

int __attribute__((always_inline)) get_sb_magic_offset() {
    u64 offset;
    LOAD_CONSTANT("sb_magic_offset", offset);
    return offset;
}

int __attribute__((always_inline)) get_sb_flags_offset() {
    u64 offset;
    LOAD_CONSTANT("sb_flags_offset", offset);
    return offset;
}

// get_sb_magic returns the magic number of a superblock, which can be used to identify the format of the filesystem
static __attribute__((always_inline)) int get_sb_magic(struct super_block *sb) {
    u64 magic;
    bpf_probe_read(&magic, sizeof(magic), (char *)sb + get_sb_magic_offset());
    return magic;
}

static __attribute__((always_inline)) int get_sb_flags(struct super_block *sb) {
    u64 s_flags;
    bpf_probe_read(&s_flags, sizeof(s_flags), (char *)sb + get_sb_flags_offset());
    return s_flags;
}

// https://elixir.bootlin.com/linux/v6.13.7/source/include/uapi/linux/mount.h#L47
#define MS_NOUSER (1 << 31)

static __attribute__((always_inline)) int is_non_mountable_dentry(struct dentry *dentry) {
    struct super_block *sb = get_dentry_sb(dentry);
    return get_sb_flags(sb) & MS_NOUSER;
}

static __attribute__((always_inline)) int is_tmpfs(struct dentry *dentry) {
    struct super_block *sb = get_dentry_sb(dentry);
    return get_sb_magic(sb) == TMPFS_MAGIC;
}

static __attribute__((always_inline)) int is_overlayfs(struct dentry *dentry) {
    struct super_block *sb = get_dentry_sb(dentry);
    return get_sb_magic(sb) == OVERLAYFS_SUPER_MAGIC;
}

int __attribute__((always_inline)) get_ovl_lower_ino_direct(struct dentry *dentry) {
    struct inode *d_inode = get_dentry_inode(dentry);

    // escape from the embedded vfs_inode to reach ovl_inode
    struct inode *lower;
    bpf_probe_read(&lower, sizeof(lower), (char *)d_inode + get_sizeof_inode() + 8);

    return get_inode_ino(lower);
}

struct dentry * __attribute__((always_inline)) get_ovl_lower_dentry_from_ovl_path(struct dentry *dentry) {
    struct inode *d_inode = get_dentry_inode(dentry);

    // escape from the embedded vfs_inode to reach ovl_inode
    struct dentry *lower;
    bpf_probe_read(&lower, sizeof(lower), (char *)d_inode + get_sizeof_inode() + 16);

    return lower;
}

int __attribute__((always_inline)) get_ovl_lower_ino_from_ovl_path(struct dentry *dentry) {
    struct dentry *lower = get_ovl_lower_dentry_from_ovl_path(dentry);
    return get_dentry_ino(lower);
}

struct dentry * __attribute__((always_inline)) get_ovl_lower_dentry_from_ovl_entry(struct dentry *dentry) {
    struct inode *d_inode = get_dentry_inode(dentry);

    // escape from the embedded vfs_inode to reach ovl_entry
    // struct inode vfs_inode;
    // struct dentry *__upperdentry;
    // struct ovl_entry *oe;
    void *oe;
    bpf_probe_read(&oe, sizeof(oe), (char *)d_inode + get_sizeof_inode() + 8);

    struct dentry *lower;
    // 4 for the __num_lower field + 4 of padding + 8 for the layer ptr in ovl_path
    bpf_probe_read(&lower, sizeof(lower), (char *)oe + 4 + 4 + 8);

    return lower;
}

int __attribute__((always_inline)) get_ovl_lower_ino_from_ovl_entry(struct dentry *dentry) {
    struct dentry *lower = get_ovl_lower_dentry_from_ovl_entry(dentry);
    return get_dentry_ino(lower);
}

struct dentry * __attribute__((always_inline)) get_ovl_upper_dentry(struct dentry *dentry) {
    struct inode *d_inode = get_dentry_inode(dentry);

    // escape from the embedded vfs_inode to reach upper dentry
    // struct inode vfs_inode;
    // struct dentry *__upperdentry;
    struct dentry *upper;
    bpf_probe_read(&upper, sizeof(upper), (char *)d_inode + get_sizeof_inode());

    return upper;
}

int __attribute__((always_inline)) get_ovl_upper_ino(struct dentry *dentry) {
    struct dentry *upper = get_ovl_upper_dentry(dentry);
    return get_dentry_ino(upper);
}

int __attribute__((always_inline)) get_ovl_lower_ino(struct dentry *dentry) {
    switch (get_ovl_path_in_inode()) {
    case 2:
        return get_ovl_lower_ino_from_ovl_entry(dentry);
    case 1:
        return get_ovl_lower_ino_from_ovl_path(dentry);
    }
    return get_ovl_lower_ino_direct(dentry);
}

int __attribute__((always_inline)) get_ovl_upper_nlink(struct dentry *dentry) {
    struct dentry *upper = get_ovl_upper_dentry(dentry);
    return get_dentry_nlink(upper);
}

int __attribute__((always_inline)) get_ovl_lower_nlink_from_ovl_entry(struct dentry *dentry) {
    struct dentry *lower = get_ovl_lower_dentry_from_ovl_entry(dentry);
    return get_dentry_nlink(lower);
}

int __attribute__((always_inline)) get_ovl_lower_nlink_from_ovl_path(struct dentry *dentry) {
    struct dentry *lower = get_ovl_lower_dentry_from_ovl_path(dentry);
    return get_dentry_nlink(lower);
}

int __attribute__((always_inline)) get_ovl_lower_nlink(struct dentry *dentry) {
    if (get_ovl_path_in_inode() == 2) {
        return get_ovl_lower_nlink_from_ovl_entry(dentry);
    }
    return get_ovl_lower_nlink_from_ovl_path(dentry);
}

int __always_inline get_overlayfs_layer(struct dentry *dentry) {
    return get_ovl_upper_ino(dentry) != 0 ? UPPER_LAYER : LOWER_LAYER;
}

void __always_inline set_overlayfs_inode(struct dentry *dentry, struct file_t *file) {
    u64 orig_inode = file->path_key.ino;
    u64 lower_inode = get_ovl_lower_ino(dentry);
    u64 upper_inode = get_ovl_upper_ino(dentry);

    // NOTE(safchain) both lower & upper inode seems to be incorrect sometimes on kernel >= 6.8.
    // Need to investigate the root cause.
    if (get_ovl_path_in_inode() == 2 && lower_inode != orig_inode && upper_inode != orig_inode) {
        return;
    }

    if (lower_inode) {
        file->path_key.ino = lower_inode;
    } else if (upper_inode) {
        file->path_key.ino = upper_inode;
    }

    file->flags |= upper_inode != 0 ? UPPER_LAYER : LOWER_LAYER;
}

void __always_inline set_overlayfs_nlink(struct dentry *dentry, struct file_t *file) {
    u64 orig_nlink = file->metadata.nlink;
    u64 lower_nlink = get_ovl_lower_nlink(dentry);
    u64 upper_nlink = get_ovl_upper_nlink(dentry);

    // keep the highest nlink as it is mostly used to discard the userspace cache
    if (lower_nlink > orig_nlink) {
        file->metadata.nlink = lower_nlink;
    } else if (upper_nlink > orig_nlink) {
        file->metadata.nlink = upper_nlink;
    }
}

#define VFS_ARG_POSITION1 1
#define VFS_ARG_POSITION2 2
#define VFS_ARG_POSITION3 3
#define VFS_ARG_POSITION4 4
#define VFS_ARG_POSITION5 5
#define VFS_ARG_POSITION6 6

static __attribute__((always_inline)) u64 get_vfs_unlink_dentry_position() {
    u64 vfs_unlink_dentry_position;
    LOAD_CONSTANT("vfs_unlink_dentry_position", vfs_unlink_dentry_position);
    return vfs_unlink_dentry_position;
}

static __attribute__((always_inline)) u64 get_vfs_mkdir_dentry_position() {
    u64 vfs_mkdir_dentry_position;
    LOAD_CONSTANT("vfs_mkdir_dentry_position", vfs_mkdir_dentry_position);
    return vfs_mkdir_dentry_position;
}

static __attribute__((always_inline)) u64 get_vfs_setxattr_dentry_position() {
    u64 vfs_setxattr_dentry_position;
    LOAD_CONSTANT("vfs_setxattr_dentry_position", vfs_setxattr_dentry_position);
    return vfs_setxattr_dentry_position;
}

static __attribute__((always_inline)) u64 get_vfs_removexattr_dentry_position() {
    u64 vfs_removexattr_dentry_position;
    LOAD_CONSTANT("vfs_removexattr_dentry_position", vfs_removexattr_dentry_position);
    return vfs_removexattr_dentry_position;
}

#define VFS_RENAME_REGISTER_INPUT 1
#define VFS_RENAME_STRUCT_INPUT 2

static __attribute__((always_inline)) u64 get_vfs_rename_input_type() {
    u64 vfs_rename_input_type;
    LOAD_CONSTANT("vfs_rename_input_type", vfs_rename_input_type);
    return vfs_rename_input_type;
}

static __attribute__((always_inline)) u64 get_vfs_rename_src_dentry_offset() {
    u64 offset;
    LOAD_CONSTANT("vfs_rename_src_dentry_offset", offset);
    return offset ? offset : 16; // offsetof(struct renamedata, old_dentry)
}

static __attribute__((always_inline)) u64 get_vfs_rename_target_dentry_offset() {
    u64 offset;
    LOAD_CONSTANT("vfs_rename_target_dentry_offset", offset);
    return offset ? offset : 40; // offsetof(struct renamedata, new_dentry)
}

static __attribute__((always_inline)) u64 get_iokiocb_ctx_offset() {
    u64 iokiocb_ctx_offset;
    LOAD_CONSTANT("iokiocb_ctx_offset", iokiocb_ctx_offset);
    return iokiocb_ctx_offset;
}

static __attribute__((always_inline)) u64 get_getattr2() {
    u64 getattr2;
    LOAD_CONSTANT("getattr2", getattr2);
    return getattr2;
}

#endif
