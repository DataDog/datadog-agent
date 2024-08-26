#ifndef __BPF_BYPASS_H__
#define __BPF_BYPASS_H__

#include "compiler.h"
#include "map-defs.h"

// default to size 1 so it doesn't accidentally break programs that aren't using it
BPF_ARRAY_MAP(program_bypassed, u32, 1)

// instruct clang that r0-r5 are clobbered, because we are going to make a helper call
#define CHECK_BPF_PROGRAM_BYPASSED() \
    unsigned long bypass_program; \
    asm("%0 = " "bypass_program" " ll" : "=r"(bypass_program) :: "memory");

#define BPF_BYPASSABLE_KPROBE(name, args...)					    \
name(struct pt_regs *ctx);						    \
static __always_inline typeof(name(0))					    \
____##name(struct pt_regs *ctx, ##args);				    \
typeof(name(0)) name(struct pt_regs *ctx)				    \
{									    \
    CHECK_BPF_PROGRAM_BYPASSED()                            \
	_Pragma("GCC diagnostic push")					    \
	_Pragma("GCC diagnostic ignored \"-Wint-conversion\"")		    \
	return ____##name(___bpf_kprobe_args(args));			    \
	_Pragma("GCC diagnostic pop")					    \
}									    \
static __always_inline typeof(name(0))					    \
____##name(struct pt_regs *ctx, ##args)

#define BPF_BYPASSABLE_KRETPROBE(name, args...)					    \
name(struct pt_regs *ctx);						    \
static __always_inline typeof(name(0))					    \
____##name(struct pt_regs *ctx, ##args);				    \
typeof(name(0)) name(struct pt_regs *ctx)				    \
{									    \
    CHECK_BPF_PROGRAM_BYPASSED()                            \
	_Pragma("GCC diagnostic push")					    \
	_Pragma("GCC diagnostic ignored \"-Wint-conversion\"")		    \
	return ____##name(___bpf_kretprobe_args(args));			    \
	_Pragma("GCC diagnostic pop")					    \
}									    \
static __always_inline typeof(name(0)) ____##name(struct pt_regs *ctx, ##args)

/* BPF_BYPASSABLE_UPROBE and BPF_BYPASSABLE_URETPROBE are identical to BPF_BYPASSABLE_KPROBE and BPF_BYPASSABLE_KRETPROBE,
 * but are named way less confusingly for SEC("uprobe") and SEC("uretprobe")
 * use cases.
 */
#define BPF_BYPASSABLE_UPROBE(name, args...)  BPF_BYPASSABLE_KPROBE(name, ##args)
#define BPF_BYPASSABLE_URETPROBE(name, args...)  BPF_BYPASSABLE_KRETPROBE(name, ##args)

#endif
