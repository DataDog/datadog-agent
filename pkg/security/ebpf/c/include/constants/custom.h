#ifndef _CONSTANTS_CUSTOM_H
#define _CONSTANTS_CUSTOM_H

#include "macros.h"

#define TTY_NAME_LEN 64
#define CONTAINER_ID_LEN 64
#define MAX_XATTR_NAME_LEN 200
#define CHAR_TO_UINT32_BASE_10_MAX_LEN 11
#define BASENAME_FILTER_SIZE 256
#define FSTYPE_LEN 16
#define MAX_PATH_LEN 256
#define REVISION_ARRAY_SIZE 4096
#define INODE_DISCARDER_TYPE 0

#define PATH_ID_MAP_SIZE 512

#define MAX_PERF_STR_BUFF_LEN 256
#define MAX_STR_BUFF_LEN (1 << 15)
#define MAX_ARRAY_ELEMENT_SIZE 4096
#define MAX_ARRAY_ELEMENT_PER_TAIL 27
#define MAX_ARGS_ELEMENTS (MAX_ARRAY_ELEMENT_PER_TAIL * (32 / 2)) // split tailcall limit
#define MAX_ARGS_READ_PER_TAIL 160

#define EXEC_GET_ENVS_OFFSET 0
#define EXEC_PARSE_ARGS_ENVS_SPLIT 1
#define EXEC_PARSE_ARGS_ENVS 2

#define DENTRY_INVALID -1
#define DENTRY_DISCARDED -2
#define DENTRY_ERROR -3
#define FAKE_INODE_MSW 0xdeadc001UL
#define DR_MAX_TAIL_CALL 29
#define DR_MAX_ITERATION_DEPTH 47
#define DR_MAX_SEGMENT_LENGTH 255
#define DR_NO_CALLBACK -1

enum TAIL_CALL_PROG_TYPE {
    KPROBE_OR_FENTRY_TYPE = 0,
    TRACEPOINT_TYPE = 1,
};

enum DENTRY_RESOLVER_KEYS {
    DR_DENTRY_RESOLVER_KERN_KEY,
    DR_AD_FILTER_KEY,
    DR_DENTRY_RESOLVER_KERN_INPUTS,
    DR_ERPC_KEY,
};

#define DR_ERPC_BUFFER_LENGTH 8 * 4096

enum DENTRY_ERPC_RESOLUTION_CODE {
    DR_ERPC_OK,
    DR_ERPC_CACHE_MISS,
    DR_ERPC_BUFFER_SIZE,
    DR_ERPC_WRITE_PAGE_FAULT,
    DR_ERPC_TAIL_CALL_ERROR,
    DR_ERPC_READ_PAGE_FAULT,
    DR_ERPC_UNKNOWN_ERROR,
};

enum TC_TAIL_CALL_KEYS {
    DNS_REQUEST = 1,
    DNS_REQUEST_PARSER,
    IMDS_REQUEST,
    DNS_RESPONSE
};

enum TC_RAWPACKET_KEYS {
    RAW_PACKET_FILTER,
    // reserved keys for raw packet filter tail calls
};

#define DNS_MAX_LENGTH 256
#define DNS_RECEIVE_MAX_LENGTH 512
#define DNS_EVENT_KEY 0

#define EGRESS 1
#define INGRESS 2
#define PACKET_KEY 0
#define IMDS_EVENT_KEY 0
#define IMDS_MAX_LENGTH 2048

#define STATE_NULL 0
#define STATE_NEWLINK 1
#define STATE_REGISTER_PEER_DEVICE 2

#define RPC_CMD 0xdeadc001

#define FSTYPE_LEN 16

#define SYSCALL_ENCODING_TABLE_SIZE 64 // 64 * 8 = 512 > 450, bytes should be enough to hold all 450 syscalls
#define SYSCALL_MONITOR_TYPE_DUMP 1
#define SYSCALL_MONITOR_TYPE_DRIFT 2

#define SELINUX_WRITE_BUFFER_LEN 64
#define SELINUX_ENFORCE_STATUS_DISABLE_KEY 0
#define SELINUX_ENFORCE_STATUS_ENFORCE_KEY 1

#define EXIT_SYSCALL_KEY 1
#define EXECVE_SYSCALL_KEY 2

#ifndef USE_RING_BUFFER
#if LINUX_VERSION_CODE >= KERNEL_VERSION(5, 8, 0)
#define USE_RING_BUFFER 1
#endif
#endif

#ifndef BPF_OBJ_NAME_LEN
#define BPF_OBJ_NAME_LEN 16U
#endif

#define EVENT_GEN_SIZE 16

#ifndef VALID_OPEN_FLAGS
#define VALID_OPEN_FLAGS                                                    \
    (O_RDONLY | O_WRONLY | O_RDWR | O_CREAT | O_EXCL | O_NOCTTY | O_TRUNC | \
        O_APPEND | O_NDELAY | O_NONBLOCK | __O_SYNC | O_DSYNC |             \
        FASYNC | O_DIRECT | O_LARGEFILE | O_DIRECTORY | O_NOFOLLOW |        \
        O_NOATIME | O_CLOEXEC | O_PATH | __O_TMPFILE)
#endif

#define MAX_SYSCALL_CTX_ENTRIES 8192
#define MAX_SYSCALL_ARG_MAX_SIZE 128
#define MAX_SYSCALL_CTX_SIZE MAX_SYSCALL_ARG_MAX_SIZE * 3 + 4 + 1 // id + types octet + 3 args

__attribute__((always_inline)) u64 is_cgroup_activity_dumps_enabled() {
    u64 cgroup_activity_dumps_enabled;
    LOAD_CONSTANT("cgroup_activity_dumps_enabled", cgroup_activity_dumps_enabled);
    return cgroup_activity_dumps_enabled != 0;
}

#define CGROUP_DEFAULT 1
#define CGROUP_CENTOS_7 2

static __attribute__((always_inline)) u32 get_cgroup_write_type(void) {
    u64 type;
    LOAD_CONSTANT("cgroup_write_type", type);
    return type;
}

static __attribute__((always_inline)) u64 get_discarder_retention() {
    u64 retention = 0;
    LOAD_CONSTANT("discarder_retention", retention);
    return retention ? retention : SEC_TO_NS(5);
}

static __always_inline u32 is_runtime_discarded() {
    u64 discarded = 0;
    LOAD_CONSTANT("runtime_discarded", discarded);
    return discarded;
}

static __attribute__((always_inline)) int is_runtime_request() {
    u64 pid;
    LOAD_CONSTANT("runtime_pid", pid);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    return pid_tgid >> 32 == pid;
}

static __attribute__((always_inline)) u32 get_netns() {
    u64 netns;
    LOAD_CONSTANT("netns", netns);
    return (u32)netns;
}

static __attribute__((always_inline)) u64 get_syscall_monitor_event_period() {
    u64 syscall_monitor_event_period;
    LOAD_CONSTANT("syscall_monitor_event_period", syscall_monitor_event_period);
    return syscall_monitor_event_period;
}

static __attribute__((always_inline)) u64 is_send_signal_available() {
    u64 send_signal;
    LOAD_CONSTANT("send_signal", send_signal);
    return send_signal;
};

static __attribute__((always_inline)) u64 is_anomaly_syscalls_enabled() {
    u64 anomaly;
    LOAD_CONSTANT("anomaly_syscalls", anomaly);
    return anomaly;
};

static __attribute__((always_inline)) u64 get_imds_ip() {
    u64 imds_ip;
    LOAD_CONSTANT("imds_ip", imds_ip);
    return imds_ip;
};

#define CGROUP_MANAGER_UNDEFINED 0
#define CGROUP_MANAGER_DOCKER 1
#define CGROUP_MANAGER_CRIO 2
#define CGROUP_MANAGER_PODMAN 3
#define CGROUP_MANAGER_CRI 4
#define CGROUP_MANAGER_SYSTEMD 5

#define CGROUP_MANAGER_MASK 0xff

#define CGROUP_SYSTEMD_SERVICE (1 << 8)
#define CGROUP_SYSTEMD_SCOPE (1 << 8) + 1

#define ACTIVE_FLOWS_MAX_SIZE 128

enum PID_ROUTE_TYPE
{
    BIND_ENTRY,
    PROCFS_ENTRY,
    FLOW_CLASSIFICATION_ENTRY,
};

enum FLUSH_NETWORK_STATS_TYPE
{
    PID_EXIT,
    PID_EXEC,
    NETWORK_STATS_TICKER,
};

static __attribute__((always_inline)) u64 get_network_monitor_period() {
    u64 network_monitor_period;
    LOAD_CONSTANT("network_monitor_period", network_monitor_period);
    return network_monitor_period;
}

static __attribute__((always_inline)) u64 is_sk_storage_supported() {
    u64 is_sk_storage_supported;
    LOAD_CONSTANT("is_sk_storage_supported", is_sk_storage_supported);
    return is_sk_storage_supported;
}

static __attribute__((always_inline)) u64 is_network_flow_monitor_enabled() {
    u64 is_network_flow_monitor_enabled;
    LOAD_CONSTANT("is_network_flow_monitor_enabled", is_network_flow_monitor_enabled);
    return is_network_flow_monitor_enabled;
}

#define SYSCTL_OK 1

#define MAX_SYSCTL_BUFFER_LEN 1024
#define MAX_SYSCTL_OBJ_LEN 256
#define SYSCTL_EVENT_GEN_KEY 0

#define SYSCTL_NAME_TRUNCATED (1 << 0)
#define SYSCTL_OLD_VALUE_TRUNCATED (1 << 1)
#define SYSCTL_NEW_VALUE_TRUNCATED (1 << 2)
#define MAX_BPF_FILTER_SIZE (511 * sizeof(struct sock_filter))


static __attribute__((always_inline)) u64 has_tracing_helpers_in_cgroup_sysctl() {
    u64 tracing_helpers_in_cgroup_sysctl;
    LOAD_CONSTANT("tracing_helpers_in_cgroup_sysctl", tracing_helpers_in_cgroup_sysctl);
    return tracing_helpers_in_cgroup_sysctl;
}

enum link_target_dentry_origin {
    ORIGIN_UNSET = 0,
    ORIGIN_RETHOOK_FILENAME_CREATE,
    ORIGIN_RETHOOK___LOOKUP_HASH,
};

enum global_rate_limiter_type {
    RAW_PACKET_LIMITER = 0,
};

#define TAIL_CALL_FNC_NAME(name, ...) tail_call_##name(__VA_ARGS__)
#define TAIL_CALL_FNC(name, ...) TAIL_CALL_TARGET("\"" #name "\"") \
	int TAIL_CALL_FNC_NAME(name, __VA_ARGS__)

#define TAIL_CALL_TRACEPOINT_FNC_NAME(name, ...) tail_call_tracepoint_##name(__VA_ARGS__)
#define TAIL_CALL_TRACEPOINT_TARGET(name) SEC("tracepoint/" name)
#define TAIL_CALL_TRACEPOINT_FNC(name, ...) TAIL_CALL_TRACEPOINT_TARGET("\"" #name "\"") \
    int TAIL_CALL_TRACEPOINT_FNC_NAME(name, __VA_ARGS__)

#define TAIL_CALL_FNC_WITH_HOOK_POINT(hookpoint, name, ...) TAIL_CALL_TARGET_WITH_HOOK_POINT(hookpoint) \
    int TAIL_CALL_FNC_NAME(name, __VA_ARGS__)

#define TAIL_CALL_CLASSIFIER_FNC_NAME(name, ...) tail_call_classifier_##name(__VA_ARGS__)
#define TAIL_CALL_CLASSIFIER_TARGET(name) SEC("classifier/" name)
#define TAIL_CALL_CLASSIFIER_FNC(name, ...) TAIL_CALL_CLASSIFIER_TARGET("\"" #name "\"") \
    int TAIL_CALL_CLASSIFIER_FNC_NAME(name, __VA_ARGS__)

#endif
