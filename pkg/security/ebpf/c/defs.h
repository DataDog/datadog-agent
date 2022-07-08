#ifndef _DEFS_H_
#define _DEFS_H_

#include "bpf_helpers.h"

#include "constants.h"

#if defined(__x86_64__)
  #define SYSCALL64_PREFIX "__x64_"
  #define SYSCALL32_PREFIX "__ia32_"

  #define SYSCALL64_PT_REGS_PARM1(x) ((x)->di)
  #define SYSCALL64_PT_REGS_PARM2(x) ((x)->si)
  #define SYSCALL64_PT_REGS_PARM3(x) ((x)->dx)
  #if USE_SYSCALL_WRAPPER == 1
   #define SYSCALL64_PT_REGS_PARM4(x) ((x)->r10)
  #else
  #define SYSCALL64_PT_REGS_PARM4(x) ((x)->cx)
  #endif
  #define SYSCALL64_PT_REGS_PARM5(x) ((x)->r8)
  #define SYSCALL64_PT_REGS_PARM6(x) ((x)->r9)

  #define SYSCALL32_PT_REGS_PARM1(x) ((x)->bx)
  #define SYSCALL32_PT_REGS_PARM2(x) ((x)->cx)
  #define SYSCALL32_PT_REGS_PARM3(x) ((x)->dx)
  #define SYSCALL32_PT_REGS_PARM4(x) ((x)->si)
  #define SYSCALL32_PT_REGS_PARM5(x) ((x)->di)
  #define SYSCALL32_PT_REGS_PARM6(x) ((x)->bp)

#elif defined(__aarch64__)
  #define SYSCALL64_PREFIX "__arm64_"
  #define SYSCALL32_PREFIX "__arm32_"

  #define SYSCALL64_PT_REGS_PARM1(x) PT_REGS_PARM1(x)
  #define SYSCALL64_PT_REGS_PARM2(x) PT_REGS_PARM2(x)
  #define SYSCALL64_PT_REGS_PARM3(x) PT_REGS_PARM3(x)
  #define SYSCALL64_PT_REGS_PARM4(x) PT_REGS_PARM4(x)
  #define SYSCALL64_PT_REGS_PARM5(x) PT_REGS_PARM5(x)
  #define SYSCALL64_PT_REGS_PARM6(x) PT_REGS_PARM6(x)

  #define SYSCALL32_PT_REGS_PARM1(x) PT_REGS_PARM1(x)
  #define SYSCALL32_PT_REGS_PARM2(x) PT_REGS_PARM2(x)
  #define SYSCALL32_PT_REGS_PARM3(x) PT_REGS_PARM3(x)
  #define SYSCALL32_PT_REGS_PARM4(x) PT_REGS_PARM4(x)
  #define SYSCALL32_PT_REGS_PARM5(x) PT_REGS_PARM5(x)
  #define SYSCALL32_PT_REGS_PARM6(x) PT_REGS_PARM6(x)

#else
  #error "Unsupported platform"
#endif

/*
 * __MAP - apply a macro to syscall arguments
 * __MAP(n, m, t1, a1, t2, a2, ..., tn, an) will expand to
 *    m(t1, a1), m(t2, a2), ..., m(tn, an)
 * The first argument must be equal to the amount of type/name
 * pairs given.  Note that this list of pairs (i.e. the arguments
 * of __MAP starting at the third one) is in the same format as
 * for SYSCALL_DEFINE<n>/COMPAT_SYSCALL_DEFINE<n>
 */
#define __JOIN0(m,...)
#define __JOIN1(m,t,a,...) ,m(t,a)
#define __JOIN2(m,t,a,...) ,m(t,a) __JOIN1(m,__VA_ARGS__)
#define __JOIN3(m,t,a,...) ,m(t,a) __JOIN2(m,__VA_ARGS__)
#define __JOIN4(m,t,a,...) ,m(t,a) __JOIN3(m,__VA_ARGS__)
#define __JOIN5(m,t,a,...) ,m(t,a) __JOIN4(m,__VA_ARGS__)
#define __JOIN6(m,t,a,...) ,m(t,a) __JOIN5(m,__VA_ARGS__)
#define __JOIN(n,...) __JOIN##n(__VA_ARGS__)

#define __MAP0(n,m,...)
#define __MAP1(n,m,t1,a1,...) m(1,t1,a1)
#define __MAP2(n,m,t1,a1,t2,a2) m(1,t1,a1) m(2,t2,a2)
#define __MAP3(n,m,t1,a1,t2,a2,t3,a3) m(1,t1,a1) m(2,t2,a2) m(3,t3,a3)
#define __MAP4(n,m,t1,a1,t2,a2,t3,a3,t4,a4) m(1,t1,a1) m(2,t2,a2) m(3,t3,a3) m(4,t4,a4)
#define __MAP5(n,m,t1,a1,t2,a2,t3,a3,t4,a4,t5,a5) m(1,t1,a1) m(2,t2,a2) m(3,t3,a3) m(4,t4,a4) m(5,t5,a5)
#define __MAP6(n,m,t1,a1,t2,a2,t3,a3,t4,a4,t5,a5,t6,a6) m(1,t1,a1) m(2,t2,a2) m(3,t3,a3) m(4,t4,a4) m(5,t5,a5) m(6,t6,a6)
#define __MAP(n,...) __MAP##n(n,__VA_ARGS__)

#define __SC_DECL(t, a) t a
#define __SC_PASS(t, a) a

#define SYSCALL_ABI_HOOKx(x,word_size,type,TYPE,prefix,syscall,suffix,...) \
    int __attribute__((always_inline)) type##__##sys##syscall(struct pt_regs *ctx __JOIN(x,__SC_DECL,__VA_ARGS__)); \
    SEC(#type "/" SYSCALL##word_size##_PREFIX #prefix SYSCALL_PREFIX #syscall #suffix) \
    int type##__ ##word_size##_##prefix ##sys##syscall##suffix(struct pt_regs *ctx) { \
        SYSCALL_##TYPE##_PROLOG(x,__SC_##word_size##_PARAM,syscall,__VA_ARGS__) \
        return type##__sys##syscall(ctx __JOIN(x,__SC_PASS,__VA_ARGS__)); \
    }

#define SYSCALL_HOOK_COMMON(x,type,syscall,...) int __attribute__((always_inline)) type##__sys##syscall(struct pt_regs *ctx __JOIN(x,__SC_DECL,__VA_ARGS__))

#if USE_SYSCALL_WRAPPER == 1
  #define SYSCALL_PREFIX "sys"
  #define __SC_64_PARAM(n, t, a) t a; bpf_probe_read(&a, sizeof(t), (void*) &SYSCALL64_PT_REGS_PARM##n(rctx));
  #define __SC_32_PARAM(n, t, a) t a; bpf_probe_read(&a, sizeof(t), (void*) &SYSCALL32_PT_REGS_PARM##n(rctx));
  #define SYSCALL_KPROBE_PROLOG(x,m,syscall,...) \
    struct pt_regs *rctx = (struct pt_regs *) PT_REGS_PARM1(ctx); \
    if (!rctx) return 0; \
    __MAP(x,m,__VA_ARGS__)
  #define SYSCALL_KRETPROBE_PROLOG(...)
  #define SYSCALL_HOOKx(x,type,TYPE,prefix,name,...) \
    SYSCALL_ABI_HOOKx(x,32,type,TYPE,prefix,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_HOOK_COMMON(x,type,name,__VA_ARGS__)
  #define SYSCALL_COMPAT_HOOKx(x,type,TYPE,name,...) \
    SYSCALL_ABI_HOOKx(x,32,type,TYPE,compat_,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_HOOK_COMMON(x,type,name,__VA_ARGS__)
  #define SYSCALL_COMPAT_TIME_HOOKx(x,type,TYPE,name,...) \
    SYSCALL_ABI_HOOKx(x,32,type,TYPE,compat_,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,32,type,TYPE,,name,_time32,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,_time32,__VA_ARGS__) \
    SYSCALL_HOOK_COMMON(x,type,name,__VA_ARGS__)
#else
  #undef SYSCALL32_PREFIX
  #undef SYSCALL64_PREFIX
  #define SYSCALL32_PREFIX ""
  #define SYSCALL64_PREFIX ""
  #define SYSCALL_PREFIX "sys"
  #define __SC_64_PARAM(n, t, a) t a = (t) SYSCALL64_PT_REGS_PARM##n(ctx);
  #define __SC_32_PARAM(n, t, a) t a = (t) SYSCALL32_PT_REGS_PARM##n(ctx);
  #define SYSCALL_KPROBE_PROLOG(x,m,syscall,...) \
    struct pt_regs *rctx = ctx; \
    if (!rctx) return 0; \
    __MAP(x,m,__VA_ARGS__)
  #define SYSCALL_KRETPROBE_PROLOG(...)
  #define SYSCALL_HOOKx(x,type,TYPE,prefix,name,...) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,compat_,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_HOOK_COMMON(x,type,name,__VA_ARGS__)
  #define SYSCALL_COMPAT_HOOKx(x,type,TYPE,name,...) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,compat_,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_HOOK_COMMON(x,type,name,__VA_ARGS__)
  #define SYSCALL_COMPAT_TIME_HOOKx(x,type,TYPE,name,...) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,compat_,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_HOOK_COMMON(x,type,name,__VA_ARGS__)
#endif

#define SYSCALL_KPROBE0(name, ...) SYSCALL_HOOKx(0,kprobe,KPROBE,,_##name,__VA_ARGS__)
#define SYSCALL_KPROBE1(name, ...) SYSCALL_HOOKx(1,kprobe,KPROBE,,_##name,__VA_ARGS__)
#define SYSCALL_KPROBE2(name, ...) SYSCALL_HOOKx(2,kprobe,KPROBE,,_##name,__VA_ARGS__)
#define SYSCALL_KPROBE3(name, ...) SYSCALL_HOOKx(3,kprobe,KPROBE,,_##name,__VA_ARGS__)
#define SYSCALL_KPROBE4(name, ...) SYSCALL_HOOKx(4,kprobe,KPROBE,,_##name,__VA_ARGS__)
#define SYSCALL_KPROBE5(name, ...) SYSCALL_HOOKx(5,kprobe,KPROBE,,_##name,__VA_ARGS__)
#define SYSCALL_KPROBE6(name, ...) SYSCALL_HOOKx(6,kprobe,KPROBE,,_##name,__VA_ARGS__)

#define SYSCALL_KRETPROBE(name, ...) SYSCALL_HOOKx(0,kretprobe,KRETPROBE,,_##name)

#define SYSCALL_COMPAT_KPROBE0(name, ...) SYSCALL_COMPAT_HOOKx(0,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE1(name, ...) SYSCALL_COMPAT_HOOKx(1,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE2(name, ...) SYSCALL_COMPAT_HOOKx(2,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE3(name, ...) SYSCALL_COMPAT_HOOKx(3,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE4(name, ...) SYSCALL_COMPAT_HOOKx(4,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE5(name, ...) SYSCALL_COMPAT_HOOKx(5,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE6(name, ...) SYSCALL_COMPAT_HOOKx(6,kprobe,KPROBE,_##name,__VA_ARGS__)

#define SYSCALL_COMPAT_KRETPROBE(name, ...) SYSCALL_COMPAT_HOOKx(0,kretprobe,KRETPROBE,_##name)

#define SYSCALL_COMPAT_TIME_KPROBE0(name, ...) SYSCALL_COMPAT_TIME_HOOKx(0,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE1(name, ...) SYSCALL_COMPAT_TIME_HOOKx(1,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE2(name, ...) SYSCALL_COMPAT_TIME_HOOKx(2,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE3(name, ...) SYSCALL_COMPAT_TIME_HOOKx(3,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE4(name, ...) SYSCALL_COMPAT_TIME_HOOKx(4,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE5(name, ...) SYSCALL_COMPAT_TIME_HOOKx(5,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE6(name, ...) SYSCALL_COMPAT_TIME_HOOKx(6,kprobe,KPROBE,_##name,__VA_ARGS__)

#define SYSCALL_COMPAT_TIME_KRETPROBE(name, ...) SYSCALL_COMPAT_TIME_HOOKx(0,kretprobe,KRETPROBE,_##name)

#define TTY_NAME_LEN 64
#define CONTAINER_ID_LEN 64
#define MAX_XATTR_NAME_LEN 200

#define bpf_printk(fmt, ...)                       \
	({                                             \
		char ____fmt[] = fmt;                      \
		bpf_trace_printk(____fmt, sizeof(____fmt), \
						 ##__VA_ARGS__);           \
	})

#define IS_UNHANDLED_ERROR(retval) retval < 0 && retval != -EACCES && retval != -EPERM
#define IS_ERR(ptr)     ((unsigned long)(ptr) > (unsigned long)(-1000))

#define IS_KTHREAD(ppid, pid) ppid == 2 || pid == 2

enum event_type
{
    EVENT_ANY = 0,
    EVENT_FIRST_DISCARDER = 1,
    EVENT_OPEN = EVENT_FIRST_DISCARDER,
    EVENT_MKDIR,
    EVENT_LINK,
    EVENT_RENAME,
    EVENT_UNLINK,
    EVENT_RMDIR,
    EVENT_CHMOD,
    EVENT_CHOWN,
    EVENT_UTIME,
    EVENT_SETXATTR,
    EVENT_REMOVEXATTR,
    EVENT_LAST_DISCARDER = EVENT_REMOVEXATTR,

    EVENT_MOUNT,
    EVENT_UMOUNT,
    EVENT_FORK,
    EVENT_EXEC,
    EVENT_EXIT,
    EVENT_INVALIDATE_DENTRY,
    EVENT_SETUID,
    EVENT_SETGID,
    EVENT_CAPSET,
    EVENT_ARGS_ENVS,
    EVENT_MOUNT_RELEASED,
    EVENT_SELINUX,
    EVENT_BPF,
    EVENT_PTRACE,
    EVENT_MMAP,
    EVENT_MPROTECT,
    EVENT_INIT_MODULE,
    EVENT_DELETE_MODULE,
    EVENT_SIGNAL,
    EVENT_SPLICE,
    EVENT_CGROUP_TRACING,
    EVENT_DNS,
    EVENT_NET_DEVICE,
    EVENT_VETH_PAIR,
    EVENT_BIND,
    EVENT_MAX, // has to be the last one

    EVENT_ALL = 0xffffffff // used as a mask for all the events
};

struct kevent_t {
    u64 cpu;
    u64 timestamp;
    u32 type;
    u8 async;
    u8 padding[3];
};

struct syscall_t {
    s64 retval;
};

struct span_context_t {
   u64 span_id;
   u64 trace_id;
};

struct process_context_t {
    u32 pid;
    u32 tid;
    u32 netns;
    u32 padding;
};

struct container_context_t {
    char container_id[CONTAINER_ID_LEN];
};

enum file_flags {
    LOWER_LAYER = 1 << 0,
    UPPER_LAYER = 1 << 1,
};

struct path_key_t {
    u64 ino;
    u32 mount_id;
    u32 path_id;
};

struct ktimeval {
    long tv_sec;
    long tv_nsec;
};

struct file_metadata_t {
    u32 uid;
    u32 gid;
    u32 nlink;
    u16 mode;
    char padding[2];

    struct ktimeval ctime;
    struct ktimeval mtime;
};

struct file_t {
    struct path_key_t path_key;
    u32 flags;
    u32 padding;
    struct file_metadata_t metadata;
};

struct tracepoint_raw_syscalls_sys_exit_t
{
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

struct bpf_map_def SEC("maps/path_id") path_id = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

static __attribute__((always_inline)) u32 get_path_id(int invalidate) {
    u32 key = 0;

    u32 *prev_id = bpf_map_lookup_elem(&path_id, &key);
    if (!prev_id) {
        return 0;
    }

    u32 id = *prev_id;

    // need to invalidate the current path id for event which may change the association inode/name like
    // unlink, rename, rmdir.
    if (invalidate) {
        __sync_fetch_and_add(prev_id, 1);
    }

    return id;
}

struct bpf_map_def SEC("maps/flushing_discarders") flushing_discarders = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

static __attribute__((always_inline)) u32 is_flushing_discarders(void) {
    u32 key = 0;
    u32 *prev_id = bpf_map_lookup_elem(&flushing_discarders, &key);
    return prev_id != NULL && *prev_id;
}

struct perf_map_stats_t {
    u64 bytes;
    u64 count;
    u64 lost;
};

struct bpf_map_def SEC("maps/events") events = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .max_entries = 0,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/events_stats") events_stats = {
    .type = BPF_MAP_TYPE_PERCPU_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct perf_map_stats_t),
    .max_entries = EVENT_MAX,
    .pinning = 0,
    .namespace = "",
};

#define send_event_with_size_ptr_perf(ctx, event_type, kernel_event, kernel_event_size)                                     \
    kernel_event->event.type = event_type;                                                                             \
    kernel_event->event.cpu = bpf_get_smp_processor_id();                                                              \
    kernel_event->event.timestamp = bpf_ktime_get_ns();                                                                \
                                                                                                                       \
    int perf_ret = bpf_perf_event_output(ctx, &events, kernel_event->event.cpu, kernel_event, kernel_event_size);      \
                                                                                                                       \
    if (kernel_event->event.type < EVENT_MAX) {                                                                        \
        u64 lookup_type = event_type;                                                                                  \
        struct perf_map_stats_t *stats = bpf_map_lookup_elem(&events_stats, &lookup_type);                             \
        if (stats != NULL) {                                                                                           \
            if (!perf_ret) {                                                                                           \
                __sync_fetch_and_add(&stats->bytes, kernel_event_size + 4);                                            \
                __sync_fetch_and_add(&stats->count, 1);                                                                \
            } else {                                                                                                   \
                __sync_fetch_and_add(&stats->lost, 1);                                                                 \
            }                                                                                                          \
        }                                                                                                              \
    }                                                                                                                  \

#define send_event_with_size_ptr_ringbuf(ctx, event_type, kernel_event, kernel_event_size)                                     \
    kernel_event->event.type = event_type;                                                                             \
    kernel_event->event.cpu = bpf_get_smp_processor_id();                                                              \
    kernel_event->event.timestamp = bpf_ktime_get_ns();                                                                \
                                                                                                                       \
    int perf_ret = bpf_ringbuf_output(&events, kernel_event, kernel_event_size, 0);                                    \
                                                                                                                       \
    if (kernel_event->event.type < EVENT_MAX) {                                                                        \
        u64 lookup_type = event_type;                                                                                  \
        struct perf_map_stats_t *stats = bpf_map_lookup_elem(&events_stats, &lookup_type);                             \
        if (stats != NULL) {                                                                                           \
            if (!perf_ret) {                                                                                           \
                __sync_fetch_and_add(&stats->bytes, kernel_event_size + 4);                                            \
                __sync_fetch_and_add(&stats->count, 1);                                                                \
            } else {                                                                                                   \
                __sync_fetch_and_add(&stats->lost, 1);                                                                 \
            }                                                                                                          \
        }                                                                                                              \
    }                                                                                                                  \

#define send_event_with_size_perf(ctx, event_type, kernel_event, kernel_event_size)                                         \
    kernel_event.event.type = event_type;                                                                              \
    kernel_event.event.cpu = bpf_get_smp_processor_id();                                                               \
    kernel_event.event.timestamp = bpf_ktime_get_ns();                                                                 \
                                                                                                                       \
    int perf_ret = bpf_perf_event_output(ctx, &events, kernel_event.event.cpu, &kernel_event, kernel_event_size);      \
                                                                                                                       \
    if (kernel_event.event.type < EVENT_MAX) {                                                                         \
        struct perf_map_stats_t *stats = bpf_map_lookup_elem(&events_stats, &kernel_event.event.type);                 \
        if (stats != NULL) {                                                                                           \
            if (!perf_ret) {                                                                                           \
                __sync_fetch_and_add(&stats->bytes, kernel_event_size + 4);                                            \
                __sync_fetch_and_add(&stats->count, 1);                                                                \
            } else {                                                                                                   \
                __sync_fetch_and_add(&stats->lost, 1);                                                                 \
            }                                                                                                          \
        }                                                                                                              \
    }                                                                                                                  \

#define send_event_with_size_ringbuf(ctx, event_type, kernel_event, kernel_event_size)                                         \
    kernel_event.event.type = event_type;                                                                              \
    kernel_event.event.cpu = bpf_get_smp_processor_id();                                                               \
    kernel_event.event.timestamp = bpf_ktime_get_ns();                                                                 \
                                                                                                                       \
    int perf_ret = bpf_ringbuf_output(&events, &kernel_event, kernel_event_size, 0);                                   \
                                                                                                                       \
    if (kernel_event.event.type < EVENT_MAX) {                                                                         \
        struct perf_map_stats_t *stats = bpf_map_lookup_elem(&events_stats, &kernel_event.event.type);                 \
        if (stats != NULL) {                                                                                           \
            if (!perf_ret) {                                                                                           \
                __sync_fetch_and_add(&stats->bytes, kernel_event_size + 4);                                            \
                __sync_fetch_and_add(&stats->count, 1);                                                                \
            } else {                                                                                                   \
                __sync_fetch_and_add(&stats->lost, 1);                                                                 \
            }                                                                                                          \
        }                                                                                                              \
    }                                                                                                                  \

#define send_event(ctx, event_type, kernel_event)                                                                      \
    u64 size = sizeof(kernel_event);                                                                                   \
    u64 use_ring_buffer;                                                                                               \
    LOAD_CONSTANT("use_ring_buffer", use_ring_buffer);                                                                 \
    if (use_ring_buffer) {                                                                                             \
        send_event_with_size_ringbuf(ctx, event_type, kernel_event, size)                                              \
    } else {                                                                                                           \
        send_event_with_size_perf(ctx, event_type, kernel_event, size)                                                 \
    }                                                                                                                  \

#define send_event_ptr(ctx, event_type, kernel_event)                                                                  \
    u64 size = sizeof(*kernel_event);                                                                                  \
    u64 use_ring_buffer;                                                                                               \
    LOAD_CONSTANT("use_ring_buffer", use_ring_buffer);                                                                 \
    if (use_ring_buffer) {                                                                                             \
        send_event_with_size_ptr_ringbuf(ctx, event_type, kernel_event, size)                                          \
    } else {                                                                                                           \
        send_event_with_size_ptr_perf(ctx, event_type, kernel_event, size)                                             \
    }                                                                                                                  \

#define send_event_with_size_ptr(ctx, event_type, kernel_event, size)                                                  \
    u64 use_ring_buffer;                                                                                               \
    LOAD_CONSTANT("use_ring_buffer", use_ring_buffer);                                                                 \
    if (use_ring_buffer) {                                                                                             \
        send_event_with_size_ptr_ringbuf(ctx, event_type, kernel_event, size)                                          \
    } else {                                                                                                           \
        send_event_with_size_ptr_perf(ctx, event_type, kernel_event, size)                                             \
    }                                                                                                                  \

// implemented in the discarder.h file
int __attribute__((always_inline)) bump_discarder_revision(u32 mount_id);

struct mount_released_event_t {
    struct kevent_t event;
    u32 mount_id;
    u32 discarder_revision;
};

struct mount_ref_t {
    u32 umounted;
    s32 counter;
};

struct bpf_map_def SEC("maps/mount_ref") mount_ref = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct mount_ref_t),
    .max_entries = 64000,
    .pinning = 0,
    .namespace = "",
};

static __attribute__((always_inline)) void inc_mount_ref(u32 mount_id) {
    u32 key = mount_id;
    struct mount_ref_t zero = {};

    bpf_map_update_elem(&mount_ref, &key, &zero, BPF_NOEXIST);
    struct mount_ref_t *ref = bpf_map_lookup_elem(&mount_ref, &key);
    if (ref) {
        __sync_fetch_and_add(&ref->counter, 1);
    }
}

static __attribute__((always_inline)) void dec_mount_ref(struct pt_regs *ctx, u32 mount_id) {
    u32 key = mount_id;
    struct mount_ref_t *ref = bpf_map_lookup_elem(&mount_ref, &key);
    if (ref) {
        __sync_fetch_and_add(&ref->counter, -1);
        if (ref->counter > 0 || !ref->umounted) {
            return;
        }
        bpf_map_delete_elem(&mount_ref, &key);
    } else {
        return;
    }

    struct mount_released_event_t event = {
        .mount_id = mount_id,
        .discarder_revision = bump_discarder_revision(mount_id),
    };

    send_event(ctx, EVENT_MOUNT_RELEASED, event);
}

static __attribute__((always_inline)) void umounted(struct pt_regs *ctx, u32 mount_id) {
    u32 key = mount_id;
    struct mount_ref_t *ref = bpf_map_lookup_elem(&mount_ref, &key);
    if (ref) {
        if (ref->counter <= 0) {
            bpf_map_delete_elem(&mount_ref, &key);
        } else {
            ref->umounted = 1;
            return;
        }
    }

    struct mount_released_event_t event = {
        .mount_id = mount_id,
        .discarder_revision = bump_discarder_revision(mount_id),
    };
    send_event(ctx, EVENT_MOUNT_RELEASED, event);
}

static __attribute__((always_inline)) u32 ord(u8 c) {
    if (c >= 49 && c <= 57) {
        return c - 48;
    }
    return 0;
}

#define CHAR_TO_UINT32_BASE_10_MAX_LEN 11

static __attribute__((always_inline)) u32 atoi(char *buff) {
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
        res += ord(c) * base_multiplier;
        base_multiplier = base_multiplier * 10;
        cursor--;
    }

    return res;
}

// implemented in the probe.c file
void __attribute__((always_inline)) invalidate_inode(struct pt_regs *ctx, u32 mount_id, u64 inode, int send_invalidate_event);

struct bpf_map_def SEC("maps/enabled_events") enabled_events = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u64),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

static __attribute__((always_inline)) u64 get_enabled_events(void) {
    u32 key = 0;
    u64 *mask = bpf_map_lookup_elem(&enabled_events, &key);
    if (mask) {
        return *mask;
    }
    return 0;
}

static __attribute__((always_inline)) int mask_has_event(u64 mask, enum event_type event) {
    return mask & (1 << (event-EVENT_FIRST_DISCARDER));
}

static __attribute__((always_inline)) int is_event_enabled(enum event_type event) {
    return mask_has_event(get_enabled_events(), event);
}

static __attribute__((always_inline)) void add_event_to_mask(u64 *mask, enum event_type event) {
    if (event == EVENT_ALL) {
        *mask = event;
    } else {
        *mask |= 1 << (event - EVENT_FIRST_DISCARDER);
    }
}

#define VFS_ARG_POSITION1 1
#define VFS_ARG_POSITION2 2
#define VFS_ARG_POSITION3 3
#define VFS_ARG_POSITION4 4
#define VFS_ARG_POSITION5 5
#define VFS_ARG_POSITION6 6

static __attribute__((always_inline)) u64 get_vfs_unlink_dentry_position() {
    u64 vfs_unlink_dentry_position;
    LOAD_CONSTANT("vfs_unlink_dentry_position", vfs_unlink_dentry_position);
    return vfs_unlink_dentry_position;
}

static __attribute__((always_inline)) u64 get_vfs_mkdir_dentry_position() {
    u64 vfs_mkdir_dentry_position;
    LOAD_CONSTANT("vfs_mkdir_dentry_position", vfs_mkdir_dentry_position);
    return vfs_mkdir_dentry_position;
}

static __attribute__((always_inline)) u64 get_vfs_link_target_dentry_position() {
    u64 vfs_link_target_dentry_position;
    LOAD_CONSTANT("vfs_link_target_dentry_position", vfs_link_target_dentry_position);
    return vfs_link_target_dentry_position;;
}

static __attribute__((always_inline)) u64 get_vfs_setxattr_dentry_position() {
    u64 vfs_setxattr_dentry_position;
    LOAD_CONSTANT("vfs_setxattr_dentry_position", vfs_setxattr_dentry_position);
    return vfs_setxattr_dentry_position;
}

static __attribute__((always_inline)) u64 get_vfs_removexattr_dentry_position() {
    u64 vfs_removexattr_dentry_position;
    LOAD_CONSTANT("vfs_removexattr_dentry_position", vfs_removexattr_dentry_position);
    return vfs_removexattr_dentry_position;
}

#define VFS_RENAME_REGISTER_INPUT 1
#define VFS_RENAME_STRUCT_INPUT   2

static __attribute__((always_inline)) u64 get_vfs_rename_input_type() {
    u64 vfs_rename_input_type;
    LOAD_CONSTANT("vfs_rename_input_type", vfs_rename_input_type);
    return vfs_rename_input_type;
}

static __attribute__((always_inline)) u64 get_vfs_rename_src_dentry_offset() {
    u64 offset;
    LOAD_CONSTANT("vfs_rename_src_dentry_offset", offset);
    return offset ? offset : 16; // offsetof(struct renamedata, old_dentry)
}

static __attribute__((always_inline)) u64 get_vfs_rename_target_dentry_offset() {
    u64 offset;
    LOAD_CONSTANT("vfs_rename_target_dentry_offset", offset);
    return offset ? offset : 40; // offsetof(struct renamedata, new_dentry)
}

struct inode_discarder_t {
    struct path_key_t path_key;
    u32 is_leaf;
    u32 padding;
};

struct is_discarded_by_inode_t {
    u64 event_type;
    struct inode_discarder_t discarder;
    u64 now;
    u32 tgid;
    u32 activity_dump_state;
};

static __attribute__((always_inline))
void *bpf_map_lookup_or_try_init(struct bpf_map_def *map, void *key, void *zero) {
    if (map == NULL) {
        return NULL;
    }

    void *value = bpf_map_lookup_elem(map, key);
    if (value != NULL)
        return value;

    // Use BPF_NOEXIST to prevent race condition
    if (bpf_map_update_elem(map, key, zero, BPF_NOEXIST) < 0)
        return NULL;

    return bpf_map_lookup_elem(map, key);
}
#endif
