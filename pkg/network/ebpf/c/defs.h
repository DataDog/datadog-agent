#ifndef _DEFS_H_
#define _DEFS_H_

__maybe_unused static const __u64 ENABLED = 1;

#ifdef COMPILE_CORE
#define MAX_ERRNO 4095
#define IS_ERR_VALUE(x) ((unsigned long)(void *)(x) >= (unsigned long)-MAX_ERRNO)

static __always_inline bool IS_ERR_OR_NULL(const void *ptr)
{
    return !ptr || IS_ERR_VALUE((unsigned long)ptr);
}
#else
#include <linux/err.h>
#endif // COMPILE_CORE

#endif
