// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2026-present Datadog, Inc.

#include <errno.h>
#include <stdint.h>
#include <stdlib.h>
#include <unistd.h>

// jemalloc 5.3.0 does not provide these libc entry points. In particular,
// glibc's pvalloc can use its private allocator, which cannot be freed by
// jemalloc. Keep these adapters in the executable alongside jemalloc.
void *pvalloc(size_t size) {
    long page_size = sysconf(_SC_PAGESIZE);
    if (page_size <= 0 || size > SIZE_MAX - ((size_t)page_size - 1)) {
        errno = ENOMEM;
        return NULL;
    }
    size_t page = (size_t)page_size;
    size_t rounded = size == 0 ? page : (size + page - 1) / page * page;
    void *ptr = NULL;
    int error = posix_memalign(&ptr, page, rounded);
    if (error != 0) {
        errno = error;
        return NULL;
    }
    return ptr;
}

void *reallocarray(void *ptr, size_t count, size_t size) {
    if (size != 0 && count > SIZE_MAX / size) {
        errno = ENOMEM;
        return NULL;
    }
    return realloc(ptr, count * size);
}
