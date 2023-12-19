#ifndef __BPF_HELPERS_CUSTOM__
#define __BPF_HELPERS_CUSTOM__

#include "bpf_cross_compile.h"

struct trace_event_raw_bpf_trace_printk___x {};

#ifdef BPF_NO_GLOBAL_DATA
#define BPF_PRINTK_FMT_MOD
#else
#define BPF_PRINTK_FMT_MOD static const
#endif

#ifdef COMPILE_CORE
#define BPF_PRINTK_ADDS_NEWLINE bpf_core_type_exists(struct trace_event_raw_bpf_trace_printk___x)
#elif LINUX_VERSION_CODE >= KERNEL_VERSION(5, 9, 0)
#define BPF_PRINTK_ADDS_NEWLINE 1
#else
#define BPF_PRINTK_ADDS_NEWLINE 0
#endif

#undef DEBUG
#define DEBUG 1

/*
 * Macro to output debug logs to /sys/kernel/debug/tracing/trace_pipe
 *
 * Some sources regarding the feature detection:
 * -·In https://github.com/torvalds/linux/commit/ac5a72ea5c898, kernel tracepoint
 *   bpf_trace_printk is added at the same time that bpf_trace_printk starts adding
 *   newlines by itself. Use that for transparent newline detection.
 */
#ifdef DEBUG
#define log_debug(fmt, ...)                                            \
    ({                                                                 \
        if (BPF_PRINTK_ADDS_NEWLINE) {                                 \
            BPF_PRINTK_FMT_MOD char ____fmt[] = fmt;                   \
            bpf_trace_printk(____fmt, sizeof(____fmt), ##__VA_ARGS__); \
        } else {                                                       \
            BPF_PRINTK_FMT_MOD char ____fmt[] = fmt "\n";              \
            bpf_trace_printk(____fmt, sizeof(____fmt), ##__VA_ARGS__); \
        }                                                              \
    })
#else
// No op
#define log_debug(fmt, ...)
#endif

/* llvm builtin functions that eBPF C program may use to
 * emit BPF_LD_ABS and BPF_LD_IND instructions
 */
unsigned long long
load_byte(void *skb,
    unsigned long long off) asm("llvm.bpf.load.byte");
unsigned long long load_half(void *skb,
    unsigned long long off) asm("llvm.bpf.load.half");
unsigned long long load_word(void *skb,
    unsigned long long off) asm("llvm.bpf.load.word");

#endif
