#ifndef __BPF_HELPERS_CUSTOM__
#define __BPF_HELPERS_CUSTOM__

#include "bpf_cross_compile.h"

/* Macro to output debug logs to /sys/kernel/debug/tracing/trace_pipe
 */
#ifdef DEBUG
#define log_debug(fmt, ...)                                        \
    ({                                                             \
        char ____fmt[] = fmt;                                      \
        bpf_trace_printk(____fmt, sizeof(____fmt), ##__VA_ARGS__); \
    })
#else
// No op
#define log_debug(fmt, ...)
#endif

/* llvm builtin functions that eBPF C program may use to
 * emit BPF_LD_ABS and BPF_LD_IND instructions
 */
unsigned long long load_byte(void* skb,
    unsigned long long off) asm("llvm.bpf.load.byte");
unsigned long long load_half(void* skb,
    unsigned long long off) asm("llvm.bpf.load.half");
unsigned long long load_word(void* skb,
    unsigned long long off) asm("llvm.bpf.load.word");

#endif
