import os
from subprocess import check_output

import pytest
from cffi import FFI

HERE = os.path.abspath(os.path.dirname(__file__))
HEADER = os.path.join(HERE, '..', 'include', 'datadog_agent_six.h')
LIB = os.path.join(HERE, '..', 'six', 'libdatadog-agent-six.so')

@pytest.fixture(scope='session')
def lib():
    ffi = FFI()
    out = check_output(['cc', '-E', '-DDATADOG_AGENT_SIX_TEST', HEADER]).decode('utf-8')
    ffi.cdef(out)

    return ffi.dlopen(LIB, ffi.RTLD_LAZY)
