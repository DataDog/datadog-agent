# (C) Datadog, Inc. 2018-present
# All rights reserved
# Licensed under a 3-clause BSD style license (see LICENSE)
import os
import sys

from datadog_checks.base.utils.platform import Platform

EMBEDDED_DIR = 'embedded'

if Platform.is_windows():
    EMBEDDED_DIR += str(sys.version_info[0])


def get_ca_certs_path():
    """
    Get a path to the trusted certificates of the system
    """
    for f in _get_ca_certs_paths():
        if os.path.exists(f):
            return f
    return None


def _get_ca_certs_paths():
    """
    Get a list of possible paths containing certificates

    Check is installed via pip to:
     * Windows: embedded/lib/site-packages/datadog_checks/http_check
     * Linux: embedded/lib/python2.7/site-packages/datadog_checks/http_check

    Certificate is installed to:
     * embedded/ssl/certs/cacert.pem

    walk up to `embedded`, and back down to ssl/certs to find the certificate file
    """
    ca_certs = []

    embedded_root = os.path.dirname(os.path.abspath(__file__))
    for _ in range(10):
        if os.path.basename(embedded_root) == EMBEDDED_DIR:
            ca_certs.append(os.path.join(embedded_root, 'ssl', 'certs', 'cacert.pem'))
            break
        embedded_root = os.path.dirname(embedded_root)
    else:
        raise OSError(
            'Unable to locate `embedded` directory. Please specify ca_certs in your http yaml configuration file.'
        )

    try:
        import tornado
    except ImportError:
        # if `tornado` is not present, simply ignore its certificates
        pass
    else:
        ca_certs.append(os.path.join(os.path.dirname(tornado.__file__), 'ca-certificates.crt'))

    ca_certs.append('/etc/ssl/certs/ca-certificates.crt')

    return ca_certs
