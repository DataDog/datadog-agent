#ifndef _CONSTANTS_FENTRY_MACRO_H_
#define _CONSTANTS_FENTRY_MACRO_H_

#ifdef USE_FENTRY

#define HOOK_ENTRY(func_name) SEC("fentry/" func_name)
#define HOOK_EXIT(func_name) SEC("fexit/" func_name)
typedef unsigned long long ctx_t;
#define CTX_PARM1(ctx) (u64)(ctx[0])
#define CTX_PARM2(ctx) (u64)(ctx[1])
#define CTX_PARM3(ctx) (u64)(ctx[2])

#define CTX_PARMRET(ctx, argc) (u64)(ctx[argc+1])

#else

#define HOOK_ENTRY(func_name) SEC("kprobe/" func_name)
#define HOOK_EXIT(func_name) SEC("kretprobe/" func_name)
typedef struct pt_regs ctx_t;
#define CTX_PARM1(ctx) PT_REGS_PARM1(ctx)
#define CTX_PARM2(ctx) PT_REGS_PARM2(ctx)
#define CTX_PARM3(ctx) PT_REGS_PARM3(ctx)

#define CTX_PARMRET(ctx, _argc) PT_REGS_RC(ctx)

#endif

#endif
