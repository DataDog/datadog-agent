"""Install as checks.d/allocator_check.py and run with the real Core Agent.

Enable telemetry.python_memory to exercise rtloader's RAW allocator accounting.
DD_ALLOCATOR_TEST_LIBRARY points at library.so, built against libc.
"""

import ctypes
import os

from datadog_checks.base import AgentCheck


class Allocator(ctypes.Structure):
    _fields_ = [(name, ctypes.c_void_p) for name in ("ctx", "malloc", "calloc", "realloc", "free")]


class PythonMemoryStats(ctypes.Structure):
    _fields_ = [("inuse", ctypes.c_size_t), ("alloc", ctypes.c_size_t)]


class SymbolInfo(ctypes.Structure):
    _fields_ = [
        ("filename", ctypes.c_char_p),
        ("base", ctypes.c_void_p),
        ("name", ctypes.c_char_p),
        ("address", ctypes.c_void_p),
    ]


class AllocatorCheck(AgentCheck):
    def check(self, instance):
        process = ctypes.CDLL(None)
        process.mallctl.argtypes = [
            ctypes.c_char_p,
            ctypes.c_void_p,
            ctypes.POINTER(ctypes.c_size_t),
            ctypes.c_void_p,
            ctypes.c_size_t,
        ]
        process.malloc_usable_size.argtypes = [ctypes.c_void_p]
        process.malloc_usable_size.restype = ctypes.c_size_t
        python = ctypes.pythonapi
        python.PyMem_RawMalloc.argtypes = [ctypes.c_size_t]
        python.PyMem_RawMalloc.restype = ctypes.c_void_p
        python.PyMem_RawRealloc.argtypes = [ctypes.c_void_p, ctypes.c_size_t]
        python.PyMem_RawRealloc.restype = ctypes.c_void_p
        python.PyMem_RawFree.argtypes = [ctypes.c_void_p]
        python.PyMem_GetAllocator.argtypes = [ctypes.c_int, ctypes.POINTER(Allocator)]
        raw, objects = Allocator(), Allocator()
        python.PyMem_GetAllocator(0, ctypes.byref(raw))
        python.PyMem_GetAllocator(2, ctypes.byref(objects))
        info = SymbolInfo()
        process.dladdr.argtypes = [ctypes.c_void_p, ctypes.POINTER(SymbolInfo)]
        assert process.dladdr(raw.malloc, ctypes.byref(info))
        assert b"libdatadog-agent-three" in info.filename, "enable telemetry.python_memory"
        assert raw.ctx and raw.malloc != objects.malloc
        assert process.dladdr(objects.malloc, ctypes.byref(info))
        assert b"libpython" in info.filename, "Python's small-object allocator must remain installed"
        # Three::initPymemStats installs its RtLoader instance as the RAW context.
        # Query the actual rtloader API, using the structure in rtloader_types.h.
        process.get_pymem_stats.argtypes = [ctypes.c_void_p, ctypes.POINTER(PythonMemoryStats)]
        before, after = PythonMemoryStats(), PythonMemoryStats()
        process.get_pymem_stats(raw.ctx, ctypes.byref(before))

        def owned(pointer, size):
            assert pointer and process.malloc_usable_size(pointer) >= size
            arena = ctypes.c_uint()
            length = ctypes.c_size_t(ctypes.sizeof(arena))
            ptr = ctypes.c_void_p(pointer)
            assert (
                process.mallctl(
                    b"arenas.lookup", ctypes.byref(arena), ctypes.byref(length), ctypes.byref(ptr), ctypes.sizeof(ptr)
                )
                == 0
            )

        # RtLoader's factory constructs Three with C++ new. Its RAW context is
        # that same object, so this also checks libstdc++ allocation interposition.
        owned(raw.ctx, 1)
        ptr = python.PyMem_RawMalloc(257)
        owned(ptr, 257)
        ctypes.memset(ptr, 0x5A, 257)
        ptr = python.PyMem_RawRealloc(ptr, 8193)
        owned(ptr, 8193)
        assert ctypes.string_at(ptr, 257) == b"\x5a" * 257
        python.PyMem_RawFree(ptr)
        process.get_pymem_stats(raw.ctx, ctypes.byref(after))
        assert after.alloc >= before.alloc + 257 + 8193
        assert after.inuse < before.inuse + 1024 * 1024, "RAW free accounting leaked or underflowed"

        # Existing native extensions are loaded both locally and globally.
        for mode in (ctypes.RTLD_LOCAL, ctypes.RTLD_GLOBAL):
            library = ctypes.CDLL(os.environ["DD_ALLOCATOR_TEST_LIBRARY"], mode=mode)
            library.allocate.argtypes = [ctypes.c_size_t]
            library.allocate.restype = ctypes.c_void_p
            library.release.argtypes = [ctypes.c_void_p]
            ptr = library.allocate(8193)
            owned(ptr, 8193)
            library.release(ptr)
        self.gauge("allocator.binding.success", 1)
