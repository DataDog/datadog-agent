import ssl
import sys

import _hashlib
from checks import AgentCheck
from cryptography.hazmat.backends import default_backend


class FIPSModeCheck(AgentCheck):
    def check(self, instance):
        self.gauge('e2e.fips_mode', _hashlib.get_fips_mode())
        # _hashlib.get_fips_mode() only tests the fipsmodule.cnf enabled value
        # it doesn't mean that FIPS mode is operating correctly, so we check
        # that the FIPS provider DLL is loaded as well
        if sys.platform == "win32":
            self.gauge('e2e.fips_dll_loaded', _is_fips_dll_loaded(self.log))
        self.gauge('e2e.fips_cryptography', _is_cryptography_fips(self.log))
        self.gauge('e2e.fips_ssl', _is_ssl_fips(self.log))


def _is_fips_dll_loaded(logger):
    import win32api

    # the module is loaded on demand, import _hashlib is enough to load itf
    try:
        handle = win32api.GetModuleHandle("fips.dll")
    except Exception as e:
        logger.exception(e)
        handle = 0
    return handle != 0


def _is_cryptography_fips(logger):
    try:
        default_backend()._enable_fips()
        return 1
    except Exception as e:
        logger.exception(e)
        return 0


def _is_ssl_fips(logger):
    # we expect ssl to throw an exception in FIPS mode whenever we try to set
    # the context cipher list to "MD5"
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    try:
        ctx.set_ciphers("MD5")
        return 0
    except ssl.SSLError as e:
        logger.exception(e)
        return 1
