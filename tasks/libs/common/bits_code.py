"""Opt-in task policy for Bits Code; the marker does not grant credentials."""

import os


def running_in_bits_code():
    return os.environ.get("DD_BITS_CODE") == "1"


def require_host_authentication(operation):
    if running_in_bits_code():
        raise RuntimeError(
            f"{operation} is unavailable in Bits Code tasks. "
            "Use the host coding agent's authenticated operation for this service; "
            "if none is available, run the operation from an authenticated developer environment. "
            "DD_BITS_CODE does not supply API tokens or cloud credentials."
        )
