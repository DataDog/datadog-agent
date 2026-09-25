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
//
// Three stamps are compared, because each rules out a different story:
//   - exec_entry_stamp, written by trace__sys_execveat under the pid_tgid the entry was
//     cached under, says whether the entry send_exec_event popped is the one this task's
//     execve created. ctx_id identifies the execve, and collect_syscall_ctx assigns ids
//     from 1, so a popped entry carrying ctx_id 0 cannot have come from that path at all.
//   - exec_dentry_open_stamp, written by handle_exec_event, says whether the only writer of
//     exec.file.path_key ran, and against which entry.
//   - the key and dentry as send_exec_event itself sees them, so a key that is fine here but
//     zero in userspace separates a kernel problem from a userspace one.
// The diag is written for every exec, not just the zero-key ones, so that its absence is
// unambiguous rather than being a second thing needing explanation.
// handle_exec_event records the key it resolved as well as its identity, because that key
// belongs to the execve being reported even when send_exec_event later pops a different
// entry -- which makes it the authoritative source rather than just a diagnostic.
struct exec_open_stamp_t {
    u64 pid_tgid;
    u64 ino;
    u32 mount_id;
    u32 path_id;
    u32 ctx_id;
    u32 padding;
};

#define EXEC_DIAG_HAS_DENTRY 1
#define EXEC_DIAG_HAS_OPEN_STAMP 2
#define EXEC_DIAG_HAS_ENTRY_STAMP 4
#define EXEC_DIAG_ROUTE_DIRECT 8        // entry found under the current pid_tgid
#define EXEC_DIAG_ROUTE_IMPERSONATED 16 // entry found via exec_pid_transfer
#define EXEC_DIAG_KEY_REPAIRED 32       // the foreign entry's key was replaced by the stamp's

struct exec_zero_key_diag_t {
    u64 send_pid_tgid;   // task send_exec_event is running on
    u64 open_pid_tgid;   // task handle_exec_event ran on; 0 when it never ran
    u64 entry_ino;       // path_key as send_exec_event sees it, before the event is built
    u64 found_key;       // pid_tgid the popped entry was actually found under
    u32 entry_mount_id;
    u32 send_ctx_id;     // ctx_id on the entry send_exec_event popped
    u32 open_ctx_id;     // ctx_id on the entry handle_exec_event populated
    u32 stamped_ctx_id;  // ctx_id trace__sys_execveat recorded for this pid_tgid
    u32 flags;           // EXEC_DIAG_* above
    u32 padding;
};

#endif
