#ifndef __BPF_CROSS_COMPILE__
#define __BPF_CROSS_COMPILE__

#ifdef COMPILE_CORE
#define bpf_helper_exists(fn) bpf_core_enum_value_exists(enum bpf_func_id, fn)
#endif

#ifdef COMPILE_RUNTIME

#define bpf_helper_exists(x) __E_ ## x

#endif /* defined(COMPILE_RUNTIME) */

#endif /* defined(__BPF_CROSS_COMPILE__) */
