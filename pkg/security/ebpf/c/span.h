#ifndef _SPAN_H_
#define _SPAN_H_

#include "defs.h"

enum tls_format {
   DEFAULT
};

struct span_tls_t {
   u64 format;
   u64 max_threads;
   void *base;
};

struct bpf_map_def SEC("maps/span_tls") span_tls = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct span_tls_t),
    .max_entries = 4096,
    .pinning = 0,
    .namespace = "",
};

// defined in process.h
u64 lookup_pid_ns(u64 pid_tgid);

int __attribute__((always_inline)) handle_register_span_memory(void *data) {
   struct span_tls_t tls = {};
   bpf_probe_read(&tls, sizeof(tls), data);

   u64 pid_tgid = bpf_get_current_pid_tgid();
   u32 tgid = pid_tgid >> 32;

   bpf_map_update_elem(&span_tls, &tgid, &tls, BPF_NOEXIST);

   return 0;
}

int __attribute__((always_inline)) unregister_span_memory() {
   u64 pid_tgid = bpf_get_current_pid_tgid();
   u32 tgid = pid_tgid >> 32;

   bpf_map_delete_elem(&span_tls, &tgid);

   return 0;
}

void __attribute__((always_inline)) fill_span_context(struct span_context_t *span) {
   u64 pid_tgid = bpf_get_current_pid_tgid();
   u32 tgid = pid_tgid >> 32;
  
   struct span_tls_t *tls = bpf_map_lookup_elem(&span_tls, &tgid);
   if (tls) {
      u32 tid = pid_tgid;

      u64 pid = lookup_pid_ns(pid_tgid);
      if (pid) {
         tid = pid;
      }

      int offset = (tid % tls->max_threads) * sizeof(struct span_context_t);
      int ret = bpf_probe_read(span, sizeof(struct span_context_t), tls->base + offset);
      if (ret < 0) {
         span->span_id = 0;
         span->trace_id = 0;
      }
   }
}

void __attribute__((always_inline)) copy_span_context(struct span_context_t *src, struct span_context_t *dst) {
   dst->span_id = src->span_id;
   dst->trace_id = src->trace_id;
}

#endif