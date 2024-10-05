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
#define __PT_PARM10_REG regs[9]
#define __PT_PARM11_REG regs[10]
#define __PT_PARM12_REG regs[11]
#define __PT_PARM13_REG regs[12]
#define __PT_PARM14_REG regs[13]
#define __PT_PARM15_REG regs[14]
#define __PT_PARM16_REG regs[15]
#define __PT_PARM17_REG regs[16]
#define __PT_PARM18_REG regs[17]
#define __PT_PARM19_REG regs[18]
#define __PT_PARM20_REG regs[19]
#define __PT_PARM21_REG regs[20]
#define __PT_PARM22_REG regs[21]
#define __PT_PARM23_REG regs[22]
#define __PT_PARM24_REG regs[23]
#define __PT_PARM25_REG regs[24]
#define __PT_PARM26_REG regs[25]
#define __PT_PARM27_REG regs[26]
#define __PT_PARM28_REG regs[27]
#define __PT_PARM29_REG regs[28]
#define __PT_PARM30_REG regs[29]
#define __PT_PARM31_REG regs[30]
#define __PT_PARM32_REG regs[31]

#define DI_REGISTER_0 __PT_PARM1_REG
#define DI_REGISTER_1 __PT_PARM2_REG
#define DI_REGISTER_2 __PT_PARM3_REG
#define DI_REGISTER_3 __PT_PARM4_REG
#define DI_REGISTER_4 __PT_PARM5_REG
#define DI_REGISTER_5 __PT_PARM6_REG
#define DI_REGISTER_6 __PT_PARM7_REG
#define DI_REGISTER_7 __PT_PARM8_REG
#define DI_REGISTER_8 __PT_PARM9_REG
#define DI_REGISTER_9 __PT_PARM10_REG
#define DI_REGISTER_10 __PT_PARM11_REG
#define DI_REGISTER_11 __PT_PARM12_REG
#define DI_REGISTER_12 __PT_PARM13_REG
#define DI_REGISTER_13 __PT_PARM14_REG
#define DI_REGISTER_14 __PT_PARM15_REG
#define DI_REGISTER_15 __PT_PARM16_REG
#define DI_REGISTER_16 __PT_PARM17_REG
#define DI_REGISTER_17 __PT_PARM18_REG
#define DI_REGISTER_18 __PT_PARM19_REG
#define DI_REGISTER_19 __PT_PARM20_REG
#define DI_REGISTER_20 __PT_PARM21_REG
#define DI_REGISTER_21 __PT_PARM22_REG
#define DI_REGISTER_22 __PT_PARM23_REG
#define DI_REGISTER_23 __PT_PARM24_REG
#define DI_REGISTER_24 __PT_PARM25_REG
#define DI_REGISTER_25 __PT_PARM26_REG
#define DI_REGISTER_26 __PT_PARM27_REG
#define DI_REGISTER_27 __PT_PARM28_REG
#define DI_REGISTER_28 __PT_PARM29_REG
#define DI_REGISTER_29 __PT_PARM30_REG
#define DI_REGISTER_30 __PT_PARM31_REG
#define DI_REGISTER_31 __PT_PARM32_REG

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
