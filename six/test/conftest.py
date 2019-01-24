import os
import sys
from subprocess import check_output

import pytest
from cffi import FFI

HERE = os.path.abspath(os.path.dirname(__file__))
HEADER = os.path.join(HERE, '..', 'include', 'datadog_agent_six.h')

def _lib_name():
    if sys.platform.startswith('darwin'):
        lib_name = 'libdatadog-agent-six.dylib'
    elif sys.platform.startswith('linux'):
        lib_name = 'libdatadog-agent-six.so'
    else:
        lib_name = ''

    return os.path.join(HERE, '..', 'six', lib_name)

@pytest.fixture(scope='session')
def lib():
    ffi = FFI()
    out = check_output(['cc', '-E', '-DDATADOG_AGENT_SIX_TEST', HEADER]).decode('utf-8')
    ffi.cdef(out)

    return ffi.dlopen(_lib_name(), ffi.RTLD_LAZY)
