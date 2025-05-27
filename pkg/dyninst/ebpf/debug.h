#ifndef __LOG_H__
#define __LOG_H__

#include "bpf_helpers.h"

#ifndef DYNINST_DEBUG

#define LOG(level, fmt, ...)                                                   \

#else

// TODO: port logging to existing utilities
// // DEBUG enables debug logging and controls the level. It is defined during
// // compilation.
// LOG is a macro that prints a message if the level is less than or equal to
// the DEBUG level.
#define LOG(level, fmt, ...)                                                   \
  if (level <= DYNINST_DEBUG) {                                                \
    bpf_printk(fmt, ##__VA_ARGS__);                                            \
  }

static const char* padding(unsigned long depth) {
  // 64 space characters
  static const char spaces[] =
      "                                                                ";
  if (depth > 64) {
    return " <too deep> ";
  }
  return spaces + 64 - depth;
}

#endif

#endif // __LOG_H__
