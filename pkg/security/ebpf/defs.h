#ifndef _DEFS_H
#define _DEFS_H 1

#define TTY_NAME_LEN 64

# define printk(fmt, ...)						\
		({							\
			char ____fmt[] = fmt;				\
			bpf_trace_printk(____fmt, sizeof(____fmt),	\
				     ##__VA_ARGS__);			\
		})

enum event_type
{
    EVENT_MAY_OPEN = 1,
    EVENT_VFS_MKDIR,
    EVENT_VFS_LINK,
    EVENT_VFS_RENAME,
    EVENT_VFS_UNLINK,
    EVENT_VFS_RMDIR,
    EVENT_VFS_MODIFY,
};

struct event_t {
    u64 type;
    u64 timestamp;
    u64 retval;
};

struct process_data_t {
    // Process data
    u64  pidns;
    char comm[TASK_COMM_LEN];
    char tty_name[TTY_NAME_LEN];
    u32  pid;
    u32  tid;
    u32  uid;
    u32  gid;
};

struct dentry_data_t {
    struct event_t event;
    struct process_data_t process_data;
    int    flags;
    int    mode;
    int    src_inode;
    u32    src_pathname_key;
    int    src_mount_id;
    int    target_inode;
    u32    target_pathname_key;
    int    target_mount_id;
};

struct unlink_event_t {
    struct process_data_t process;
    int    inode;
    u32    pathname_key;
    int    mount_id;
};

struct dentry_cache_t {
    struct dentry_data_t data;
    struct inode *src_dir;
    struct dentry *src_dentry;
    struct inode *target_dir;
    struct dentry *target_dentry;
};

struct process_discriminator_t {
    char comm[TASK_COMM_LEN];
};

struct path_leaf_t {
  u32 parent;
  char name[NAME_MAX];
};

struct bpf_map_def SEC("maps/dentry_cache") dentry_cache = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(struct dentry_cache_t),
    .max_entries = 256,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/process_discriminators") process_discriminators = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(struct process_discriminator_t),
    .value_size = sizeof(u8),
    .max_entries = 256,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/dentry_events") dentry_events = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .key_size = sizeof(__u32),
    .value_size = sizeof(__u32),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/test") test = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u32),
    .value_size = sizeof(__u32),
    .max_entries = 1234,
    .pinning = 0,
    .namespace = "",
};

#endif