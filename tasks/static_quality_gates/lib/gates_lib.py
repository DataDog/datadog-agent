import glob
import os
from types import SimpleNamespace

from invoke.exceptions import Exit

from tasks.libs.common.color import color_message


def argument_extractor(entry_args, **kwargs):
    for key in kwargs.keys():
        kwargs[key] = entry_args[key]
    return SimpleNamespace(**kwargs)


def find_package_path(flavor, package_os, arch):
    package_dir = os.environ['OMNIBUS_PACKAGE_DIR']
    separator = '_' if package_os == 'debian' else '-'
    extension = "deb" if package_os == 'debian' else "rpm"
    glob_pattern = f'{package_dir}/{flavor}{separator}7*{arch}.{extension}'
    package_paths = glob.glob(glob_pattern)
    if len(package_paths) > 1:
        raise Exit(code=1, message=color_message(f"Too many files matching {glob_pattern}: {package_paths}", "red"))
    elif len(package_paths) == 0:
        raise Exit(code=1, message=color_message(f"Couldn't find any file matching {glob_pattern}", "red"))
    return package_paths[0]
