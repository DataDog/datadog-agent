#ifndef _DEFS_H_
#define _DEFS_H_

#include "compiler.h"

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


// verbose logs will trigger themselves in a loop when you are ssh'd in.
// disable this if you want cleaner local debugging output
#define LOG_VERBOSE

#ifdef LOG_VERBOSE
#define log_verbose(format, ...) log_debug(format, ##__VA_ARGS__)
#else
#define log_verbose(format, ...)
#endif

#endif
