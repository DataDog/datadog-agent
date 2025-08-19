/* SPDX-License-Identifier: (GPL-2.0-only OR BSD-2-Clause) */
/* Copyright Authors of Cilium */

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

/* The LOAD_CONSTANT macro is used to define a named constant that will be replaced
 * at runtime by the Go code. This replaces usage of a bpf_map for storing values, which
 * eliminates a bpf_map_lookup_elem per kprobe hit. The constants are best accessed with a
 * dedicated inlined function.
 */
#define LOAD_CONSTANT(param, var) asm("%0 = " param " ll" : "=r"(var))

#endif /* __BPF_COMPILER_H_ */
