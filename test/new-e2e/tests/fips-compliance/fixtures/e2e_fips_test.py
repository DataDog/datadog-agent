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
        if sys.platform.startswith("linux"):
            self.gauge('e2e.fips_confluent_kafka', _is_confluent_kafka_fips(self.log))
            self.gauge('e2e.fips_aerospike', _is_aerospike_fips(self.log))


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


def _is_confluent_kafka_fips(logger):
    # librdkafka creates its TLS context when the client is created. In FIPS mode the default
    # context must work and the "MD5" cipher list must be rejected. A copy of OpenSSL other than
    # the Agent's would either accept MD5 or fail to create any TLS context.
    from confluent_kafka import KafkaException, Producer

    try:
        Producer({'security.protocol': 'SSL'})
    except KafkaException as e:
        logger.exception(e)
        return 0
    try:
        Producer({'security.protocol': 'SSL', 'ssl.cipher.suites': 'MD5'})
        return 0
    except KafkaException as e:
        logger.exception(e)
        return 1


def _is_aerospike_fips(logger):
    # The aerospike client creates its TLS context before connecting. Nothing listens on port 1, so
    # the default context must fail with a connection error, and the "MD5" cipher list with a TLS error.
    import aerospike
    from aerospike import exception

    def connect(tls):
        aerospike.client({'hosts': [('localhost', 1)], 'tls': tls}).connect()

    try:
        connect({'enable': True})
        return 0
    except exception.ConnectionError:
        pass
    except exception.AerospikeError as e:
        logger.exception(e)
        return 0
    try:
        connect({'enable': True, 'cipher_suite': 'MD5'})
        return 0
    except exception.TLSError as e:
        logger.exception(e)
        return 1
    except exception.AerospikeError as e:
        logger.exception(e)
        return 0
