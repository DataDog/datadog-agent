#ifndef _STRUCTS_TRACEPOINTS_H_
#define _STRUCTS_TRACEPOINTS_H_

struct tracepoint_raw_syscalls_sys_exit_t {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    long id;
    long ret;
};

struct tracepoint_syscalls_sys_exit_t {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    int __syscall_ret;
    long ret;
};

struct _tracepoint_sched_process_fork {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    char parent_comm[16];
    pid_t parent_pid;
    char child_comm[16];
    pid_t child_pid;
};

struct _tracepoint_sched_process_exec {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    int data_loc_filename;
    pid_t pid;
    pid_t old_pid;
};

struct tracepoint_syscalls_sys_exit_mmap_t {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    int __syscall_nr;
    long ret;
};

struct tracepoint_io_uring_io_uring_create_t {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    int fd;
    void *ctx;
    u32 sq_entries;
    u32 cq_entries;
    u32 flags;
};

struct _tracepoint_raw_syscalls_sys_enter {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;
    long id;
    unsigned long args[6];
};

struct tracepoint_module_module_load_t {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    unsigned int taints;
    int data_loc_modname;
};

#endif
