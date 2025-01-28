import _hashlib
import win32api
from checks import AgentCheck


class HelloCheck(AgentCheck):
    def check(self, instance):
        self.gauge('e2e.fips_mode', _hashlib.get_fips_mode())
        # _hashlib.get_fips_mode() only tests the fipsmodule.cnf enabled value
        # it doesn't mean that FIPS mode is operating correctly, so we check
        # that the FIPS provider DLL is loaded as well
        self.gauge('e2e.fips_dll_loaded', _is_fips_dll_loaded())


def _is_fips_dll_loaded():
    # the module is loaded on demand, import _hashlib is enough to load itf
    try:
        handle = win32api.GetModuleHandle("fips.dll")
    except Exception:
        handle = 0
    return handle != 0
