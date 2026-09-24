#ifndef _STRUCTS_PROCESS_H_
#define _STRUCTS_PROCESS_H_

#include "constants/custom.h"
#include "events_context.h"
#include "dentry_resolver.h"

struct process_entry_t {
    struct file_t executable;

    u64 exec_timestamp;
    char tty_name[TTY_NAME_LEN];
    char comm[TASK_COMM_LEN];
};

struct proc_cache_t {
    struct cgroup_context_t cgroup;
    struct process_entry_t entry;
};

struct credentials_t {
    u32 uid;
    u32 gid;
    u32 euid;
    u32 egid;
    u32 fsuid;
    u32 fsgid;
    u32 auid;
    u32 is_auid_set;
    u64 cap_effective;
    u64 cap_permitted;
};

struct pid_cache_t {
    u64 cookie;
    u64 fork_timestamp;
    u64 exit_timestamp;
    u64 user_session_id;
    u64 fork_flags;
    u32 sid;
    u32 padding_sid;
    struct credentials_t credentials;
};

struct args_envs_t {
    u64 id;
    u32 count; // argc/envc retrieved from the kernel
    u32 counter; // counter incremented while parsing args/envs
    u8 truncated;
};

struct args_envs_parsing_context_t {
    const char *args_start;
    u64 envs_offset;
    u64 parsing_offset;
    u32 args_count;
};

// linux_binprm_t contains content from the linux_binprm struct, which holds the arguments used for loading binaries
// We only need enough information from the executable field to be able to resolve the dentry.
struct linux_binprm_t {
    struct path_key_t interpreter;
};

struct str_array_buffer_t {
    char value[MAX_STR_BUFF_LEN];
};

union selinux_write_payload_t {
    // 1 for true, 0 for false, -1 (max) for error
    u32 bool_value;
    struct {
        u16 disable_value;
        u16 enforce_value;
    } status;
};

// Debug aid for exec events that reach userspace with an all-zero file path_key.
// handle_exec_event() is the only writer of syscall->exec.file.path_key, so the stamp it
// leaves behind answers the two questions the event itself cannot: did that hook run for
// this exec at all, and did it see the *same* syscall cache entry that send_exec_event()
// later popped. A differing ctx_id means the two hooks worked on different entries, which
// would explain both the 0/0 key and the foreign execve pathname the syscall context
// reports. See exec_dentry_open_stamp / exec_zero_key_diag.
struct exec_open_stamp_t {
    u64 pid_tgid;
    u32 ctx_id;
    u32 padding;
};

struct exec_zero_key_diag_t {
    u64 open_pid_tgid; // task handle_exec_event() ran on; 0 when it never ran for this tgid
    u64 send_pid_tgid; // task send_exec_event() is running on
    u32 open_ctx_id;   // ctx_id on the entry handle_exec_event() populated
    u32 send_ctx_id;   // ctx_id on the entry send_exec_event() popped
};

#endif
