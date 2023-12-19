#ifndef __BPF_HELPERS_CUSTOM__
#define __BPF_HELPERS_CUSTOM__

#include "bpf_cross_compile.h"

// If we can, try to move the definition of the format string off
// the stack and into the read-only section of the binary.
#ifdef BPF_NO_GLOBAL_DATA
#define BPF_PRINTK_FMT_MOD
#else
#define BPF_PRINTK_FMT_MOD static const
#endif

/*
 * The existence of this tracepoint is used to detect if the bpf_trace_printk
 * function adds a newline to the output or not (added in
 * https://github.com/torvalds/linux/commit/ac5a72ea5c898, went upstream with 5.9)
 *
 * We define our own struct definition if our vmlinux.h is outdated.
 * BPF CO-RE ignores the ___ and everything after it
 */
struct trace_event_raw_bpf_trace_printk___x {};

#ifdef COMPILE_CORE
#define BPF_PRINTK_ADDS_NEWLINE bpf_core_type_exists(struct trace_event_raw_bpf_trace_printk___x)
#elif LINUX_VERSION_CODE >= KERNEL_VERSION(5, 9, 0)
#define BPF_PRINTK_ADDS_NEWLINE 1
#else
#define BPF_PRINTK_ADDS_NEWLINE 0
#endif

/*
 * Macro to output debug logs to /sys/kernel/debug/tracing/trace_pipe
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
