#ifndef _DEFS_H_
#define _DEFS_H_

#include "../../../ebpf/c/bpf_helpers.h"

#if USE_SYSCALL_WRAPPER == 1
  #define SYSCALL_PREFIX "__x64_sys_"
  #define SYSCALL_KPROBE(syscall) SEC("kprobe/" SYSCALL_PREFIX #syscall) int kprobe__sys_##syscall(struct pt_regs *ctx)
  #define SYSCALL_KRETPROBE(syscall) SEC("kretprobe/" SYSCALL_PREFIX #syscall) int kretprobe__sys_##syscall(struct pt_regs *ctx)
#else
  #define SYSCALL_PREFIX "SyS_"
  #define SYSCALL_KPROBE(syscall) SEC("kprobe/" SYSCALL_PREFIX #syscall) int kprobe__sys_##syscall(struct pt_regs *ctx)
  #define SYSCALL_KRETPROBE(syscall) SEC("kretprobe/" SYSCALL_PREFIX #syscall) int kretprobe__sys_##syscall(struct pt_regs *ctx)
#endif

#define TTY_NAME_LEN 64

#define bpf_printk(fmt, ...)                       \
	({                                             \
		char ____fmt[] = fmt;                      \
		bpf_trace_printk(____fmt, sizeof(____fmt), \
						 ##__VA_ARGS__);           \
	})

#define IS_UNHANDLED_ERROR(retval) retval < 0 && retval != -EACCES && retval != -EPERM

enum event_type
{
    EVENT_OPEN = 1,
    EVENT_MKDIR,
    EVENT_LINK,
    EVENT_RENAME,
    EVENT_UNLINK,
    EVENT_RMDIR,
    EVENT_CHMOD,
    EVENT_CHOWN,
    EVENT_UTIME,
    EVENT_MOUNT,
    EVENT_UMOUNT,
    EVENT_EXEC,
};

struct event_t {
    u64 type;
    u64 timestamp;
    s64 retval;
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
    u32  numlower;
    u32  padding;
};

#define CONTAINER_ID_LEN 64

struct bpf_map_def SEC("maps/events") events = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .key_size = sizeof(__u32),
    .value_size = sizeof(__u32),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

#define send_event(ctx, event) \
    bpf_perf_event_output(ctx, &events, bpf_get_smp_processor_id(), &event, sizeof(event))

struct bpf_map_def SEC("maps/mountpoints_events") mountpoints_events = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .key_size = sizeof(__u32),
    .value_size = sizeof(__u32),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

#define send_mountpoints_events(ctx, event) \
    bpf_perf_event_output(ctx, &mountpoints_events, bpf_get_smp_processor_id(), &event, sizeof(event))

static __attribute__((always_inline)) u32 get_character_value(u8 c, u32 base_multiplier) {
    if (c >= 49 && c <= 57) {
        return (c - 48) * base_multiplier;
    }
    return 0;
}

#define CHAR_TO_UINT32_BASE_10_MAX_LEN 11

static __attribute__((always_inline)) u32 char_to_uint32_base_10(char *buff) {
    u32 res = 0;
    int base_multiplier = 1;
    u8 c = 0;
    char buffer[CHAR_TO_UINT32_BASE_10_MAX_LEN];

    int size = bpf_probe_read_str(&buffer, sizeof(buffer), buff);
    if (size <= 1) {
        return 0;
    }
    u32 cursor = size - 2;

#pragma unroll
    for (int i = 1; i < CHAR_TO_UINT32_BASE_10_MAX_LEN; i++)
    {
        if (cursor < 0) {
            return res;
        }
        bpf_probe_read(&c, sizeof(c), buffer + cursor);
        res += get_character_value(c, base_multiplier);
        base_multiplier = base_multiplier * 10;
        cursor--;
    }

    return res;
}

static __attribute__((always_inline)) u32 copy_container_id(char dst[CONTAINER_ID_LEN], char src[CONTAINER_ID_LEN]) {
    if (src[0] == 0) {
        return 0;
    }

#pragma unroll
    for (int i = 0; i < CONTAINER_ID_LEN; i++)
    {
        dst[i] = src[i];
    }
    return CONTAINER_ID_LEN;
}

#endif
