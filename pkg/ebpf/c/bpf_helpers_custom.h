#ifndef __BPF_HELPERS_CUSTOM__
#define __BPF_HELPERS_CUSTOM__

#include "bpf_cross_compile.h"

extern void __format_check(const char *fmt, ...) __attribute__ ((format(printf, 1, 2)));

/*
 * Macro to output debug logs to /sys/kernel/debug/tracing/trace_pipe
 *
 * By default it always adds a newline. A instruction patcher is used to remove
 * the extra one in linux >= 5.9.
 *
 * The trick here is that bpf_trace_printk doesn't seem to care if you pass a size
 * larger than what you actually print, as long as the format string is null terminated
 * somewhere and the buffer itself is of the proper size. That way, we can modify the
 * last character and set it to null.
 */
#ifdef DEBUG
#define log_debug(fmt, ...)                                        \
    ({                                                             \
        char ____fmt[] = fmt "\n";                                 \
        if (0) __format_check(fmt, ##__VA_ARGS__);                 \
        bpf_trace_printk(____fmt, sizeof(____fmt), ##__VA_ARGS__); \
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

// declare our own versions of these enums, because they don't exist on <5.8
enum {
	DD_BPF_RB_NO_WAKEUP = 1,
	DD_BPF_RB_FORCE_WAKEUP = 2,
};

enum {
	DD_BPF_RB_AVAIL_DATA = 0,
	DD_BPF_RB_RING_SIZE = 1,
	DD_BPF_RB_CONS_POS = 2,
	DD_BPF_RB_PROD_POS = 3,
};

#endif
