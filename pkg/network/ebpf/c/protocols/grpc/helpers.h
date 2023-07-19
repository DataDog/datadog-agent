#ifndef __GRPC_HELPERS_H
#define __GRPC_HELPERS_H

#include "bpf_builtins.h"

static __always_inline bool is_grpc(const char *buf, __u32 size) {
  return false;
}

#endif
