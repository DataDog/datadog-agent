"""
Miscellaneous functions, no tasks here
"""
from __future__ import print_function

import os
import platform

import invoke


def bin_name(name):
    """
    Generate platform dependent names for binaries
    """
    if invoke.platform.WINDOWS:
        return "{}.exe".format(name)
    return name


def pkg_config_path():
    """
    Return the complete path to the embedded pkg-config folder
    """
    path = os.path.join(os.path.dirname("."), "pkg-config", platform.system().lower())
    return os.path.abspath(path)
