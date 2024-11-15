#ifndef _EVENTS_H_
#define _EVENTS_H_

#include "constants/custom.h"
#include "structs/all.h"

struct invalidate_dentry_event_t {
    struct kevent_t event;
    u64 inode;
    u32 mount_id;
    u32 padding;
};

struct bind_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    u64 addr[2];
    u16 family;
    u16 port;
};

struct connect_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    u64 addr[2];
    u16 family;
    u16 port;
};

struct bpf_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    struct bpf_map_t map;
    struct bpf_prog_t prog;
    int cmd;
    u32 padding;
};

struct args_envs_event_t {
    struct kevent_t event;
    u64 id;
    u32 size;
    char value[MAX_PERF_STR_BUFF_LEN];
};

struct process_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_context_t syscall_ctx;
    struct process_entry_t proc_entry;
    struct pid_cache_t pid_entry;
    struct linux_binprm_t linux_binprm;
    u64 args_id;
    u64 envs_id;
    u32 args_truncated;
    u32 envs_truncated;
};

struct exit_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    u32 exit_code;
};

struct login_uid_write_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    u32 auid;
};

struct setuid_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    u32 uid;
    u32 euid;
    u32 fsuid;
};

struct setgid_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    u32 gid;
    u32 egid;
    u32 fsgid;
};

struct capset_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    u64 cap_effective;
    u64 cap_permitted;
};

struct cgroup_tracing_event_t {
    struct kevent_t event;
    struct container_context_t container;
    struct activity_dump_config config;
    u64 cookie;
};

struct cgroup_write_event_t {
    struct kevent_t event;
    struct file_t file;
    u32 pid; // pid of the process added to the cgroup
    u32 cgroup_flags;
};

struct utimes_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct file_t file;
    struct ktimeval atime, mtime;
};

struct chmod_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct file_t file;
    u32 mode;
    u32 padding;
};

struct chown_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct file_t file;
    uid_t uid;
    gid_t gid;
};

struct mmap_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    struct file_t file;
    u64 addr;
    u64 offset;
    u64 len;
    u64 protection;
    u64 flags;
};

struct dns_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct network_context_t network;

    u16 id;
    u16 qdcount;
    u16 qtype;
    u16 qclass;
    u16 size;
    char name[DNS_MAX_LENGTH];
};

struct imds_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct network_context_t network;

    u8 body[IMDS_MAX_LENGTH];
};

struct link_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct file_t source;
    struct file_t target;
};

struct mkdir_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
    u32 mode;
    u32 padding;
};

struct init_module_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    struct file_t file;
    char name[MODULE_NAME_LEN];
    char args[128];
    u32 args_truncated;
    u32 loaded_from_memory;
    u32 padding;
};

struct delete_module_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    char name[MODULE_NAME_LEN];
};

struct mount_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct mount_fields_t mountfields;
};

struct unshare_mntns_event_t {
    struct kevent_t event;
    struct mount_fields_t mountfields;
};

struct mprotect_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    u64 vm_start;
    u64 vm_end;
    u64 vm_protection;
    u64 req_protection;
};

struct net_device_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    struct device_t device;
};

struct veth_pair_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    struct device_t host_device;
    struct device_t peer_device;
};

struct open_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct file_t file;
    u32 flags;
    u32 mode;
};

struct ptrace_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    u32 request;
    u32 pid;
    u64 addr;
};

struct syscall_monitor_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;

    u64 event_reason;
    char syscalls[SYSCALL_ENCODING_TABLE_SIZE];
};

struct rename_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct file_t old;
    struct file_t new;
};

struct rmdir_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
};

struct selinux_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct file_t file;
    u32 event_kind;
    union selinux_write_payload_t payload;
};

struct setxattr_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
    char name[MAX_XATTR_NAME_LEN];
};

struct signal_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    u32 pid;
    u32 type;
};

struct splice_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    struct file_t file;
    u32 pipe_entry_flag;
    u32 pipe_exit_flag;
};

struct umount_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    u32 mount_id;
};

struct unlink_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct file_t file;
    u32 flags;
    u32 padding;
};

struct chdir_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct file_t file;
};

struct on_demand_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;

    u32 synth_id;
    char data[256];
};

#endif
