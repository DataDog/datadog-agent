"""System-library and per-software allowlists used by the healthcheck.

Ported verbatim from omnibus-ruby (datadog-5.5.0); the upstream names use
"whitelist" terminology, kept here only to mark the source of each entry.
    lib/omnibus/health_check.rb        WHITELIST_LIBS / MAC_WHITELIST_LIBS
    omnibus/config/software/datadog-agent-integrations-py3.rb (whitelist_file lines)

TODO(ABLD-357 follow-up): many of these entries are inherited from upstream
Chef/omnibus and likely no longer apply to the agent's supported platforms
(e.g. libdb-4.5/4.7/4.8, rrdtoolmodule, Solaris-9 libs). Once the bazel
healthcheck rule is wired up and passing, audit each line and remove what is
no longer triggered.
"""

import re

LINUX_ALLOWLIST = [
    re.compile(r"ld-linux"),
    re.compile(r"libc\.so"),
    re.compile(r"libdb-4\.5\.so"),
    re.compile(r"libdb-4\.7\.so"),
    re.compile(r"libdb-4\.8\.so"),
    re.compile(r"libdb-5\.3\.so"),
    re.compile(r"libdl"),
    re.compile(r"libfreebl\d\.so"),
    re.compile(r"libgcc_s\.so"),
    re.compile(r"libm\.so"),
    re.compile(r"libnsl\.so"),
    re.compile(r"libpthread"),
    re.compile(r"libresolv\.so"),
    re.compile(r"librt\.so"),
    re.compile(r"librrd\.so"),
    re.compile(r"libstdc\+\+\.so"),
    re.compile(r"libutil\.so"),
    re.compile(r"linux-vdso.+"),
    re.compile(r"linux-gate\.so"),
    re.compile(r"rrdtoolmodule\.so"),
]

MACOS_ALLOWLIST = [
    re.compile(r"libobjc\.A\.dylib"),
    re.compile(r"libSystem\.B\.dylib"),
    re.compile(r"libgcc_s\.1\.dylib"),
    re.compile(r"CoreFoundation"),
    re.compile(r"CoreServices"),
    re.compile(r"Tcl$"),
    re.compile(r"Cocoa$"),
    re.compile(r"Carbon$"),
    re.compile(r"IOKit$"),
    re.compile(r"Kerberos"),
    re.compile(r"Tk$"),
    re.compile(r"libutil\.dylib"),
    re.compile(r"libffi\.dylib"),
    re.compile(r"libncurses\.5\.4\.dylib"),
    re.compile(r"libiconv"),
    re.compile(r"libstdc\+\+\.6\.dylib"),
    re.compile(r"libc\+\+\.1\.dylib"),
    re.compile(r"libzstd\.1\.dylib"),
    re.compile(r"Security"),
    re.compile(r"SystemConfiguration"),
    re.compile(r"libresolv\.9\.dylib"),
]

# Per-software allowlist patterns carried over from
# omnibus/config/software/datadog-agent-integrations-py3.rb (upstream `whitelist_file`).
#
# In omnibus the python version is interpolated into each path. We compile a
# version-agnostic regex (any python3.NN segment) so this list does not need
# to be touched when bumping Python.
_PY_INTEGRATIONS_ALLOWLIST_TEMPLATES = [
    r"embedded/lib/python3\.\d+/site-packages/\.libsaerospike",
    r"embedded/lib/python3\.\d+/site-packages/aerospike\.libs",
    r"embedded/lib/python3\.\d+/site-packages/psycopg_binary\.libs",
    r"embedded/lib/python3\.\d+/site-packages/pymqi",
]

FILE_ALLOWLIST = [re.compile(p) for p in _PY_INTEGRATIONS_ALLOWLIST_TEMPLATES]
