# AIX: best-effort preload of libc++abi.a with RTLD_GLOBAL.
#
# Background:
#   Some C++ extensions compiled with IBM XLC++ declare __xlcxx_personality_v0
#   as an undefined symbol, expecting the IBM XLC++ ABI runtime (libc++abi.a)
#   to provide it. Python is compiled with GCC and does not load libc++abi.a
#   automatically, so those extensions fail to import without this preload.
#
#   pydantic_core previously triggered this via a dependency on the IBM-provided
#   /usr/lib/libunwind.a(libunwind.so.1), which needs __xlcxx_personality_v0.
#   pydantic_core now uses a bundled libunwind derived from the GCC runtime
#   (libgcc_s.a), which has no libc++abi.a dependency, so pydantic_core no
#   longer requires this preload. It is kept as a best-effort safety net for
#   any other XLC++-compiled extension that might be loaded at runtime.
#
# Outcome: either the load succeeds (libc++abi.a is present, RTLD_GLOBAL makes
# __xlcxx_personality_v0 available to all subsequent modules) or the except
# clause silently continues (file absent — AIX 7.2 TL4 and earlier — and no
# extension that needs it is present, so nothing breaks).
import ctypes as _ctypes

try:
    _ctypes.CDLL('/usr/lib/libc++abi.a(libc++abi.so.1)', _ctypes.RTLD_GLOBAL)
except Exception:
    pass  # best-effort; Python still starts even if this fails
