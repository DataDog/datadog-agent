#ifndef __PROBE_H__
#define __PROBE_H__

#include "binary_search.h"
#include "bpf_helpers.h"
#include "debug.h"
#include "framing.h"
#include "scratch.h"
#include "stack_machine.h"
#include "types.h"
#include "vmlinux.h"

// This map is populated from userspace with the registers of the thread with
// the pid key. It is utilized to walk the stack of goroutines which were
// running on a thread at the time of the snapshot.
struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  // This number represents the number of threads for which we can store
  // registers.
  __uint(max_entries, 512);
  // The key is the pid of the thread.
  __type(key, uint32_t);
  __type(value, struct pt_regs);
} thread_regs SEC(".maps");

struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, 1024);
  // The hash of the stack.
  __type(key, uint64_t);
  // The value is irrelevant, but BPF doesn't seem to allow zero-sized values.
  __type(value, uint32_t);
} target_stack_hash_set SEC(".maps");

// Check if the stack hash is in the set, returning true if it is.
bool check_stack_hash(uint64_t stack_hash) {
  const uint32_t* value =
      (uint32_t*)bpf_map_lookup_elem(&target_stack_hash_set, &stack_hash);
  return value != NULL;
}

// From
// https://github.com/torvalds/linux/blob/5a6a09e9/include/uapi/asm-generic/errno-base.h#L21
// TODO: Include a header that defines these.
#define EEXIST 17 /* File exists */

// Check if the stack hash is in the set, and add it if it is not.
// Return true if the stack hash was not in the set and the stack
// should be submitted.
bool upsert_stack_hash(uint64_t stack_hash) {
  const uint32_t zero = 0;
  const int errno = bpf_map_update_elem(&target_stack_hash_set, &stack_hash,
                                        &zero, BPF_NOEXIST);
  if (errno == -EEXIST) {
    return false;
  }
  if (errno != 0) {
    LOG(1, "failed to update target_stack_hash_set %lld (%llx)", stack_hash,
        errno);
  }
  return true;
}

typedef struct target_stack_frame {
  uint64_t fp;
  uint64_t pc;
} target_stack_frame_t;

static long populate_stack_frame(unsigned long _i, void* ctx) {
  stack_walk_ctx_t* g = (*(stack_walk_ctx_t**)(ctx));
  unsigned long i = _i + g->idx_shift;
  if (i >= (STACK_DEPTH - 1)) {
    return 1;
  }
  unsigned long next = i + 1;
  target_stack_frame_t cur;
  if (bpf_probe_read_user(&cur, sizeof(cur), (void*)g->stack.fps[i])) {
    return 1;
  }
  g->stack.fps[next] = cur.fp;
  g->stack.pcs.pcs[next] = cur.pc;
  if (cur.fp == 0) {
    return 1;
  }
  return 0;
}

#endif // __PROBE_H__
