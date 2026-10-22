// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2026-present Datadog, Inc.

#define _GNU_SOURCE
#undef NDEBUG
#include <assert.h>
#include <dlfcn.h>
#include <errno.h>
#include <malloc.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>

typedef int (*mallctl_t)(const char *, void *, size_t *, void *, size_t);

static void executable_definition(const char *name) {
    void *symbol = dlsym(RTLD_DEFAULT, name);
    Dl_info info;
    struct stat binary, definition;
    assert(symbol && dladdr(symbol, &info));
    assert(stat("/proc/self/exe", &binary) == 0);
    assert(stat(info.dli_fname, &definition) == 0);
    if (binary.st_dev != definition.st_dev || binary.st_ino != definition.st_ino) {
        fprintf(stderr, "%s resolved to %s instead of executable\n", name, info.dli_fname);
        abort();
    }
}

static void owned(void *ptr, size_t size) {
    assert(ptr && malloc_usable_size(ptr) >= size);
    mallctl_t control = (mallctl_t)dlsym(RTLD_DEFAULT, "mallctl");
    unsigned arena;
    size_t length = sizeof(arena);
    assert(control && control("arenas.lookup", &arena, &length, &ptr, sizeof(ptr)) == 0);
}

static void exercise_library(const char *path, int mode) {
    void *handle = dlopen(path, RTLD_NOW | mode);
    if (!handle) {
        fprintf(stderr, "dlopen: %s\n", dlerror());
        abort();
    }
    void *(*address)(const char *) = dlsym(handle, "allocator_address");
    void *(*allocate)(size_t) = dlsym(handle, "allocate");
    void *(*zero)(size_t, size_t) = dlsym(handle, "allocate_zero");
    void *(*resize)(void *, size_t) = dlsym(handle, "resize");
    void (*release)(void *) = dlsym(handle, "release");
    size_t (*usable)(void *) = dlsym(handle, "usable");
    void *(*aligned)(size_t, size_t) = dlsym(handle, "allocate_aligned");
    assert(address && allocate && zero && resize && release && usable && aligned);
    const char *names[] = {"malloc", "calloc", "realloc", "free", "posix_memalign",
        "aligned_alloc", "memalign", "valloc", "pvalloc", "malloc_usable_size"};
    for (size_t i = 0; i < sizeof(names) / sizeof(names[0]); i++) {
        executable_definition(names[i]);
        assert(address(names[i]) == dlsym(RTLD_DEFAULT, names[i]));
    }
    unsigned char *ptr = allocate(257);
    owned(ptr, 257);
    memset(ptr, 0x5a, 257);
    ptr = realloc(ptr, 4097);
    owned(ptr, 4097);
    for (size_t i = 0; i < 257; i++) assert(ptr[i] == 0x5a);
    release(ptr);

    ptr = calloc(257, 1);
    assert(usable(ptr) >= 257);
    for (size_t i = 0; i < 257; i++) assert(ptr[i] == 0);
    memset(ptr, 0x3c, 257);
    ptr = resize(ptr, 8193);
    owned(ptr, 8193);
    for (size_t i = 0; i < 257; i++) assert(ptr[i] == 0x3c);
    free(ptr);

    ptr = zero(257, 1);
    owned(ptr, 257);
    for (size_t i = 0; i < 257; i++) assert(ptr[i] == 0);
    free(ptr);
    ptr = aligned(4096, 8192);
    owned(ptr, 8192);
    assert((uintptr_t)ptr % 4096 == 0);
    free(ptr);
    void *aligned_ptr = NULL;
    assert(posix_memalign(&aligned_ptr, 4096, 8193) == 0);
    owned(aligned_ptr, 8193);
    assert((uintptr_t)aligned_ptr % 4096 == 0);
    release(aligned_ptr);
    ptr = memalign(64, 65);
    owned(ptr, 65);
    assert((uintptr_t)ptr % 64 == 0);
    release(ptr);
    ptr = valloc(65);
    owned(ptr, 65);
    assert((uintptr_t)ptr % sysconf(_SC_PAGESIZE) == 0);
    release(ptr);
    void *(*page_allocate)(size_t) = dlsym(RTLD_DEFAULT, "pvalloc");
    ptr = page_allocate(65);
    owned(ptr, (size_t)sysconf(_SC_PAGESIZE));
    release(ptr);
    errno = 0;
    assert(page_allocate(SIZE_MAX) == NULL && errno == ENOMEM);
    void *(*array_resize)(void *, size_t, size_t) = dlsym(RTLD_DEFAULT, "reallocarray");
    ptr = allocate(257);
    memset(ptr, 0x5a, 257);
    assert(array_resize(ptr, SIZE_MAX, 2) == NULL && errno == ENOMEM);
    ptr = array_resize(ptr, 2, 257);
    owned(ptr, 514);
    for (size_t i = 0; i < 257; i++) assert(ptr[i] == 0x5a);
    release(ptr);
    assert(dlclose(handle) == 0);
}

// This probe is preloaded, not an allocator. The test always unsets MALLOC_CONF
// and first runs without any jemalloc preload. The real product then runs its
// normal version/startup path after this constructor returns.
__attribute__((constructor)) static void validate_allocator(void) {
    const char *library = getenv("DD_ALLOCATOR_TEST_LIBRARY");
    assert(library);
    executable_definition("mallctl");
    executable_definition("reallocarray");
    exercise_library(library, RTLD_LOCAL);
    exercise_library(library, RTLD_GLOBAL);
    fputs("native allocator bindings OK (LOCAL, GLOBAL)\n", stderr);
}
