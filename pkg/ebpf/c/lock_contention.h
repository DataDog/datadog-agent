#ifndef LOCK_CONTENTION_H
#define LOCK_CONTENTION_H

struct lock_range {
    unsigned long long addr_start;
    unsigned long long range;
};

typedef struct lock_range lock_range_t;

#endif // LOCK_CONTENTION_H
