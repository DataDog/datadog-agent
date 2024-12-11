#ifndef __BPF_BYPASS_H__
#define __BPF_BYPASS_H__

#include "compiler.h"
#include "map-defs.h"
#include "bpf_telemetry.h"

// default to size 1 so it doesn't accidentally break programs that aren't using it
BPF_ARRAY_MAP(program_bypassed, u32, 1)

// instruct clang that r0-r5 are clobbered, because we are going to make a helper call
#define CHECK_BPF_PROGRAM_BYPASSED() \
    unsigned long bypass_program; \
    asm("%0 = " "bypass_program" " ll" : "=r"(bypass_program) :: "memory");

#define DO_BYPASS CHECK_BPF_PROGRAM_BYPASSED

#define DO_WITH1(a) \
    DO_##a()

#define DO_WITH2(a, b) \
        DO_WITH1(a) \
        DO_WITH1(b) \

#define DO_WITHx(x,...) \
        DO_WITH##x(__VA_ARGS__) \

#define _WITH(x,...) \
        DO_WITHx(x, __VA_ARGS__)

#define WITH(...) \
        _WITH(nargs(__VA_ARGS__), __VA_ARGS__)

#define BPF_KPROBE_INSTR(preamble, name, args...) \
    name(struct pt_regs *ctx);						    \
    static __always_inline typeof(name(0))					    \
    ____##name(struct pt_regs *ctx, ##args);				    \
    typeof(name(0)) name(struct pt_regs *ctx)				    \
    {									    \
        preamble; \
    	_Pragma("GCC diagnostic push")					    \
    	_Pragma("GCC diagnostic ignored \"-Wint-conversion\"")		    \
    	return ____##name(___bpf_kprobe_args(args));			    \
    	_Pragma("GCC diagnostic pop")					    \
    }									    \
    static __always_inline typeof(name(0))					    \
    ____##name(struct pt_regs *ctx, ##args)

#define BPF_KRETPROBE_INSTR(preamble, name, args...) \
    name(struct pt_regs *ctx);						    \
    static __always_inline typeof(name(0))					    \
    ____##name(struct pt_regs *ctx, ##args);				    \
    typeof(name(0)) name(struct pt_regs *ctx)				    \
    {									    \
        preamble; \
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
#define BPF_UPROBE_INSTR(preamble, name, args...)  BPF_KPROBE_INSTR(preamble, name, ##args)
#define BPF_URETPROBE_INSTR(preamble, name, args...)  BPF_KRETPROBE_INSTR(preamble, name, ##args)

#endif
