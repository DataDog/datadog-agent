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

#define DO_WITH1(name, a) \
static __always_inline void ____bpf_preamble__##name##__##a() { \
    DO_##a() \
}\

#define DO_WITH2(name, a, b) \
        DO_WITH1(name, a) \
        DO_WITH1(name, b) \

#define DO_WITH_CALL1(name, a) \
        static __always_inline void ____bpf_preamble__##name() { \
            ____bpf_preamble__##name##__##a(); \
        }

#define DO_WITH_CALL2(name, a, b) \
        static __always_inline void ____bpf_preamble__##name() { \
            ____bpf_preamble__##name##__##a(); \
            ____bpf_preamble__##name##__##b(); \
        }

#define DO_WITHx(name, x,...) \
        DO_WITH##x(name, __VA_ARGS__) \
        DO_WITH_CALL##x(name, __VA_ARGS__)

#define _WITH(name, x,...) \
        DO_WITHx(name, x, __VA_ARGS__)

#define WITH_PREAMBLE(name, ...) \
        _WITH(name, nargs(__VA_ARGS__), __VA_ARGS__)

#define WITH(...) \
    __VA_ARGS__

#define BPF_KPROBE_INSTR(preamble, name, args...) \
    name(struct pt_regs *ctx);						    \
    WITH_PREAMBLE(name, preamble) \
    static __always_inline typeof(name(0))					    \
    ____##name(struct pt_regs *ctx, ##args);				    \
    typeof(name(0)) name(struct pt_regs *ctx)				    \
    {									    \
        ____bpf_preamble__##name(); \
    	_Pragma("GCC diagnostic push")					    \
    	_Pragma("GCC diagnostic ignored \"-Wint-conversion\"")		    \
    	return ____##name(___bpf_kprobe_args(args));			    \
    	_Pragma("GCC diagnostic pop")					    \
    }									    \
    static __always_inline typeof(name(0))					    \
    ____##name(struct pt_regs *ctx, ##args)

#define BPF_KRETPROBE_INSTR(preamble, name, args...) \
    name(struct pt_regs *ctx);						    \
    WITH_PREAMBLE(name, preamble) \
    static __always_inline typeof(name(0))					    \
    ____##name(struct pt_regs *ctx, ##args);				    \
    typeof(name(0)) name(struct pt_regs *ctx)				    \
    {									    \
        ____bpf_preamble__##name(); \
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
