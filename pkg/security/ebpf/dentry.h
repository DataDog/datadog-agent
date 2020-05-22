#ifndef _DENTRY_H_
#define _DENTRY_H_

#include <linux/dcache.h>
#include <linux/types.h>

#define DENTRY_MAX_DEPTH 16

struct path_key_t {
    unsigned long ino;
    dev_t dev;
    u32 padding;
};

struct path_leaf_t {
  struct path_key_t parent;
  char name[64];
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

int __attribute__((always_inline)) get_inode_mount_id(struct inode *dir) {
    // Mount ID
    int mount_id;
    struct super_block *spb;
    bpf_probe_read(&spb, sizeof(spb), &dir->i_sb);

    struct list_head s_mounts;
    bpf_probe_read(&s_mounts, sizeof(s_mounts), &spb->s_mounts);

    bpf_probe_read(&mount_id, sizeof(int), (void *) s_mounts.next + 172);
    // bpf_probe_read(&mount_id, sizeof(int), &((struct mount *) s_mounts.next)->mnt_id);

    return mount_id;
}

struct dentry * __attribute__((always_inline)) get_inode_mountpoint(struct inode *dir) {
    // Mount ID
    struct dentry *mountpoint = NULL;
    struct super_block *spb;
    bpf_probe_read(&spb, sizeof(spb), &dir->i_sb);

    struct list_head s_mounts;
    bpf_probe_read(&s_mounts, sizeof(s_mounts), &spb->s_mounts);

    // bpf_probe_read(&mountpoint, sizeof(mountpoint), (void *) s_mounts.next - offsetof(struct mount, mnt_instance) + offsetof(struct mount, mnt_mountpoint));
    bpf_probe_read(&mountpoint, sizeof(mountpoint), (void *) s_mounts.next - 88);

    return mountpoint;
}

struct inode * __attribute__((always_inline)) get_dentry_inode(struct dentry *dentry) {
    struct inode *d_inode;
    bpf_probe_read(&d_inode, sizeof(d_inode), &dentry->d_inode);
    return d_inode;
}

unsigned long __attribute__((always_inline)) get_dentry_ino(struct dentry *dentry) {
    struct inode *d_inode;
    bpf_probe_read(&d_inode, sizeof(d_inode), &dentry->d_inode);
    return get_inode_ino(d_inode);
}

struct inode* __attribute__((always_inline)) get_file_inode(struct file *file) {
    struct inode *f_inode;
    bpf_probe_read(&f_inode, sizeof(f_inode), &file->f_inode);
    return f_inode;
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

void __attribute__((always_inline)) get_dentry_name(struct dentry *dentry, void *buffer, size_t n) {
    struct qstr qstr;
    bpf_probe_read(&qstr, sizeof(qstr), &dentry->d_name);
    bpf_probe_read_str(buffer, n, (void *)qstr.name);
}

#define get_dentry_key(dentry) (struct path_key_t) { .ino = get_dentry_ino(dentry), .dev = get_dentry_dev(dentry) }
#define get_inode_key(inode) (struct path_key_t) { .ino = get_inode_ino(inode), .dev = get_inode_dev(dentry) }

static __attribute__((always_inline)) int resolve_dentry(struct dentry *dentry, struct path_key_t key) {
    struct path_leaf_t map_value = {};
    struct path_key_t next_key = key;
    struct qstr qstr;
    struct dentry *d_parent;

#pragma unroll
    for (int i = 0; i < DENTRY_MAX_DEPTH; i++)
    {
        d_parent = NULL;
        bpf_probe_read(&d_parent, sizeof(d_parent), &dentry->d_parent);

        key = next_key;
        if (dentry == d_parent) {
            struct inode *d_inode = get_dentry_inode(dentry);
            dentry = get_inode_mountpoint(d_inode);
            next_key = get_dentry_key(dentry);
            bpf_probe_read(&d_parent, sizeof(d_parent), &dentry->d_parent);
        } else {
            next_key = get_dentry_key(d_parent);
        }

        bpf_probe_read(&qstr, sizeof(qstr), &dentry->d_name);
        bpf_probe_read_str(&map_value.name, sizeof(map_value.name), (void*) qstr.name);

        if (map_value.name[0] == '/' || map_value.name[0] == 0) {
            next_key.ino = 0;
            next_key.dev = 0;
        }

        map_value.parent = next_key;

        if (bpf_map_lookup_elem(&pathnames, &key) == NULL) {
            bpf_map_update_elem(&pathnames, &key, &map_value, BPF_ANY);
        }

        dentry = d_parent;
        if (next_key.ino == 0)
            return i + 1;
    }

    // If the last next_id isn't null, this means that there are still other parents to fetch.
    // TODO: use BPF_PROG_ARRAY to recursively fetch 32 more times. For now, add a fake parent to notify
    // that we couldn't fetch everything.

    if (next_key.ino != 0) {
        map_value.name[0] = map_value.name[0];
        map_value.parent.dev = 0;
        map_value.parent.ino = 0;
        bpf_map_update_elem(&pathnames, &next_key, &map_value, BPF_ANY);
    }

    return DENTRY_MAX_DEPTH;
}

#endif