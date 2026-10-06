"""Dies without returning a value, for procmgr Crashed-state e2e tests."""

import os
import signal
import sys

if sys.platform == "win32":
    import ctypes

    kernel32 = ctypes.windll.kernel32
    # Windows reports a fatal exception code as the process exit code.
    kernel32.TerminateProcess(kernel32.GetCurrentProcess(), 0xC0000005)
else:
    os.kill(os.getpid(), signal.SIGSEGV)
