#ifndef _DEFS_H
#define _DEFS_H

#include "../../ebpf/c/bpf_helpers.h"

#ifdef CONFIG_ARCH_HAS_SYSCALL_WRAPPER
  #define SYSCALL_PREFIX "__x64_sys_"
  #define SYSCALL_KPROBE(syscall) SEC("kprobe/" SYSCALL_PREFIX #syscall) int kprobe__sys_##syscall(struct pt_regs *ctx)
  #define SYSCALL_KRETPROBE(syscall) SEC("kretprobe/" SYSCALL_PREFIX #syscall) int kretprobe__sys_##syscall(struct pt_regs *ctx)
#else
  #define SYSCALL_PREFIX "SyS_"
  #define SYSCALL_KPROBE(syscall) SEC("kprobe/" SYSCALL_PREFIX #syscall) int kprobe__sys_##syscall(struct pt_regs *ctx)
  #define SYSCALL_KRETPROBE(syscall) SEC("kretprobe/" SYSCALL_PREFIX #syscall) int kretprobe__sys_##syscall(struct pt_regs *ctx)
#endif

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
    EVENT_VFS_SETATTR,
    EVENT_VFS_UNLINK,
    EVENT_VFS_RMDIR,
};

struct event_t {
    u64 type;
    u64 timestamp;
    u64 retval;
};

struct event_context_t {
    char comm[TASK_COMM_LEN];
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

struct process_discriminator_t {
    char comm[TASK_COMM_LEN];
};

struct bpf_map_def SEC("maps/process_discriminators") process_discriminators = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(struct process_discriminator_t),
    .value_size = sizeof(u8),
    .max_entries = 256,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/events") events = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .key_size = sizeof(__u32),
    .value_size = sizeof(__u32),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

void __attribute__((always_inline)) fill_event_context(struct event_context_t *event_context) {
    bpf_get_current_comm(&event_context->comm, sizeof(event_context->comm));
}

static __attribute__((always_inline)) int filter(struct event_context_t *event_context) {
    int found = bpf_map_lookup_elem(&process_discriminators, &event_context->comm) != 0;
    if (found) {
        printk("Process filter found for %s\n", event_context->comm);
    }
    return !found;
}

int __attribute__((always_inline)) filter_process() {
    struct event_context_t event_context;
    fill_event_context(&event_context);
    return !filter(&event_context);
}

#define send_event(ctx, event) \
    bpf_perf_event_output(ctx, &events, bpf_get_smp_processor_id(), &event, sizeof(event))

#endif