// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// This file implements a set of hooks for memory allocation routines to track
// memory usage.

// This implementation uses a number of non-portable GNU extensions: RTLD_NEXT
// dlsym handle to fetch symbol definitions from linked shared libraries,
// malloc_usable_size() to fetch sizes of allocations and the fact that all
// allocation functions return pointers that can be used with free() and
// malloc_usable_size().

// References:
// https://refspecs.linuxfoundation.org/elf/elf.pdf
// - Section "Shared Object Dependencies" on the order of run-time symbol resolution

#define _GNU_SOURCE 1

#include <dlfcn.h>
#include <malloc.h>
#include <string.h>
#include <stdlib.h>
#include <unistd.h>

#if __GNUC__ > 4
#  include <stdatomic.h>
#  define mallochook_cnt_t atomic_size_t
#  define mallochook_inc(a, v) a += v
#  define mallochook_dec(a, v) a -= v
#  define mallochook_get(a) a
#else
#  define mallochook_cnt_t unsigned long
#  define mallochook_inc(a, v) __sync_fetch_and_add(&a, v)
#  define mallochook_dec(a, v) __sync_fetch_and_sub(&a, v)
#  define mallochook_get(a) __sync_fetch_and_add(&a, 0)
#endif

static void *(*mallochook_malloc)(size_t size);
static void *(*mallochook_calloc)(size_t nmemb, size_t size);
static void *(*mallochook_realloc)(void *ptr, size_t size);
static void *(*mallochook_reallocarray)(void *ptr, size_t nmemb, size_t size);
static void (*mallochook_free)(void *ptr);

static int (*mallochook_posix_memalign)(void **memptr, size_t alignment, size_t size);
static void *(*mallochook_aligned_alloc)(size_t alignment, size_t size);
static void *(*mallochook_valloc)(size_t size);

static void *(*mallochook_memalign)(size_t alignment, size_t size);
static void *(*mallochook_pvalloc)(size_t size);

static mallochook_cnt_t mallochook_heap_inuse;
static mallochook_cnt_t mallochook_heap_alloc;

static void mallochook_track_alloc(void *ptr) {
    if (ptr != NULL) {
        size_t usable = malloc_usable_size(ptr);
        mallochook_inc(mallochook_heap_inuse, usable);
        mallochook_inc(mallochook_heap_alloc, usable);
    }
}

static void mallochook_track_free(void *ptr) {
    if (ptr != NULL) {
        size_t usable = malloc_usable_size(ptr);
        mallochook_dec(mallochook_heap_inuse, usable);
    }
}

static void* mallochook_loadsym(const char *name) {
    dlerror(); // Clear last error, see dlsym man page
    void *ptr = dlsym(RTLD_NEXT, name);
    if (ptr == NULL) {
        char *err = dlerror();
        if (err == NULL) {
            err = "symbol is defined, but null";
        }
        // printf calls malloc, but we may have just failed to load one
        const char *reloc_err = "error patching symbol ";
        // not much we can do about errors or partial writes here, but this
        // keeps the -Werror happy.
        int n = 0;
        n += write(2, reloc_err, strlen(reloc_err));
        n += write(2, name, strlen(name));
        n += write(2, ": ", 2);
        n += write(2, err, strlen(err));
        n += write(2, "\n", 1);
    }
    return ptr;
}

// Temporary calloc implementation to use while we are loading symbols using
// dlsym, which in turn calls calloc. This codepath can handle allocation errors
// and will use a global static buffer instead. mallochook_init will ensure that
// all symbols are resolved during process startup when only one thread is
// running.
static void *mallochook_calloc_stub(size_t nmemb, size_t size) {
    return NULL;
}

static void mallochook_load_all(void) {
    mallochook_calloc = mallochook_calloc_stub;
    mallochook_calloc = mallochook_loadsym("calloc");

    mallochook_malloc = mallochook_loadsym("malloc");
    mallochook_realloc = mallochook_loadsym("realloc");
    mallochook_reallocarray = mallochook_loadsym("reallocarray");
    mallochook_free = mallochook_loadsym("free");

    mallochook_posix_memalign = mallochook_loadsym("posix_memalign");
    mallochook_aligned_alloc = mallochook_loadsym("aligned_alloc");
    mallochook_valloc = mallochook_loadsym("valloc");

    mallochook_memalign = mallochook_loadsym("memalign");
    mallochook_pvalloc = mallochook_loadsym("pvalloc");
}

static void mallochook_ensure_loaded(void) {
    if (mallochook_calloc == NULL) {
        mallochook_load_all();
    }
}

static void mallochook_init(void) __attribute__((constructor));
void mallochook_init(void) {
    mallochook_ensure_loaded();
}

void *malloc(size_t size) {
    mallochook_ensure_loaded();
    void *ptr = mallochook_malloc(size);
    mallochook_track_alloc(ptr);
    return ptr;
}

void *calloc(size_t nmemb, size_t size) {
    mallochook_ensure_loaded();
    void *ptr = mallochook_calloc(nmemb, size);
    mallochook_track_alloc(ptr);
    return ptr;
}

void *realloc(void *ptr, size_t size) {
    mallochook_ensure_loaded();
    mallochook_track_free(ptr);
    ptr = mallochook_realloc(ptr, size);
    mallochook_track_alloc(ptr);
    return ptr;
}

void *reallocarray(void *ptr, size_t nmemb, size_t size) {
    mallochook_ensure_loaded();
    mallochook_track_free(ptr);
    ptr = mallochook_reallocarray(ptr, nmemb, size);
    mallochook_track_alloc(ptr);
    return ptr;
}

void free(void *ptr) {
    mallochook_ensure_loaded();
    mallochook_track_free(ptr);
    mallochook_free(ptr);
}

int posix_memalign(void **memptr, size_t alignment, size_t size) {
    mallochook_ensure_loaded();
    int rc = mallochook_posix_memalign(memptr, alignment, size);
    if (rc == 0) {
        mallochook_track_alloc(*memptr);
    }
    return rc;
}

void *aligned_alloc(size_t alignment, size_t size) {
    mallochook_ensure_loaded();
    void *ptr = mallochook_aligned_alloc(alignment, size);
    mallochook_track_alloc(ptr);
    return ptr;
}

void *valloc(size_t size) {
    mallochook_ensure_loaded();
    void *ptr = mallochook_valloc(size);
    mallochook_track_alloc(ptr);
    return ptr;
}

void *memalign(size_t alignment, size_t size) {
    mallochook_ensure_loaded();
    void *ptr = mallochook_memalign(alignment, size);
    mallochook_track_alloc(ptr);
    return ptr;
}

void *pvalloc(size_t size) {
    mallochook_ensure_loaded();
    void *ptr = mallochook_pvalloc(size);
    mallochook_track_alloc(ptr);
    return ptr;
}

void mallochook_get_stats(size_t *inuse, size_t *alloc) {
    *inuse = mallochook_heap_inuse;
    *alloc = mallochook_heap_alloc;
}
