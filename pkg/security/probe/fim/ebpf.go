package fim

// source - eBPF Probe C source
const source string = `
#include <uapi/linux/ptrace.h>
#include <uapi/linux/limits.h>
#include <linux/sched.h>
#include <linux/fs_struct.h>
#include <linux/dcache.h>
#include <linux/fs.h>
#include <linux/mount.h>
#include <linux/types.h>
#include <linux/fs_pin.h>
#include <linux/mount.h>
#include <linux/tty.h>
#include <linux/sched/signal.h>

#include <linux/nsproxy.h>
#include <linux/pid_namespace.h>
#include <linux/ns_common.h>

enum event_type
{
    EVENT_MAY_OPEN,
    EVENT_VFS_MKDIR,
    EVENT_VFS_LINK,
    EVENT_VFS_RENAME,
    EVENT_VFS_UNLINK,
    EVENT_VFS_RMDIR,
    EVENT_VFS_MODIFY,
};

#define TTY_NAME_LEN 64

struct process_data_t {
    // Process data
    u64  pidns;
    u64  timestamp;
    char tty_name[TTY_NAME_LEN];
    u32  pid;
    u32  tid;
    u32  uid;
    u32  gid;
};

struct dentry_data_t {
    struct process_data_t process_data;
    // Inode create data
    int    flags;
    int    mode;
    int    src_inode;
    u32    src_pathname_key;
    int    src_mount_id;
    int    target_inode;
    u32    target_pathname_key;
    int    target_mount_id;
    int    retval;
    // Probe type
    u32    event;
};
BPF_PERF_OUTPUT(dentry_events);

struct dentry_cache_t {
    struct dentry_data_t data;
    struct inode *src_dir;
    struct dentry *src_dentry;
    struct inode *target_dir;
    struct dentry *target_dentry;
};
BPF_HASH(dentry_cache, u64, struct dentry_cache_t, 1000);

static u64 fill_process_data(struct process_data_t *data) {
    // Process data
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    struct nsproxy *nsproxy = task->nsproxy;
    struct pid_namespace *pid_ns = nsproxy->pid_ns_for_children;
    data->pidns = pid_ns->ns.inum;
    data->timestamp = bpf_ktime_get_ns();
    // TTY
    struct signal_struct *signal = task->signal;
    if (signal->tty != NULL) {
        struct tty_struct *tty = signal->tty;
        bpf_probe_read_str(data->tty_name, TTY_NAME_LEN, tty->name);
    }
    // Pid & Tid
    u64 id = bpf_get_current_pid_tgid();
    data->pid = id >> 32;
    data->tid = id;
    // UID & GID
    u64 userid = bpf_get_current_uid_gid();
    data->uid = userid >> 32;
    data->gid = userid;
    return id;
}

struct path_leaf_t {
  u32 parent;
  char name[NAME_MAX];
};
BPF_HASH(pathnames, u32, struct path_leaf_t, 63000);

// Filter settings
BPF_HASH(filter_settings, u8, u8, 1);

#define FILTER_PID          1 << 0
#define FILTER_PID_NEG      1 << 1
#define FILTER_PIDNS        1 << 2
#define FILTER_PIDNS_NEG    1 << 3
#define FILTER_TTY          1 << 4
#define FILTER_SETTINGS_KEY 1
#define IGNORE 0
#define KEEP 1

// Generic filters
BPF_HASH(pid_filter, u32, u8);
BPF_HASH(pidns_filter, u64, u8);

static inline int filter_pid(u32 pid, u8 flag) {
    u8 *f = pid_filter.lookup(&pid);
    if (!f)
        return (flag == FILTER_PID_NEG);
    if (*f == IGNORE)
        return 0;
    return 1;
}

static inline int filter_pidns(u64 pidns, u8 flag) {
    u8 *f = pidns_filter.lookup(&pidns);
    if (!f)
        return (flag == FILTER_PIDNS_NEG);
    if (*f == IGNORE)
        return 0;
    return 1;
}

static inline int filter(struct process_data_t *data) {
    // Check what filters are activated;
    u8 key = FILTER_SETTINGS_KEY;
    u8 *settings = filter_settings.lookup(&key);
    if (!settings || *settings == 0) {
	    return 1;
    }
    int keep = 1;
    if ((*settings & FILTER_PIDNS) == FILTER_PIDNS) {
		keep &= filter_pidns(data->pidns, FILTER_PIDNS);
    }
    if ((*settings & FILTER_PIDNS_NEG) == FILTER_PIDNS_NEG) {
		keep &= filter_pidns(data->pidns, FILTER_PIDNS_NEG);
    }
    if ((*settings & FILTER_PID) == FILTER_PID) {
		keep &= filter_pid(data->pid, FILTER_PID);
    }
    if ((*settings & FILTER_PID_NEG) == FILTER_PID_NEG) {
		keep &= filter_pid(data->pid, FILTER_PID_NEG);
    }
    if ((*settings & FILTER_TTY) == FILTER_TTY) {
        if (data->tty_name[0] == 0) {
            keep = 0;
        }
    }
    return keep;
}

#define DENTRY_MAX_DEPTH 74

static int resolve_dentry(struct dentry *dentry, u32 pathname_key) {
    struct path_leaf_t map_value = {};
    struct qstr *qstr;
    u32 id;
    u32 next_id = pathname_key;

#pragma unroll
    for (int i = 0; i < DENTRY_MAX_DEPTH; i++)
    {
        qstr = &dentry->d_name;
        id = next_id;
        next_id = (dentry == dentry->d_parent) ? 0 : bpf_get_prandom_u32();
        bpf_probe_read_str(map_value.name, NAME_MAX, qstr->name);
        if (map_value.name[0] == 47 || map_value.name[0] == 0)
            next_id = 0;
        map_value.parent = next_id;
        pathnames.update(&id, &map_value);
        dentry = dentry->d_parent;
        if (next_id == 0)
            return i + 1;
    }

    // If the last next_id isn't null, this means that there are still other parents to fetch.
    // TODO: use BPF_PROG_ARRAY to recursively fetch 32 more times. For now, add a fake parent to notify
    // that we couldn't fetch everything.
    if (next_id != 0) {
        map_value.name[0] = 0;
        map_value.parent = 0;
        pathnames.update(&next_id, &map_value);
    }
    return DENTRY_MAX_DEPTH;
}

int trace_may_open(struct pt_regs *ctx, const struct path *path, int acc_mode, int flag) {
    struct dentry_cache_t data_cache = {};
    // Add process data
    u64 key = fill_process_data(&data_cache.data.process_data);
    // Probe type
    data_cache.data.event = EVENT_MAY_OPEN;

    // Add inode data
    struct dentry *dentry = path->dentry;
    data_cache.data.src_inode = ((struct inode *) dentry->d_inode)->i_ino;
    // Add mode and file data
    data_cache.data.flags = flag;
    data_cache.data.mode = acc_mode;
    // Mount ID
    struct vfsmount *mnt = path->mnt;
    bpf_probe_read(&data_cache.data.src_mount_id, sizeof(int), (void *) mnt + 252);

    // Filter
    if (!filter(&data_cache.data.process_data))
        return 0;

    // Cache event
    data_cache.src_dentry = dentry;
    dentry_cache.update(&key, &data_cache);
    return 0;
}

int trace_ret_may_open(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    struct dentry_cache_t *data_cache = dentry_cache.lookup(&key);
    if (!data_cache)
        return 0;
    struct dentry_data_t data = data_cache->data;
    data.retval = PT_REGS_RC(ctx);

    // Resolve dentry
    data.src_pathname_key = bpf_get_prandom_u32();
    resolve_dentry(data_cache->src_dentry, data.src_pathname_key);
    dentry_events.perf_submit(ctx, &data, sizeof(data));
    dentry_cache.delete(&key);
    return 0;
}

int trace_vfs_mkdir(struct pt_regs *ctx, struct inode *dir, struct dentry *dentry, umode_t mode) {
    struct dentry_cache_t data_cache = {};
    // Add process data
    u64 key = fill_process_data(&data_cache.data.process_data);
    // Probe type
    data_cache.data.event = EVENT_VFS_MKDIR;

    // Add mode
    data_cache.data.mode = (int) mode;

    // Mount ID
    struct super_block *spb = dir->i_sb;
    struct list_head *mnt = spb->s_mounts.next;
    bpf_probe_read(&data_cache.data.src_mount_id, sizeof(int), (void *) mnt + 172);

    // Filter
    if (!filter(&data_cache.data.process_data))
        return 0;

    // Send to cache dentry
    data_cache.src_dentry = dentry;
    dentry_cache.update(&key, &data_cache);
    return 0;
}

int trace_ret_vfs_mkdir(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    struct dentry_cache_t *data_cache = dentry_cache.lookup(&key);
    if (!data_cache)
        return 0;
    struct dentry_data_t data = data_cache->data;
    data.retval = PT_REGS_RC(ctx);

    // Add inode data
    data.src_inode = ((struct inode *) data_cache->src_dentry->d_inode)->i_ino;

    // Resolve dentry
    data.src_pathname_key = bpf_get_prandom_u32();
    resolve_dentry(data_cache->src_dentry, data.src_pathname_key);
    dentry_events.perf_submit(ctx, &data, sizeof(data));
    dentry_cache.delete(&key);
    return 0;
}

int trace_vfs_unlink(struct pt_regs *ctx, struct inode *dir, struct dentry *dentry, struct inode **delegated_inode) {
    struct dentry_cache_t data_cache = {};
    // Add process data
    u64 key = fill_process_data(&data_cache.data.process_data);
    // Probe type
    data_cache.data.event = EVENT_VFS_UNLINK;

    // Add inode data
    data_cache.data.src_inode = ((struct inode *) dentry->d_inode)->i_ino;
    // Add mount ID
    struct super_block *spb = dir->i_sb;
    struct list_head *mnt = spb->s_mounts.next;
    bpf_probe_read(&data_cache.data.src_mount_id, sizeof(int), (void *) mnt + 172);

    // Filter
    if (!filter(&data_cache.data.process_data))
        return 0;

    // Send to cache dentry
    data_cache.src_dentry = dentry;
    dentry_cache.update(&key, &data_cache);
    return 0;
}

int trace_ret_vfs_unlink(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    struct dentry_cache_t *data_cache = dentry_cache.lookup(&key);
    if (!data_cache)
        return 0;
    struct dentry_data_t data = data_cache->data;
    data.retval = PT_REGS_RC(ctx);

    // Resolve dentry
    data.src_pathname_key = bpf_get_prandom_u32();
    resolve_dentry(data_cache->src_dentry, data.src_pathname_key);
    dentry_events.perf_submit(ctx, &data, sizeof(data));
    dentry_cache.delete(&key);
    return 0;
}

int trace_vfs_rmdir(struct pt_regs *ctx, struct inode *dir, struct dentry *dentry) {
    struct dentry_cache_t data_cache = {};
    // Add process data
    u64 key = fill_process_data(&data_cache.data.process_data);
    // Probe type
    data_cache.data.event = EVENT_VFS_RMDIR;

    // Add inode data
    data_cache.data.src_inode = ((struct inode *) dentry->d_inode)->i_ino;
    // Add mount ID
    struct super_block *spb = dir->i_sb;
    struct list_head *mnt = spb->s_mounts.next;
    bpf_probe_read(&data_cache.data.src_mount_id, sizeof(int), (void *) mnt + 172);

    // Filter
    if (!filter(&data_cache.data.process_data))
        return 0;

    // Send to cache
    data_cache.src_dentry = dentry;
    dentry_cache.update(&key, &data_cache);
    return 0;
}

int trace_ret_vfs_rmdir(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    struct dentry_cache_t *data_cache = dentry_cache.lookup(&key);
    if (!data_cache)
        return 0;
    struct dentry_data_t data = data_cache->data;
    data.retval = PT_REGS_RC(ctx);

    // Resolve dentry
    data.src_pathname_key = bpf_get_prandom_u32();
    resolve_dentry(data_cache->src_dentry, data.src_pathname_key);
    dentry_events.perf_submit(ctx, &data, sizeof(data));
    dentry_cache.delete(&key);
    return 0;
}

int trace_vfs_link(struct pt_regs *ctx, struct dentry *old_dentry, struct inode *dir, struct dentry *new_dentry, struct inode **delegated_inode) {
    struct dentry_cache_t data_cache = {};
    // Add process data
    u64 key = fill_process_data(&data_cache.data.process_data);
    // Probe type
    data_cache.data.event = EVENT_VFS_LINK;

    // Add old inode data
    data_cache.data.src_inode = ((struct inode *) old_dentry->d_inode)->i_ino;
    // Add old mount ID
    struct super_block *old_spb = old_dentry->d_sb;
    struct list_head *mnt = old_spb->s_mounts.next;
    bpf_probe_read(&data_cache.data.src_mount_id, sizeof(int), (void *) mnt + 172);

    // Filter
    if (!filter(&data_cache.data.process_data))
        return 0;

    // Resolve old dentry
    data_cache.data.src_pathname_key = bpf_get_prandom_u32();
    resolve_dentry(old_dentry, data_cache.data.src_pathname_key);

    // Send to cache
    data_cache.target_dir = dir;
    data_cache.target_dentry = new_dentry;
    dentry_cache.update(&key, &data_cache);
    return 0;
}

int trace_ret_vfs_link(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    struct dentry_cache_t *data_cache = dentry_cache.lookup(&key);
    if (!data_cache)
        return 0;
    struct dentry_data_t data = data_cache->data;
    data.retval = PT_REGS_RC(ctx);

    // Add target inode data
    data.target_inode = ((struct inode *) data_cache->target_dentry->d_inode)->i_ino;
    // Add target mount ID
    struct super_block *spb = data_cache->target_dir->i_sb;
    struct list_head *mnt = spb->s_mounts.next;
    bpf_probe_read(&data.target_mount_id, sizeof(int), (void *) mnt + 172);

    // Resolve target dentry
    data.target_pathname_key = bpf_get_prandom_u32();
    resolve_dentry(data_cache->target_dentry, data.target_pathname_key);

    // Send event
    dentry_events.perf_submit(ctx, &data, sizeof(data));
    dentry_cache.delete(&key);
    return 0;
}

int trace_vfs_rename(struct pt_regs *ctx, struct inode *old_dir,
                    struct dentry *old_dentry, struct inode *new_dir,
                    struct dentry *new_dentry, struct inode **delegated_inode,
                    unsigned int flags) {
    struct dentry_cache_t data_cache = {};
    // Add process data
    u64 key = fill_process_data(&data_cache.data.process_data);
    // Probe type
    data_cache.data.event = EVENT_VFS_RENAME;

    // Add old inode data
    data_cache.data.src_inode = ((struct inode *) old_dentry->d_inode)->i_ino;
    // Add old mount ID
    struct super_block *old_spb = old_dentry->d_sb;
    struct list_head *mnt = old_spb->s_mounts.next;
    bpf_probe_read(&data_cache.data.src_mount_id, sizeof(int), (void *) mnt + 172);

    // Filter
    if (!filter(&data_cache.data.process_data))
        return 0;

    // Resolve old dentry
    data_cache.data.src_pathname_key = bpf_get_prandom_u32();
    resolve_dentry(old_dentry, data_cache.data.src_pathname_key);

    // Send to cache
    data_cache.target_dir = new_dir;
    data_cache.target_dentry = new_dentry;
    dentry_cache.update(&key, &data_cache);
    return 0;
}

int trace_ret_vfs_rename(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    struct dentry_cache_t *data_cache = dentry_cache.lookup(&key);
    if (!data_cache)
        return 0;
    struct dentry_data_t data = data_cache->data;
    data.retval = PT_REGS_RC(ctx);

    // Add target inode data
    data.target_inode = ((struct inode *) data_cache->target_dentry->d_inode)->i_ino;
    // Add target mount ID
    struct super_block *spb = data_cache->target_dir->i_sb;
    struct list_head *mnt = spb->s_mounts.next;
    bpf_probe_read(&data.target_mount_id, sizeof(int), (void *) mnt + 172);

    // Resolve target dentry
    data.target_pathname_key = bpf_get_prandom_u32();
    resolve_dentry(data_cache->target_dentry, data.target_pathname_key);

    // Send event
    dentry_events.perf_submit(ctx, &data, sizeof(data));
    dentry_cache.delete(&key);
    return 0;
}

int trace_fsnotify_parent(struct pt_regs *ctx, const struct path *path, struct dentry *dentry, __u32 mask) {
    // We only care about file modification (id est FS_MODIFY)
    if (mask != 2) {
        return 0;
    }
    struct dentry_cache_t data_cache = {};
    // Add process data
    u64 key = fill_process_data(&data_cache.data.process_data);
    // Probe type
    data_cache.data.event = EVENT_VFS_MODIFY;

    // Add old inode data
    data_cache.data.src_inode = ((struct inode *) dentry->d_inode)->i_ino;
    // Add old mount point ID
    struct super_block *old_spb = dentry->d_sb;
    bpf_probe_read(&data_cache.data.src_mount_id, 32, old_spb->s_id);

    // Filter
    if (!filter(&data_cache.data.process_data))
        return 0;

    // Send to cache
    data_cache.src_dentry = dentry;
    dentry_cache.update(&key, &data_cache);
    return 0;
}

int trace_ret_fsnotify_parent(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    struct dentry_cache_t *data_cache = dentry_cache.lookup(&key);
    if (!data_cache)
        return 0;
    struct dentry_data_t data = data_cache->data;
    data.retval = PT_REGS_RC(ctx);

    // Resolve dentry
    data.src_pathname_key = bpf_get_prandom_u32();
    resolve_dentry(data_cache->src_dentry, data.src_pathname_key);
    dentry_events.perf_submit(ctx, &data, sizeof(data));
    dentry_cache.delete(&key);
    return 0;
}

struct setattr_t {
    struct  process_data_t process_data;
    int     inode;
    u32     pathname_key;
    int     mount_id;
    u32     flags;
    umode_t	mode;
	kuid_t	uid;
	kgid_t	gid;
	long    atime[2];
	long    mtime[2];
	long    ctime[2];
    int     retval;
};
BPF_PERF_OUTPUT(setattr_events);
BPF_HASH(setattr_cache, u64, struct setattr_t, 1000);

#define SETATTR_MAX_DEPTH 74

static int resolve_setattr_dentry(struct dentry *dentry, u32 pathname_key) {
    struct path_leaf_t map_value = {};
    struct qstr *qstr;
    u32 id;
    u32 next_id = pathname_key;

#pragma unroll
    for (int i = 0; i < SETATTR_MAX_DEPTH; i++)
    {
        qstr = &dentry->d_name;
        id = next_id;
        next_id = (dentry == dentry->d_parent) ? 0 : bpf_get_prandom_u32();
        bpf_probe_read_str(map_value.name, NAME_MAX, qstr->name);
        if (map_value.name[0] == 47 || map_value.name[0] == 0)
            next_id = 0;
        map_value.parent = next_id;
        pathnames.update(&id, &map_value);
        dentry = dentry->d_parent;
        if (next_id == 0)
            return i + 1;
    }

    // If the last next_id isn't null, this means that there are still other parents to fetch.
    // TODO: use BPF_PROG_ARRAY to recursively fetch 32 more times. For now, add a fake parent to notify
    // that we couldn't fetch everything.
    if (next_id != 0) {
        map_value.name[0] = 0;
        map_value.parent = 0;
        pathnames.update(&next_id, &map_value);
    }
    return SETATTR_MAX_DEPTH;
}

int trace_security_inode_setattr(struct pt_regs *ctx, struct dentry *dentry, struct iattr *attr) {
    struct setattr_t data = {};

    // Process data
    u64 key = fill_process_data(&data.process_data);

    // SetAttr data
    data.flags = attr->ia_valid;
    data.mode = attr->ia_mode;
    data.uid = attr->ia_uid;
    data.gid = attr->ia_gid;
    data.atime[0] = attr->ia_atime.tv_sec;
    data.atime[1] = attr->ia_atime.tv_nsec;
    data.mtime[0] = attr->ia_mtime.tv_sec;
    data.mtime[1] = attr->ia_mtime.tv_nsec;
    data.ctime[0] = attr->ia_ctime.tv_sec;
    data.ctime[1] = attr->ia_ctime.tv_nsec;

    // Add inode data
    data.inode = ((struct inode *) dentry->d_inode)->i_ino;
    // Add mount ID
    struct super_block *spb = dentry->d_sb;
    struct list_head *mnt = spb->s_mounts.next;
    bpf_probe_read(&data.mount_id, sizeof(int), (void *) mnt + 172);

    // Dentry data
    data.pathname_key = bpf_get_prandom_u32();
    resolve_dentry(dentry, data.pathname_key);

    // Cache event
    setattr_cache.update(&key, &data);
    return 0;
}

int trace_ret_security_inode_setattr(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    struct setattr_t *data = setattr_cache.lookup(&key);
    if (!data)
        return 0;
    data->retval = PT_REGS_RC(ctx);

    // Send event
    setattr_events.perf_submit(ctx, data, sizeof(*data));
    setattr_cache.delete(&key);
    return 0;
}
`
