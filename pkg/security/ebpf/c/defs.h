#ifndef _DEFS_H_
#define _DEFS_H_

#include "../../../ebpf/c/bpf_helpers.h"

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
#define __JOIN1(m,t,a,...) m(t,a)
#define __JOIN2(m,t,a,...) m(t,a), __JOIN1(m,__VA_ARGS__)
#define __JOIN3(m,t,a,...) m(t,a), __JOIN2(m,__VA_ARGS__)
#define __JOIN4(m,t,a,...) m(t,a), __JOIN3(m,__VA_ARGS__)
#define __JOIN5(m,t,a,...) m(t,a), __JOIN4(m,__VA_ARGS__)
#define __JOIN6(m,t,a,...) m(t,a), __JOIN5(m,__VA_ARGS__)
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
    int __attribute__((always_inline)) type##__##sys##syscall(__JOIN(x,__SC_DECL,__VA_ARGS__)); \
    SEC(#type "/" SYSCALL##word_size##_PREFIX #prefix SYSCALL_PREFIX #syscall #suffix) \
    int type##__ ##word_size##_##prefix ##sys##syscall##suffix(struct pt_regs *ctx) { \
        SYSCALL_##TYPE##_PROLOG(x,__SC_##word_size##_PARAM,syscall,__VA_ARGS__) \
        return type##__sys##syscall(__JOIN(x,__SC_PASS,__VA_ARGS__)); \
    }

#define SYSCALL_HOOK_COMMON(x,type,syscall,...) int __attribute__((always_inline)) type##__sys##syscall(__JOIN(x,__SC_DECL,__VA_ARGS__))

#if USE_SYSCALL_WRAPPER == 1
  #define SYSCALL_PREFIX "sys"
  #define __SC_64_PARAM(n, t, a) t a; bpf_probe_read(&a, sizeof(t), (void*) &SYSCALL64_PT_REGS_PARM##n(ctx));
  #define __SC_32_PARAM(n, t, a) t a; bpf_probe_read(&a, sizeof(t), (void*) &SYSCALL32_PT_REGS_PARM##n(ctx));
  #define SYSCALL_KPROBE_PROLOG(x,m,syscall,...) \
    ctx = (struct pt_regs *) PT_REGS_PARM1(ctx); \
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

#define SYSCALL_KRETPROBE(name, ...) SYSCALL_HOOKx(1,kretprobe,KRETPROBE,,_##name,struct pt_regs*,ctx)

#define SYSCALL_COMPAT_KPROBE0(name, ...) SYSCALL_COMPAT_HOOKx(0,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE1(name, ...) SYSCALL_COMPAT_HOOKx(1,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE2(name, ...) SYSCALL_COMPAT_HOOKx(2,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE3(name, ...) SYSCALL_COMPAT_HOOKx(3,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE4(name, ...) SYSCALL_COMPAT_HOOKx(4,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE5(name, ...) SYSCALL_COMPAT_HOOKx(5,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE6(name, ...) SYSCALL_COMPAT_HOOKx(6,kprobe,KPROBE,_##name,__VA_ARGS__)

#define SYSCALL_COMPAT_KRETPROBE(name, ...) SYSCALL_COMPAT_HOOKx(1,kretprobe,KRETPROBE,_##name,struct pt_regs*,ctx)

#define SYSCALL_COMPAT_TIME_KPROBE0(name, ...) SYSCALL_COMPAT_TIME_HOOKx(0,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE1(name, ...) SYSCALL_COMPAT_TIME_HOOKx(1,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE2(name, ...) SYSCALL_COMPAT_TIME_HOOKx(2,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE3(name, ...) SYSCALL_COMPAT_TIME_HOOKx(3,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE4(name, ...) SYSCALL_COMPAT_TIME_HOOKx(4,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE5(name, ...) SYSCALL_COMPAT_TIME_HOOKx(5,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE6(name, ...) SYSCALL_COMPAT_TIME_HOOKx(6,kprobe,KPROBE,_##name,__VA_ARGS__)

#define SYSCALL_COMPAT_TIME_KRETPROBE(name, ...) SYSCALL_COMPAT_TIME_HOOKx(1,kretprobe,KRETPROBE,_##name,struct pt_regs*,ctx)

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
    EVENT_SETXATTR,
    EVENT_REMOVEXATTR,
    EVENT_EXEC,
};

struct kevent_t {
    u64 type;
};

struct file_t {
    u64 inode;
    u32 mount_id;
    u32 overlay_numlower;
};

struct syscall_t {
    u64 timestamp;
    s64 retval;
};

struct process_context_t {
    u64 pidns;
    char comm[TASK_COMM_LEN];
    char tty_name[TTY_NAME_LEN];
    u32 pid;
    u32 tid;
    u32 uid;
    u32 gid;
    struct file_t executable;
};

struct container_context_t {
    char container_id[CONTAINER_ID_LEN];
};

struct proc_cache_t {
    struct file_t executable;
    char container_id[CONTAINER_ID_LEN];
};

struct bpf_map_def SEC("maps/events") events = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .key_size = sizeof(__u32),
    .value_size = sizeof(__u32),
    .max_entries = 0,
    .pinning = 0,
    .namespace = "",
};

#define send_event(ctx, event) \
    bpf_perf_event_output(ctx, &events, bpf_get_smp_processor_id(), &event, sizeof(event))

struct bpf_map_def SEC("maps/mountpoints_events") mountpoints_events = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .key_size = sizeof(__u32),
    .value_size = sizeof(__u32),
    .max_entries = 0,
    .pinning = 0,
    .namespace = "",
};

#define send_mountpoints_events(ctx, event) \
    bpf_perf_event_output(ctx, &mountpoints_events, bpf_get_smp_processor_id(), &event, sizeof(event))

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

#endif
