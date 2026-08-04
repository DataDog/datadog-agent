#ifndef _EVENTS_H_
#define _EVENTS_H_

#include "constants/custom.h"
#include "structs/all.h"
#include <uapi/linux/filter.h>


struct invalidate_dentry_event_t {
    struct kevent_t event;
    u64 inode;
    u32 mount_id;
    u32 padding;
};

struct accept_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;

    u64 addr[2];
    u16 family;
    u16 port;
};

struct bind_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;

    u64 addr[2];
    u16 family;
    u16 port;
    u16 protocol;
    u16 padding;
    u32 sample_cookie;
    u32 sample_padding;
};

struct socket_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;

    u16 domain;
    u16 type;
    u16 protocol;
    u16 padding;
};

struct connect_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;

    u64 addr[2];
    u16 family;
    u16 port;
    u16 protocol;
    u16 padding;
    u32 sample_cookie;
    u32 sample_padding;
};

struct bpf_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
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
    struct cgroup_context_t cgroup;
    struct syscall_context_t syscall_ctx;
    struct process_entry_t proc_entry;
    struct pid_cache_t pid_entry;
    struct linux_binprm_t linux_binprm;
    u64 args_id;
    u64 envs_id;
    u32 args_truncated;
    u32 envs_truncated;
    u32 is_through_symlink;
};

struct exit_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    u32 exit_code;
};

struct login_uid_write_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    u32 auid;
};

struct setuid_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    u32 uid;
    u32 euid;
    u32 fsuid;
};

struct setgid_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    u32 gid;
    u32 egid;
    u32 fsgid;
};

struct capset_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    u64 cap_effective;
    u64 cap_permitted;
};

struct cgroup_tracing_event_t {
    struct kevent_t event;
    struct cgroup_context_t cgroup;
    struct activity_dump_config config;
    u64 cookie;
    u32 pid;
};

struct cgroup_write_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct path_key_t path_key;
    u32 pid; // pid of the process added to the cgroup
};

struct utimes_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct file_t file;
    struct ktimeval atime, mtime;
};

struct chmod_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
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
    struct cgroup_context_t cgroup;
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
    struct cgroup_context_t cgroup;
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
    struct cgroup_context_t cgroup;
    struct network_context_t network;

    u16 id;
    u16 qdcount;
    u16 qtype;
    u16 qclass;
    u16 size;
    char name[DNS_MAX_LENGTH];
};

struct short_dns_response_event_t {
    struct kevent_t event;

    struct dnshdr header;
    char data[DNS_RECEIVE_MAX_LENGTH];
};

struct full_dns_response_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct network_context_t network;

    struct dnshdr header;
    char data[DNS_RECEIVE_MAX_LENGTH];
};

union dns_responses_t {
    struct short_dns_response_event_t short_dns_response;
    struct full_dns_response_event_t full_dns_response;
};

struct imds_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct network_context_t network;

    u8 body[IMDS_MAX_LENGTH];
};

struct link_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct file_t source;
    struct file_t target;
};

struct mkdir_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct file_t file;
    u32 mode;
    u32 padding;
};

struct init_module_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
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
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;

    char name[MODULE_NAME_LEN];
};

struct mount_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct mount_fields_t mountfields;
    u32    source;
};

struct unshare_mntns_event_t {
    struct kevent_t event;
    struct mount_fields_t mountfields;
};

struct mprotect_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
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
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;

    struct device_t device;
};

struct veth_pair_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;

    struct device_t host_device;
    struct device_t peer_device;
};

struct open_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct file_t file;
    u32 flags;
    u32 mode;
    u32 sample_cookie;
    u32 sample_padding;
};

struct ptrace_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;

    u32 request;
    u32 pid;
    u64 addr;
    u32 ns_pid;
};

struct syscall_monitor_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;

    u64 event_reason;
    char syscalls[SYSCALL_ENCODING_TABLE_SIZE];
};

struct rename_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct file_t old;
    struct file_t new;
};

struct rmdir_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct file_t file;
};

struct selinux_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct file_t file;
    u32 event_kind;
    union selinux_write_payload_t payload;
};

struct setxattr_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;
    struct file_t file;
    char name[MAX_XATTR_NAME_LEN];
};

struct signal_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;

    u32 pid;
    u32 type;
};

struct splice_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;

    struct file_t file;
    u32 pipe_entry_flag;
    u32 pipe_exit_flag;
};

struct umount_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;
    u32 mount_id;
};

struct unlink_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
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
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;
    struct syscall_context_t syscall_ctx;
    struct file_t file;
};

#define ON_DEMAND_PER_ARG_SIZE 64

struct on_demand_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;

    u32 synth_id;
    char data[ON_DEMAND_PER_ARG_SIZE * 6];
};

struct raw_packet_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct network_device_context_t device;

    u32 len;
    char data[256];
};

struct network_flow_monitor_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct network_device_context_t device;

    u64 flows_count; // keep as u64 to prevent inconsistent verifier output on bounds checks
    struct flow_stats_t flows[ACTIVE_FLOWS_MAX_SIZE];
};

struct sysctl_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;

    u32 action;
    u32 file_position;
    u16 name_len;
    u16 old_value_len;
    u16 new_value_len;
    u16 flags;
    char sysctl_buffer[MAX_SYSCTL_BUFFER_LEN];
};

struct setrlimit_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;

    int resource;
    u32 target;
    u64 rlim_cur;
    u64 rlim_max;
};

struct setsockopt_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;

    u16 socket_type;
    u16 socket_family;
    u16 filter_len;
    u16 socket_protocol;
    int level;
    int optname;
    u32 truncated;
    int sent_size;
    char bpf_filters_buffer[MAX_BPF_FILTER_SIZE];
};

struct capabilities_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct capabilities_usage_t caps_usage;
};

struct prctl_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;

    int option;
    u32 sent_size;
    u32 name_truncated;
    char name[MAX_PRCTL_NAME_LEN];
};

struct tracer_memfd_seal_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct cgroup_context_t cgroup;
    struct syscall_t syscall;

    u32 fd;
};

struct sample_refresh_event_t {
    struct kevent_t event;
    u32 cookie;
    u32 padding;
};

struct nop_event_t {
    struct kevent_t event;
};

// event_t is a max-sized overlay of every event type. It is used to size a
// shared per-CPU staging buffer that a single generic program can emit any
// event from (the events ring buffer is untyped: send_event_with_size_ptr takes
// an opaque payload + size, with the concrete type carried in the kevent_t
// header). It is never dereferenced as a union; members exist only so that
// sizeof(union event_t) tracks the largest event automatically.
union event_t {
    struct invalidate_dentry_event_t invalidate_dentry;
    struct accept_event_t accept;
    struct bind_event_t bind;
    struct socket_event_t socket;
    struct connect_event_t connect;
    struct bpf_event_t bpf;
    struct args_envs_event_t args_envs;
    struct process_event_t process;
    struct exit_event_t exit;
    struct login_uid_write_event_t login_uid_write;
    struct setuid_event_t setuid;
    struct setgid_event_t setgid;
    struct capset_event_t capset;
    struct cgroup_tracing_event_t cgroup_tracing;
    struct cgroup_write_event_t cgroup_write;
    struct utimes_event_t utimes;
    struct chmod_event_t chmod;
    struct chown_event_t chown;
    struct mmap_event_t mmap;
    struct dns_event_t dns;
    struct short_dns_response_event_t short_dns_response;
    struct full_dns_response_event_t full_dns_response;
    struct imds_event_t imds;
    struct link_event_t link;
    struct mkdir_event_t mkdir;
    struct init_module_event_t init_module;
    struct delete_module_event_t delete_module;
    struct mount_event_t mount;
    struct unshare_mntns_event_t unshare_mntns;
    struct mprotect_event_t mprotect;
    struct net_device_event_t net_device;
    struct veth_pair_event_t veth_pair;
    struct open_event_t open;
    struct ptrace_event_t ptrace;
    struct syscall_monitor_event_t syscall_monitor;
    struct rename_event_t rename;
    struct rmdir_event_t rmdir;
    struct selinux_event_t selinux;
    struct setxattr_event_t setxattr;
    struct signal_event_t signal;
    struct splice_event_t splice;
    struct umount_event_t umount;
    struct unlink_event_t unlink;
    struct chdir_event_t chdir;
    struct on_demand_event_t on_demand;
    struct raw_packet_event_t raw_packet;
    struct network_flow_monitor_event_t network_flow_monitor;
    struct sysctl_event_t sysctl;
    struct setrlimit_event_t setrlimit;
    struct setsockopt_event_t setsockopt;
    struct capabilities_event_t capabilities;
    struct prctl_event_t prctl;
    struct tracer_memfd_seal_event_t tracer_memfd_seal;
    struct sample_refresh_event_t sample_refresh;
    struct nop_event_t nop;
};

// span_fill_slot_t is the value type of the span_fill_event staging map (see
// maps.h). It wraps the max-sized event payload (union event_t) with a small
// kernel-only header carrying what the generic fill_span_and_send tail program
// cannot otherwise recover: the event type, the number of bytes to emit, and the
// offset of the span field within the (type-erased) payload. The header is
// scratch: only `data` (for `size` bytes) is sent to userspace.
struct span_fill_slot_t {
    u64 event_type; // EVENT_* type passed to send_event
    u32 size;       // number of bytes of `data` to emit
    u32 span_off;   // byte offset of the span field within `data`
    union event_t data;
};

#endif
