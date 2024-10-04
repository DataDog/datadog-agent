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
#define __PT_PARM10_REG regs[9]
#define __PT_PARM11_REG regs[10]
#define __PT_PARM12_REG regs[11]
#define __PT_PARM13_REG regs[12]
#define __PT_PARM14_REG regs[13]
#define __PT_PARM15_REG regs[14]
#define __PT_PARM16_REG regs[15]
#define __PT_PARM17_REG regs[16]

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
