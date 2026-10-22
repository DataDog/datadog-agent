// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2026-present Datadog, Inc.

#define _GNU_SOURCE
#undef NDEBUG
#include <assert.h>
#include <dlfcn.h>
#include <malloc.h>
#include <stdlib.h>
#include <string.h>
#include "pkg/collector/sharedlibrary/ffi/ffi.h"

// Deliberately compiled against libc, with no jemalloc dependency. Its versioned
// ELF relocations must bind to definitions in the product executable.
void *allocator_address(const char *name) {
#define ADDRESS(symbol) if (strcmp(name, #symbol) == 0) return (void *)symbol
    ADDRESS(malloc);
    ADDRESS(calloc);
    ADDRESS(realloc);
    ADDRESS(free);
    ADDRESS(posix_memalign);
    ADDRESS(aligned_alloc);
    ADDRESS(memalign);
    ADDRESS(valloc);
    ADDRESS(pvalloc);
    ADDRESS(malloc_usable_size);
    return NULL;
}

void *allocate(size_t size) { return malloc(size); }
void *allocate_zero(size_t count, size_t size) { return calloc(count, size); }
void *resize(void *ptr, size_t size) { return realloc(ptr, size); }
void release(void *ptr) { free(ptr); }
size_t usable(void *ptr) { return malloc_usable_size(ptr); }
void *allocate_aligned(size_t alignment, size_t size) { return aligned_alloc(alignment, size); }

// Exercise the real shared-library integration loader, including the strings
// allocated by C.CString on the Go side of its ABI boundary.
void Run(char *check_id, char *init_config, char *instance_config,
         const aggregator_t *aggregator, const char **error) {
    (void)error;
    int (*control)(const char *, void *, size_t *, void *, size_t) = dlsym(RTLD_DEFAULT, "mallctl");
    assert(control);
    char *strings[] = {check_id, init_config, instance_config};
    for (size_t i = 0; i < 3; i++) {
        void *ptr = strings[i];
        unsigned arena;
        size_t length = sizeof(arena);
        assert(malloc_usable_size(ptr) >= strlen(ptr) + 1);
        assert(control("arenas.lookup", &arena, &length, &ptr, sizeof(ptr)) == 0);
    }
    char *ptr = malloc(257);
    assert(ptr);
    memset(ptr, 0x5a, 257);
    ptr = realloc(ptr, 8193);
    assert(ptr && malloc_usable_size(ptr) >= 8193);
    for (size_t i = 0; i < 257; i++) assert(ptr[i] == 0x5a);
    free(ptr);
    aggregator->cb_submit_metric(check_id, DATADOG_AGENT_RTLOADER_GAUGE,
        "allocator.ffi.success", 1, NULL, "", false);
}

const char *Version(void) { return "1.0"; }
