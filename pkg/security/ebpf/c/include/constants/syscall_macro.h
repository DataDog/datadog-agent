#ifndef _CONSTANTS_SYSCALL_MACRO_H_
#define _CONSTANTS_SYSCALL_MACRO_H_

#if defined(__x86_64__)
  #if USE_SYSCALL_WRAPPER == 1
    #define SYSCALL64_PREFIX "__x64_"
    #define SYSCALL32_PREFIX "__ia32_"
  #else
    #define SYSCALL32_PREFIX ""
    #define SYSCALL64_PREFIX ""
  #endif

  #define SYSCALL64_PT_REGS_PARM1(x) ((x)->di)
  #define SYSCALL64_PT_REGS_PARM2(x) ((x)->si)
  #define SYSCALL64_PT_REGS_PARM3(x) ((x)->dx)
  #if USE_SYSCALL_WRAPPER == 1
    #define SYSCALL64_PT_REGS_PARM4(x) ((x)->r10)
  #else
    #define SYSCALL64_PT_REGS_PARM4(x) ((x)->cx)
  #endif
  #define SYSCALL64_PT_REGS_PARM5(x) ((x)->r8)
  #define SYSCALL64_PT_REGS_PARM6(x) ((x)->r9)

  #define SYSCALL32_PT_REGS_PARM1(x) ((x)->bx)
  #define SYSCALL32_PT_REGS_PARM2(x) ((x)->cx)
  #define SYSCALL32_PT_REGS_PARM3(x) ((x)->dx)
  #define SYSCALL32_PT_REGS_PARM4(x) ((x)->si)
  #define SYSCALL32_PT_REGS_PARM5(x) ((x)->di)
  #define SYSCALL32_PT_REGS_PARM6(x) ((x)->bp)

#elif defined(__aarch64__)
  #if USE_SYSCALL_WRAPPER == 1
    #define SYSCALL64_PREFIX "__arm64_"
    #define SYSCALL32_PREFIX "__arm32_"
  #else
    #define SYSCALL32_PREFIX ""
    #define SYSCALL64_PREFIX ""
  #endif

  #define SYSCALL64_PT_REGS_PARM1(x) PT_REGS_PARM1(x)
  #define SYSCALL64_PT_REGS_PARM2(x) PT_REGS_PARM2(x)
  #define SYSCALL64_PT_REGS_PARM3(x) PT_REGS_PARM3(x)
  #define SYSCALL64_PT_REGS_PARM4(x) PT_REGS_PARM4(x)
  #define SYSCALL64_PT_REGS_PARM5(x) PT_REGS_PARM5(x)
  #define SYSCALL64_PT_REGS_PARM6(x) PT_REGS_PARM6(x)

  #define SYSCALL32_PT_REGS_PARM1(x) PT_REGS_PARM1(x)
  #define SYSCALL32_PT_REGS_PARM2(x) PT_REGS_PARM2(x)
  #define SYSCALL32_PT_REGS_PARM3(x) PT_REGS_PARM3(x)
  #define SYSCALL32_PT_REGS_PARM4(x) PT_REGS_PARM4(x)
  #define SYSCALL32_PT_REGS_PARM5(x) PT_REGS_PARM5(x)
  #define SYSCALL32_PT_REGS_PARM6(x) PT_REGS_PARM6(x)

#else
  #error "Unsupported platform"
#endif

/*
 * __MAP - apply a macro to syscall arguments
 * __MAP(n, m, t1, a1, t2, a2, ..., tn, an) will expand to
 *    m(t1, a1), m(t2, a2), ..., m(tn, an)
 * The first argument must be equal to the amount of type/name
 * pairs given.  Note that this list of pairs (i.e. the arguments
 * of __MAP starting at the third one) is in the same format as
 * for SYSCALL_DEFINE<n>/COMPAT_SYSCALL_DEFINE<n>
 */
#define __JOIN0(m,...)
#define __JOIN1(m,t,a,...) ,m(t,a)
#define __JOIN2(m,t,a,...) ,m(t,a) __JOIN1(m,__VA_ARGS__)
#define __JOIN3(m,t,a,...) ,m(t,a) __JOIN2(m,__VA_ARGS__)
#define __JOIN4(m,t,a,...) ,m(t,a) __JOIN3(m,__VA_ARGS__)
#define __JOIN5(m,t,a,...) ,m(t,a) __JOIN4(m,__VA_ARGS__)
#define __JOIN6(m,t,a,...) ,m(t,a) __JOIN5(m,__VA_ARGS__)
#define __JOIN(n,...) __JOIN##n(__VA_ARGS__)

#define __MAP0(n,m,...)
#define __MAP1(n,m,t1,a1,...) m(1,t1,a1)
#define __MAP2(n,m,t1,a1,t2,a2) m(1,t1,a1) m(2,t2,a2)
#define __MAP3(n,m,t1,a1,t2,a2,t3,a3) m(1,t1,a1) m(2,t2,a2) m(3,t3,a3)
#define __MAP4(n,m,t1,a1,t2,a2,t3,a3,t4,a4) m(1,t1,a1) m(2,t2,a2) m(3,t3,a3) m(4,t4,a4)
#define __MAP5(n,m,t1,a1,t2,a2,t3,a3,t4,a4,t5,a5) m(1,t1,a1) m(2,t2,a2) m(3,t3,a3) m(4,t4,a4) m(5,t5,a5)
#define __MAP6(n,m,t1,a1,t2,a2,t3,a3,t4,a4,t5,a5,t6,a6) m(1,t1,a1) m(2,t2,a2) m(3,t3,a3) m(4,t4,a4) m(5,t5,a5) m(6,t6,a6)
#define __MAP(n,...) __MAP##n(n,__VA_ARGS__)

#define __SC_DECL(t, a) t a
#define __SC_PASS(t, a) a

#define SYSCALL_PREFIX "sys"
#define KPROBE_CTX_TYPE struct pt_regs
#define KRETPROBE_CTX_TYPE struct pt_regs
#define FENTRY_CTX_TYPE ctx_t
#define FEXIT_CTX_TYPE ctx_t

#define SYSCALL_ABI_HOOKx(x,word_size,type,TYPE,prefix,syscall,suffix,...) \
    int __attribute__((always_inline)) type##__##sys##syscall(TYPE##_CTX_TYPE *ctx __JOIN(x,__SC_DECL,__VA_ARGS__)); \
    SEC(#type "/" SYSCALL##word_size##_PREFIX #prefix SYSCALL_PREFIX #syscall #suffix) \
    int type##__ ##word_size##_##prefix ##sys##syscall##suffix(TYPE##_CTX_TYPE *ctx) { \
        SYSCALL_##TYPE##_PROLOG(x,__SC_##word_size##_PARAM,syscall,__VA_ARGS__) \
        return type##__sys##syscall(ctx __JOIN(x,__SC_PASS,__VA_ARGS__)); \
    }

#define SYSCALL_HOOK_COMMON(x,type,TYPE,syscall,...) int __attribute__((always_inline)) type##__sys##syscall(TYPE##_CTX_TYPE *ctx __JOIN(x,__SC_DECL,__VA_ARGS__))
#define SYSCALL_KRETPROBE_PROLOG(...)
#define SYSCALL_FEXIT_PROLOG(...)

#define SYSCALL_FENTRY_PROLOG(x,m,syscall,...) \
  struct pt_regs *rctx = (struct pt_regs *) (ctx)[0]; \
  if (!rctx) return 0; \
  __MAP(x,m,__VA_ARGS__)

#if USE_SYSCALL_WRAPPER == 1
  #define __SC_64_PARAM(n, t, a) t a; bpf_probe_read(&a, sizeof(t), (void*) &SYSCALL64_PT_REGS_PARM##n(rctx));
  #define __SC_32_PARAM(n, t, a) t a; bpf_probe_read(&a, sizeof(t), (void*) &SYSCALL32_PT_REGS_PARM##n(rctx));
  #define SYSCALL_KPROBE_PROLOG(x,m,syscall,...) \
    struct pt_regs *rctx = (struct pt_regs *) PT_REGS_PARM1(ctx); \
    if (!rctx) return 0; \
    __MAP(x,m,__VA_ARGS__)
  #define SYSCALL_HOOKx(x,type,TYPE,prefix,name,...) \
    SYSCALL_ABI_HOOKx(x,32,type,TYPE,prefix,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_HOOK_COMMON(x,type,TYPE,name,__VA_ARGS__)
  #define SYSCALL_COMPAT_HOOKx(x,type,TYPE,name,...) \
    SYSCALL_ABI_HOOKx(x,32,type,TYPE,compat_,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_HOOK_COMMON(x,type,TYPE,name,__VA_ARGS__)
  #define SYSCALL_TIME_HOOKx(x,type,TYPE,name,...) \
    SYSCALL_ABI_HOOKx(x,32,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,32,type,TYPE,,name,_time32,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,_time32,__VA_ARGS__) \
    SYSCALL_HOOK_COMMON(x,type,TYPE,name,__VA_ARGS__)
  #define SYSCALL_COMPAT_TIME_HOOKx(x,type,TYPE,name,...) \
    SYSCALL_ABI_HOOKx(x,32,type,TYPE,compat_,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,32,type,TYPE,,name,_time32,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,_time32,__VA_ARGS__) \
    SYSCALL_HOOK_COMMON(x,type,TYPE,name,__VA_ARGS__)
#else
  #define __SC_64_PARAM(n, t, a) t a = (t) SYSCALL64_PT_REGS_PARM##n(rctx);
  #define __SC_32_PARAM(n, t, a) t a = (t) SYSCALL32_PT_REGS_PARM##n(rctx);
  #define SYSCALL_KPROBE_PROLOG(x,m,syscall,...) \
    struct pt_regs *rctx = ctx; \
    if (!rctx) return 0; \
    __MAP(x,m,__VA_ARGS__)
  #define SYSCALL_HOOKx(x,type,TYPE,prefix,name,...) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,compat_,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_HOOK_COMMON(x,type,TYPE,name,__VA_ARGS__)
  #define SYSCALL_COMPAT_HOOKx(x,type,TYPE,name,...) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,compat_,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_HOOK_COMMON(x,type,TYPE,name,__VA_ARGS__)
  #define SYSCALL_TIME_HOOKx(x,type,TYPE,name,...) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_HOOK_COMMON(x,type,TYPE,name,__VA_ARGS__)
  #define SYSCALL_COMPAT_TIME_HOOKx(x,type,TYPE,name,...) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,compat_,name,,__VA_ARGS__) \
    SYSCALL_ABI_HOOKx(x,64,type,TYPE,,name,,__VA_ARGS__) \
    SYSCALL_HOOK_COMMON(x,type,TYPE,name,__VA_ARGS__)
#endif

#define SYSCALL_KPROBE0(name, ...) SYSCALL_HOOKx(0,kprobe,KPROBE,,_##name,__VA_ARGS__)
#define SYSCALL_KPROBE1(name, ...) SYSCALL_HOOKx(1,kprobe,KPROBE,,_##name,__VA_ARGS__)
#define SYSCALL_KPROBE2(name, ...) SYSCALL_HOOKx(2,kprobe,KPROBE,,_##name,__VA_ARGS__)
#define SYSCALL_KPROBE3(name, ...) SYSCALL_HOOKx(3,kprobe,KPROBE,,_##name,__VA_ARGS__)
#define SYSCALL_KPROBE4(name, ...) SYSCALL_HOOKx(4,kprobe,KPROBE,,_##name,__VA_ARGS__)
#define SYSCALL_KPROBE5(name, ...) SYSCALL_HOOKx(5,kprobe,KPROBE,,_##name,__VA_ARGS__)
#define SYSCALL_KPROBE6(name, ...) SYSCALL_HOOKx(6,kprobe,KPROBE,,_##name,__VA_ARGS__)

#define SYSCALL_FENTRY0(name, ...) SYSCALL_HOOKx(0,fentry,FENTRY,,_##name,__VA_ARGS__)
#define SYSCALL_FENTRY1(name, ...) SYSCALL_HOOKx(1,fentry,FENTRY,,_##name,__VA_ARGS__)
#define SYSCALL_FENTRY2(name, ...) SYSCALL_HOOKx(2,fentry,FENTRY,,_##name,__VA_ARGS__)
#define SYSCALL_FENTRY3(name, ...) SYSCALL_HOOKx(3,fentry,FENTRY,,_##name,__VA_ARGS__)
#define SYSCALL_FENTRY4(name, ...) SYSCALL_HOOKx(4,fentry,FENTRY,,_##name,__VA_ARGS__)
#define SYSCALL_FENTRY5(name, ...) SYSCALL_HOOKx(5,fentry,FENTRY,,_##name,__VA_ARGS__)
#define SYSCALL_FENTRY6(name, ...) SYSCALL_HOOKx(6,fentry,FENTRY,,_##name,__VA_ARGS__)

#define SYSCALL_KRETPROBE(name) SYSCALL_HOOKx(0,kretprobe,KRETPROBE,,_##name)

#define SYSCALL_FEXIT(name) SYSCALL_HOOKx(0,fexit,FEXIT,,_##name,__VA_ARGS__)

#define SYSCALL_COMPAT_KPROBE0(name, ...) SYSCALL_COMPAT_HOOKx(0,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE1(name, ...) SYSCALL_COMPAT_HOOKx(1,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE2(name, ...) SYSCALL_COMPAT_HOOKx(2,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE3(name, ...) SYSCALL_COMPAT_HOOKx(3,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE4(name, ...) SYSCALL_COMPAT_HOOKx(4,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE5(name, ...) SYSCALL_COMPAT_HOOKx(5,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_KPROBE6(name, ...) SYSCALL_COMPAT_HOOKx(6,kprobe,KPROBE,_##name,__VA_ARGS__)

#define SYSCALL_COMPAT_KRETPROBE(name, ...) SYSCALL_COMPAT_HOOKx(0,kretprobe,KRETPROBE,_##name)

#define SYSCALL_COMPAT_TIME_KPROBE0(name, ...) SYSCALL_COMPAT_TIME_HOOKx(0,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE1(name, ...) SYSCALL_COMPAT_TIME_HOOKx(1,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE2(name, ...) SYSCALL_COMPAT_TIME_HOOKx(2,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE3(name, ...) SYSCALL_COMPAT_TIME_HOOKx(3,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE4(name, ...) SYSCALL_COMPAT_TIME_HOOKx(4,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE5(name, ...) SYSCALL_COMPAT_TIME_HOOKx(5,kprobe,KPROBE,_##name,__VA_ARGS__)
#define SYSCALL_COMPAT_TIME_KPROBE6(name, ...) SYSCALL_COMPAT_TIME_HOOKx(6,kprobe,KPROBE,_##name,__VA_ARGS__)

#define SYSCALL_TIME_FENTRY0(name, ...) SYSCALL_TIME_HOOKx(0,fentry,FENTRY,_##name,__VA_ARGS__)

#define SYSCALL_TIME_FEXIT(name) SYSCALL_TIME_HOOKx(0,fexit,FEXIT,_##name,__VA_ARGS__)

#define SYSCALL_COMPAT_TIME_KRETPROBE(name, ...) SYSCALL_COMPAT_TIME_HOOKx(0,kretprobe,KRETPROBE,_##name)

#endif
