#ifndef __BPF_COMPILER_H_
#define __BPF_COMPILER_H_

#ifndef __maybe_unused
# define __maybe_unused		__attribute__((__unused__))
#endif

#ifndef __nobuiltin
# if __clang_major__ >= 10
#  define __nobuiltin(X)	__attribute__((no_builtin(X)))
# else
#  define __nobuiltin(X)
# endif
#endif

#ifndef __throw_build_bug
# define __throw_build_bug()	__builtin_trap()
#endif

#ifndef barrier
# define barrier()		asm volatile("": : :"memory")
#endif

#ifndef barrier_data
# define barrier_data(ptr)	asm volatile("": :"r"(ptr) :"memory")
#endif

static __always_inline void bpf_barrier(void)
{
	/* Workaround to avoid verifier complaint:
	 * "dereference of modified ctx ptr R5 off=48+0, ctx+const is allowed,
	 *        ctx+const+const is not"
	 */
	barrier();
}

#endif /* __BPF_COMPILER_H_ */
