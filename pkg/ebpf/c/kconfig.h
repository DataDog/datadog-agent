#ifndef __KCONFIG_H
#define __KCONFIG_H

#include "bpf_metadata.h"

#include <linux/kconfig.h>
// include asm/compiler.h to fix `error: expected string literal in 'asm'` compilation error coming from mte-kasan.h
// this was fixed in https://github.com/torvalds/linux/commit/b859ebedd1e730bbda69142fca87af4e712649a1
#ifdef CONFIG_HAVE_ARCH_COMPILER_H
#include <asm/compiler.h>
#endif

#endif
