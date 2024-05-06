#ifndef LOCK_CONTENTION_H
#define LOCK_CONTENTION_H

struct lock_range {
    unsigned long long addr_start;
    unsigned long long range;
};

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
