#ifndef __BPF_TRACING_CUSTOM_H__
#define __BPF_TRACING_CUSTOM_H__

#if defined(bpf_target_x86)

#define __PT_PARM6_REG r9
#define PT_REGS_STACK_PARM(x,n)                                                     \
({                                                                                  \
    unsigned long p = 0;                                                            \
    bpf_probe_read_kernel(&p, sizeof(p), ((unsigned long *)x->__PT_SP_REG) + n);    \
    p;                                                                              \
})

#define PT_REGS_PARM7(x) PT_REGS_STACK_PARM(x,1)
#define PT_REGS_PARM8(x) PT_REGS_STACK_PARM(x,2)
#define PT_REGS_PARM9(x) PT_REGS_STACK_PARM(x,3)
#define PT_REGS_PARM10(x) PT_REGS_STACK_PARM(x,4)

#define DI_REGISTER_0 ax
#define DI_REGISTER_1 bx
#define DI_REGISTER_2 cx
#define DI_REGISTER_3 di
#define DI_REGISTER_4 si
#define DI_REGISTER_5 r8
#define DI_REGISTER_6 r9
#define DI_REGISTER_7 r10
#define DI_REGISTER_8 r11

#elif defined(bpf_target_arm64)

#define __PT_PARM6_REG regs[5]
#define PT_REGS_STACK_PARM(x,n)                                            \
({                                                                         \
    unsigned long p = 0;                                                   \
    bpf_probe_read_kernel(&p, sizeof(p), ((unsigned long *)x->sp) + n);    \
    p;                                                                     \
})

#define __PT_PARM6_REG regs[5]
#define __PT_PARM7_REG regs[6]
#define __PT_PARM8_REG regs[7]
#define __PT_PARM9_REG regs[8]

#define DI_REGISTER_0 __PT_PARM1_REG
#define DI_REGISTER_1 __PT_PARM2_REG
#define DI_REGISTER_2 __PT_PARM3_REG
#define DI_REGISTER_3 __PT_PARM4_REG
#define DI_REGISTER_4 __PT_PARM5_REG
#define DI_REGISTER_5 __PT_PARM6_REG
#define DI_REGISTER_6 __PT_PARM7_REG
#define DI_REGISTER_7 __PT_PARM8_REG
#define DI_REGISTER_8 __PT_PARM9_REG

#define PT_REGS_PARM7(x) (__PT_REGS_CAST(x)->regs[6])
#define PT_REGS_PARM8(x) (__PT_REGS_CAST(x)->regs[7])
#define PT_REGS_PARM9(x) PT_REGS_STACK_PARM(__PT_REGS_CAST(x),0)
#define PT_REGS_PARM10(x) PT_REGS_STACK_PARM(__PT_REGS_CAST(x),1)
#define PT_REGS_PARM7_CORE(x) BPF_CORE_READ(__PT_REGS_CAST(x), regs[6])
#define PT_REGS_PARM8_CORE(x) BPF_CORE_READ(__PT_REGS_CAST(x), regs[7])

#endif /* defined(bpf_target_x86) */

#if defined(bpf_target_defined)

#define PT_REGS_PARM6(x) (__PT_REGS_CAST(x)->__PT_PARM6_REG)
#define PT_REGS_PARM6_CORE(x) BPF_CORE_READ(__PT_REGS_CAST(x), __PT_PARM6_REG)

#else /* defined(bpf_target_defined) */

#define PT_REGS_PARM6(x) ({ _Pragma(__BPF_TARGET_MISSING); 0l; })
#define PT_REGS_PARM7(x) ({ _Pragma(__BPF_TARGET_MISSING); 0l; })
#define PT_REGS_PARM8(x) ({ _Pragma(__BPF_TARGET_MISSING); 0l; })
#define PT_REGS_PARM9(x) ({ _Pragma(__BPF_TARGET_MISSING); 0l; })
#define PT_REGS_PARM6_CORE(x) ({ _Pragma(__BPF_TARGET_MISSING); 0l; })
#define PT_REGS_PARM7_CORE(x) ({ _Pragma(__BPF_TARGET_MISSING); 0l; })
#define PT_REGS_PARM8_CORE(x) ({ _Pragma(__BPF_TARGET_MISSING); 0l; })

#endif

#define ___bpf_kprobe_args6(x, args...) ___bpf_kprobe_args5(args), (void *)PT_REGS_PARM6(ctx)
#define ___bpf_kprobe_args7(x, args...) ___bpf_kprobe_args6(args), (void *)PT_REGS_PARM7(ctx)
#define ___bpf_kprobe_args8(x, args...) ___bpf_kprobe_args7(args), (void *)PT_REGS_PARM8(ctx)
#define ___bpf_kprobe_args9(x, args...) ___bpf_kprobe_args8(args), (void *)PT_REGS_PARM9(ctx)

#endif
