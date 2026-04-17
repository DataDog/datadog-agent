# AIX: preload libc++abi.a so that __xlcxx_personality_v0 is available
# when pydantic_core's libunwind.a(libunwind.so.1) dependency is resolved.
#
# Background:
#   pydantic_core is a Rust extension module compiled against AIX's libunwind.a.
#   libunwind.so.1 declares __xlcxx_personality_v0 as an undefined symbol,
#   expecting the IBM XLC++ C++ ABI runtime (libc++abi.a) to provide it.
#   Python is compiled with GCC, so libc++abi.a is not loaded at Python startup.
#   Without this preload, "import pydantic" raises:
#     ImportError: rtld: 0712-001 Symbol __xlcxx_personality_v0 was referenced
#     from module /usr/lib/libunwind.a(libunwind.so.1), but a runtime definition
#     of the symbol was not found.
#
# Fix: load libc++abi.a with RTLD_GLOBAL before any checks run so the symbol
# is available to all subsequently loaded shared modules.
import ctypes as _ctypes

try:
    _ctypes.CDLL('/usr/lib/libc++abi.a(libc++abi.so.1)', _ctypes.RTLD_GLOBAL)
except Exception:
    pass  # best-effort; Python still starts even if this fails
