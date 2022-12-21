#ifndef __BPF_CROSS_COMPILE__
#define __BPF_CROSS_COMPILE__

#ifdef COMPILE_CORE
#define bpf_helper_exists(fn) bpf_core_enum_value_exists(enum bpf_func_id, fn)
#endif

#ifdef COMPILE_RUNTIME

#include <linux/version.h>

#define __E_BPF_FUNC_get_current_comm false
#define __E_BPF_FUNC_get_current_task false
#define __E_BPF_FUNC_probe_read_str false

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4,2,0)
#undef __E_BPF_FUNC_get_current_comm
#define __E_BPF_FUNC_get_current_comm true
#endif

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4,8,0)
#undef __E_BPF_FUNC_get_current_task
#define __E_BPF_FUNC_get_current_task true
#endif

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4,11,0)
#undef __E_BPF_FUNC_probe_read_str
#define __E_BPF_FUNC_probe_read_str true
#endif

#define bpf_helper_exists(x) __E_ ## x

#endif /* defined(COMPILE_RUNTIME) */

#endif /* defined(__BPF_CROSS_COMPILE__) */
