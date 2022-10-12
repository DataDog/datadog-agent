#ifndef _DEFS_H_
#define _DEFS_H_

/* The LOAD_CONSTANT macro is used to define a named constant that will be replaced
 * at runtime by the Go code. This replaces usage of a bpf_map for storing values, which
 * eliminates a bpf_map_lookup_elem per kprobe hit. The constants are best accessed with a
 * dedicated inlined function. See example functions offset_* below.
 */
#define LOAD_CONSTANT(param, var) asm("%0 = " param " ll" : "=r"(var))

__maybe_unused static const __u64 ENABLED = 1;

#endif
