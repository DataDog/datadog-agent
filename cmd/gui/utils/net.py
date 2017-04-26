# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# lib
import ctypes
import time
import random
import socket


# 3p

# project


# Due to this bug [0] CPython on certain Windows version does not
# define some IPv6 related constants that we need to hardcode
# here.
#
# [0] http://bugs.python.org/issue6926
try:
    IPPROTO_IPV6 = socket.IPPROTO_IPV6
except AttributeError:
    IPPROTO_IPV6 = 41  # from `Ws2def.h`

try:
    IPV6_V6ONLY = socket.IPV6_V6ONLY
except AttributeError:
    IPV6_V6ONLY = 27  # from `Ws2ipdef.h`

DEFAULT_DNS_TTL = 300

class sockaddr(ctypes.Structure):
    _fields_ = [("sa_family", ctypes.c_short),
                ("__pad1", ctypes.c_ushort),
                ("ipv4_addr", ctypes.c_byte * 4),
                ("ipv6_addr", ctypes.c_byte * 16),
                ("__pad2", ctypes.c_ulong)]

class DNSCache(object):
    """
    Simple, rudimentary DNS cache
    """
    def __init__(self, ttl=DEFAULT_DNS_TTL):
        self._cache = {}
        self._ttl = ttl
        random.seed()

    def resolve(self, url):
        try:
            ts, entry = self._cache.get(url, (None, None))
            now = int(time.time())
            if not ts or ts < now:
                _, _, entry = socket.gethostbyaddr(url)
                ttl = now + self._ttl
                self._cache[url] = ttl, entry

            resolve = entry[random.randint(0, len(entry)-1)]
        except Exception:
            resolve = url

        return resolve

def _inet_pton_win(address_family, ip_string):
    """
    Window specific version of `inet_pton` based on:
    https://gist.github.com/nnemkin/4966028
    """
    addr = sockaddr()
    addr.sa_family = address_family
    addr_size = ctypes.c_int(ctypes.sizeof(addr))

    str_to_addr = ctypes.windll.ws2_32.WSAStringToAddressA

    if str_to_addr(ip_string, address_family, None, ctypes.byref(addr), ctypes.byref(addr_size)) != 0:
        raise socket.error(ctypes.FormatError())

    if address_family == socket.AF_INET:
        return ctypes.string_at(addr.ipv4_addr, 4)
    if address_family == socket.AF_INET6:
        return ctypes.string_at(addr.ipv6_addr, 16)

    raise socket.error('unknown address family')


try:
    from socket import inet_pton
except ImportError:
    inet_pton = _inet_pton_win
