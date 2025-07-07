#ifndef __QUEUE_H__
#define __QUEUE_H__

#include "bpf_helpers.h"
#include "vmlinux.h"

#define DEFINE_QUEUE(prefix, elem_t, max_length)                               \
  typedef struct prefix##_queue {                                              \
    uint32_t head;                                                             \
    uint32_t len;                                                              \
  } prefix##_queue_t;                                                          \
                                                                               \
  enum {                                                                       \
    prefix##_queue_max_length = max_length,                                    \
    prefix##_queue_entries_per_shard =                                         \
        ((uint32_t)(32 << 10) / sizeof(elem_t)),                               \
    prefix##_queue_shards =                                                    \
        (max_length + prefix##_queue_entries_per_shard - 1) /                  \
        prefix##_queue_entries_per_shard,                                      \
  };                                                                           \
                                                                               \
  typedef struct prefix##_queue_shard {                                        \
    elem_t entries[prefix##_queue_entries_per_shard];                          \
  } prefix##_queue_shard_t;                                                    \
                                                                               \
  struct {                                                                     \
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);                                   \
    __uint(max_entries, prefix##_queue_shards);                                \
    __type(key, uint32_t);                                                     \
    __type(value, prefix##_queue_shard_t);                                     \
  } prefix##_queue_shards_map SEC(".maps");                                    \
                                                                               \
  static elem_t* prefix##_queue_element_at(prefix##_queue_t* queue,            \
                                           uint32_t queue_idx) {               \
    if (!queue) {                                                              \
      return NULL;                                                             \
    }                                                                          \
    uint32_t shard_idx = queue_idx / prefix##_queue_entries_per_shard;         \
    if (shard_idx >= prefix##_queue_shards) {                                  \
      return NULL;                                                             \
    }                                                                          \
    prefix##_queue_shard_t* shard =                                            \
        bpf_map_lookup_elem(&prefix##_queue_shards_map, &shard_idx);           \
    if (!shard) {                                                              \
      return NULL;                                                             \
    }                                                                          \
    barrier_var(shard_idx);                                                    \
    uint32_t entry_idx = queue_idx % prefix##_queue_entries_per_shard;         \
    if (entry_idx >= prefix##_queue_entries_per_shard) {                       \
      return NULL;                                                             \
    }                                                                          \
    return &shard->entries[entry_idx];                                         \
  }                                                                            \
                                                                               \
  static elem_t* prefix##_queue_push_back(prefix##_queue_t* queue) {           \
    if (!queue) {                                                              \
      return NULL;                                                             \
    }                                                                          \
    if (queue->len >= prefix##_queue_max_length) {                             \
      return NULL;                                                             \
    }                                                                          \
    elem_t* ret = prefix##_queue_element_at(                                   \
      queue,                                                                   \
      (queue->head + queue->len) % prefix##_queue_max_length                   \
    );                                                                         \
    queue->len++;                                                              \
    return ret;                                                                \
  }                                                                            \
                                                                               \
  static elem_t* prefix##_queue_push_front(prefix##_queue_t* queue) {          \
    if (!queue) {                                                              \
      return NULL;                                                             \
    }                                                                          \
    if (queue->len >= prefix##_queue_max_length) {                             \
      return NULL;                                                             \
    }                                                                          \
    queue->len++;                                                              \
    if (queue->head == 0) {                                                    \
      queue->head = prefix##_queue_max_length - 1;                             \
    } else {                                                                   \
      queue->head--;                                                           \
    }                                                                          \
    elem_t* ret = prefix##_queue_element_at(                                   \
      queue,                                                                   \
      queue->head                                                              \
    );                                                                         \
    return ret;                                                                \
  }                                                                            \
                                                                               \
  static elem_t* prefix##_queue_pop_front(prefix##_queue_t* queue) {           \
    if (!queue) {                                                              \
      return NULL;                                                             \
    }                                                                          \
    if (queue->len == 0) {                                                     \
      return NULL;                                                             \
    }                                                                          \
    elem_t* ret = prefix##_queue_element_at(queue, queue->head);               \
    queue->head++;                                                             \
    if (queue->head == prefix##_queue_max_length) {                            \
      queue->head = 0;                                                         \
    }                                                                          \
    queue->len--;                                                              \
    return ret;                                                                \
  }

#endif // __QUEUE_H__
