// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

// Tracking allocations in Python interpreter.
//
// See https://docs.python.org/3/c-api/memory.html#customize-memory-allocators [1]
//
// Python allocates memory using two mechanisms: pymalloc for small,
// short lived allocations, which constitute the bulk of memory usage,
// and RAW allocator for larger chunks of memory. We don't track two
// types of allocations separately, as the distinction exists largely
// inside Python implementation, out of reach for both users and
// module authors.

#include "three.h"

#if __linux__ || _WIN32
#    include <malloc.h>
#elif __APPLE__ || __FreeBSD__
#    include <malloc/malloc.h>
#endif

void Three::initPymemStats()
{
    PyObject_GetArenaAllocator(&_pymallocPrev);
    PyObjectArenaAllocator alloc{
        .ctx = static_cast<void *>(this),
        .alloc = Three::pymallocAllocCb,
        .free = Three::pymallocFreeCb,
    };
    PyObject_SetArenaAllocator(&alloc);

    PyMemAllocatorEx alloc_raw{
        .ctx = static_cast<void *>(this),
        .malloc = Three::pyrawMallocCb,
        .calloc = Three::pyrawCallocCb,
        .realloc = Three::pyrawReallocCb,
        .free = Three::pyrawFreeCb,
    };
    PyMem_SetAllocator(PYMEM_DOMAIN_RAW, &alloc_raw);
}

void Three::getPymemStats(pymem_stats_t &s)
{
    s.inuse = _pymemInuse;
    s.alloc = _pymemAlloc;
}

// Tracking allocations by Pymalloc. Pymalloc is the optimized
// allocator used for small-sized allocations in OBJ and MEM
// domains. These functions track the amount of memory requested by the
// allocator from the OS, not how much is actually used by currently
// reachable python objects (IOW, pymalloc keeps some unused memory
// around internally to speed up allocations).
void *Three::pymallocAlloc(size_t size)
{
    void *ptr = _pymallocPrev.alloc(_pymallocPrev.ctx, size);
    if (ptr != NULL) {
        _pymemInuse += size;
        _pymemAlloc += size;
    }
    return ptr;
}

void Three::pymallocFree(void *ptr, size_t size)
{
    _pymallocPrev.free(_pymallocPrev.ctx, ptr, size);
    _pymemInuse -= size;
}

void *Three::pymallocAllocCb(void *ctx, size_t size)
{
    return static_cast<Three *>(ctx)->pymallocAlloc(size);
}

void Three::pymallocFreeCb(void *ctx, void *ptr, size_t size)
{
    static_cast<Three *>(ctx)->pymallocFree(ptr, size);
}

// Tracking allocations in RAW python domain. This avoids the need to
// track individual pointers by using non-standard functions present
// on all supported platforms that return allocation size for a given
// pointer (see pyrawAllocSize).
//
// This explicitly calls the C allocator instead of adding layer on
// top of the built-in python allocator, to be sure that our pointers
// come from malloc and not some other kind of allocator, and are
// compatible with malloc_usable_size.

static size_t pyrawAllocSize(void *ptr)
{
#if __linux__
    return ::malloc_usable_size(ptr);
#elif _WIN32
    return ::_msize(ptr);
#elif __APPLE__ || __FreeBSD__
    return ::malloc_size(ptr);
#else
    return 0;
#endif
}

void Three::pyrawTrackAlloc(void *ptr)
{
    if (ptr == NULL) {
        return;
    }
    size_t size = pyrawAllocSize(ptr);
    _pymemInuse += size;
    _pymemAlloc += size;
}

void Three::pyrawTrackFree(void *ptr)
{
    if (ptr == NULL) {
        return;
    }
    size_t size = pyrawAllocSize(ptr);
    _pymemInuse -= size;
}

void *Three::pyrawMalloc(size_t size)
{
    // Required by Python, see [1]
    if (size == 0) {
        size = 1;
    }
    void *ptr = ::malloc(size);
    pyrawTrackAlloc(ptr);
    return ptr;
}

void *Three::pyrawCalloc(size_t nelem, size_t elsize)
{
    // Required by Python, see [1]
    if (nelem == 0 || elsize == 0) {
        nelem = 1;
        elsize = 1;
    }
    void *ptr = ::calloc(nelem, elsize);
    pyrawTrackAlloc(ptr);
    return ptr;
}

void *Three::pyrawRealloc(void *ptr, size_t size)
{
    // Required by Python, see [1]
    if (size == 0) {
        size = 1;
    }
    pyrawTrackFree(ptr);
    ptr = ::realloc(ptr, size);
    pyrawTrackAlloc(ptr);
    return ptr;
}

void Three::pyrawFree(void *ptr)
{
    pyrawTrackFree(ptr);
    ::free(ptr);
}

void *Three::pyrawMallocCb(void *ctx, size_t size)
{
    return static_cast<Three *>(ctx)->pyrawMalloc(size);
}

void *Three::pyrawCallocCb(void *ctx, size_t nelem, size_t elsize)
{
    return static_cast<Three *>(ctx)->pyrawCalloc(nelem, elsize);
}

void *Three::pyrawReallocCb(void *ctx, void *ptr, size_t new_size)
{
    return static_cast<Three *>(ctx)->pyrawRealloc(ptr, new_size);
}

void Three::pyrawFreeCb(void *ctx, void *ptr)
{
    static_cast<Three *>(ctx)->pyrawFree(ptr);
}
