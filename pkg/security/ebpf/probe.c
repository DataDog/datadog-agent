#include <linux/compiler.h>

#include <linux/kconfig.h>

/* clang 8 does not support "asm volatile goto" yet.
 * So redefine asm_volatile_goto to some invalid asm code.
 * If asm_volatile_goto is actually used by the bpf program,
 * a compilation error will appear.
 */
#ifdef asm_volatile_goto
#undef asm_volatile_goto
#endif
#define asm_volatile_goto(x...) asm volatile("invalid use of asm_volatile_goto")
#pragma clang diagnostic ignored "-Wunused-label"

#include <linux/ptrace.h>
#include <linux/types.h>
#include <linux/version.h>

#include "defs.h"
#include "process.h"
#include "dentry.h"
#include "mkdir.h"
#include "rmdir.h"
#include "unlink.h"
#include "rename.h"
#include "open.h"

__u32 _version SEC("version") = 0xFFFFFFFE;

char LICENSE[] SEC("license") = "GPL";