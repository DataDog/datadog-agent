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
#define PID_DISCARDER_TYPE 1
#define ERPC_MONITOR_KEY 1
#define DISCARDER_MONITOR_KEY 2

#define MAX_PERF_STR_BUFF_LEN 256
#define MAX_STR_BUFF_LEN (1 << 15)
#define MAX_ARRAY_ELEMENT_SIZE 4096
#define MAX_ARRAY_ELEMENT_PER_TAIL 28
#define MAX_ARGS_ELEMENTS (MAX_ARRAY_ELEMENT_PER_TAIL * (32 / 2)) // split tailcall limit
#define MAX_ARGS_READ_PER_TAIL 208

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

#define DR_KPROBE     1
#define DR_TRACEPOINT 2

#define DR_ERPC_KEY                        0
#define DR_ERPC_PARENT_KEY                 1
#define DR_ERPC_SEGMENT_KEY                2
#define DR_KPROBE_DENTRY_RESOLVER_KERN_KEY 3
#define DR_KPROBE_AD_FILTER_KEY            4

#define DR_TRACEPOINT_DENTRY_RESOLVER_KERN_KEY 0
#define DR_TRACEPOINT_AD_FILTER_KEY            1

#define DR_ERPC_BUFFER_LENGTH 8*4096

#define DR_ERPC_OK                0
#define DR_ERPC_CACHE_MISS        1
#define DR_ERPC_BUFFER_SIZE       2
#define DR_ERPC_WRITE_PAGE_FAULT  3
#define DR_ERPC_TAIL_CALL_ERROR   4
#define DR_ERPC_READ_PAGE_FAULT   5
#define DR_ERPC_UNKNOWN_ERROR     6

#define DNS_MAX_LENGTH 256
#define DNS_EVENT_KEY 0
#define DNS_REQUEST        1
#define DNS_REQUEST_PARSER 2

#define EGRESS 1
#define INGRESS 2
#define ACT_OK TC_ACT_UNSPEC
#define ACT_SHOT TC_ACT_SHOT
#define PACKET_KEY 0

#define STATE_NULL 0
#define STATE_NEWLINK 1
#define STATE_REGISTER_PEER_DEVICE 2

#define RPC_CMD 0xdeadc001

#define FSTYPE_LEN 16

#define SYSCALL_ENCODING_TABLE_SIZE 64 // 64 * 8 = 512 > 450, bytes should be enough to hold all 450 syscalls

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

#ifndef VALID_OPEN_FLAGS
#define VALID_OPEN_FLAGS \
        (O_RDONLY | O_WRONLY | O_RDWR | O_CREAT | O_EXCL | O_NOCTTY | O_TRUNC | \
         O_APPEND | O_NDELAY | O_NONBLOCK | __O_SYNC | O_DSYNC | \
         FASYNC | O_DIRECT | O_LARGEFILE | O_DIRECTORY | O_NOFOLLOW | \
         O_NOATIME | O_CLOEXEC | O_PATH | __O_TMPFILE)
#endif

__attribute__((always_inline)) u64 is_cgroup_activity_dumps_enabled() {
    u64 cgroup_activity_dumps_enabled;
    LOAD_CONSTANT("cgroup_activity_dumps_enabled", cgroup_activity_dumps_enabled);
    return cgroup_activity_dumps_enabled != 0;
}

#define CGROUP_DEFAULT  1
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
    return (u32) netns;
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

#endif
