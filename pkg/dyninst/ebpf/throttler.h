#ifndef __THROTTLER_H__
#define __THROTTLER_H__

#include "types.h"

typedef struct throttler {
  uint64_t last_probe_run_ns;
  int64_t budget;
} throttler_t;

struct {
  __uint(type, BPF_MAP_TYPE_ARRAY);
  __uint(max_entries, 0);
  __type(key, uint32_t);
  __type(value, throttler_t);
} throttler_buf SEC(".maps");

static bool should_throttle(uint32_t throttler_idx, uint64_t start_ns) {
  throttler_t* throttler = (throttler_t*)bpf_map_lookup_elem(&throttler_buf, &throttler_idx);
  if (!throttler) {
    return true;
  }
  // Try twice to determine throttling result.
  for (int i = 0; i < 2; i++) {
    // Check if we are within budget. First do only a memory read, to avoid
    // contention on a most executed (and thus throttled) probes.
    if (throttler->budget > 0) {
        if (__sync_fetch_and_sub(&throttler->budget, 1) > 0) {
            return false;
        }
    }
    // We are out of budget, check if throttling period passed and budget
    // could be refreshed.
    throttler_params_t* params = bpf_map_lookup_elem(&throttler_params, &throttler_idx);
    if (!params) {
      return true;
    }
    if (throttler->last_probe_run_ns > 0 && start_ns - throttler->last_probe_run_ns < params->period_ns) {
        return true;
    }
    // Try to refresh the budget. We need to make sure we do it only once
    // per the throttling period. We make an assumption that ns measurement
    // is never the same.
    int64_t budget = params->budget;
    uint64_t local_last_probe_run_ns = throttler->last_probe_run_ns;
    if (__sync_val_compare_and_swap(&throttler->last_probe_run_ns, local_last_probe_run_ns, start_ns) == local_last_probe_run_ns) {
        // Note that any probe that reads the budget between the preceeding last_probe_run_ns update and following budget refresh
        // will be rejected. In practise this results in immaterial over-throttling - it requires probing a hot function, in
        // which case we will throttle affected call, and instead probe some future call.
        throttler->budget = budget - 1;
        return false;
    }
    // We failed to refresh the budget, maybe try again.
  }
  // We failed to determine throttling result, conservatively reject.
  return true;
}

#endif // __THROTTLER_H__
