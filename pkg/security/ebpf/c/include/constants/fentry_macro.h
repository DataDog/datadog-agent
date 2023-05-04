#ifndef _CONSTANTS_FENTRY_MACRO_H_
#define _CONSTANTS_FENTRY_MACRO_H_

#ifdef USE_FENTRY

#define HOOK_ENTRY(func_name) SEC("fentry/" func_name)
typedef unsigned long long ctx_t;
#define CTX_PARM1(ctx) (void *)(ctx[0])

#else

#define HOOK_ENTRY(func_name) SEC("kprobe/" func_name)
typedef struct pt_regs ctx_t;
#define CTX_PARM1(ctx) PT_REGS_PARM1(ctx)

#endif

#endif
