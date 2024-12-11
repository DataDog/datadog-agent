#ifndef LOCK_CONTENTION_H
#define LOCK_CONTENTION_H

typedef enum {
    HASH_BUCKET_LOCK = 1,
    HASH_PCPU_FREELIST_LOCK,
    HASH_GLOBAL_FREELIST_LOCK,
    PERCPU_LRU_FREELIST_LOCK,
    LRU_GLOBAL_FREELIST_LOCK,
    LRU_PCPU_FREELIST_LOCK,
    RINGBUF_SPINLOCK,
    RINGBUF_WAITQ_SPINLOCK,
} lock_type_t;

struct lock_range {
    unsigned long long addr_start;
    unsigned long long range;
    lock_type_t type;
} __attribute__((packed));

struct contention_data {
    unsigned long long total_time;
    unsigned long long min_time;
    unsigned long long max_time;
    unsigned int count;
    unsigned int flags;
};

typedef struct lock_range lock_range_t;
typedef struct contention_data contention_data_t;

#endif // LOCK_CONTENTION_H
