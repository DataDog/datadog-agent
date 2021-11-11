#!/usr/bin/env python3

import subprocess
import sys
from os.path import dirname, exists, join, relpath


def go_module_for_package(package_path):
    """
    Finds the go module containing the given package, and the package's path
    relative to that module.  This only works for `./`-relative package paths,
    in the current repository.  The returned module does not contain a trailing
    `/` character.  If the package path does not exist, the return value is
    `.`.
    """
    assert package_path.startswith('./')
    module_path = package_path
    while module_path != '.':
        if exists(join(module_path, 'go.mod')):
            break
        module_path = dirname(module_path)
    relative_package = relpath(package_path, start=module_path)
    if relative_package != '.' and not relative_package[0].startswith('./'):
        relative_package = "./" + relative_package
    return module_path, relative_package


def is_go_file(path):
    """Checks if file is a go file from the Agent code."""
    return (path.startswith("pkg") or path.startswith("cmd")) and path.endswith(".go")


# Exclude non go files
go_files = (path for path in sys.argv[1:] if is_go_file(path))

# Get the package for each file
packages = {f'./{dirname(f)}' for f in go_files}
if len(packages) == 0:
    sys.exit()

# separate those by module

by_mod = {}
for package_path in packages:
    mod, pkg = go_module_for_package(package_path)
    by_mod.setdefault(mod, set()).add(pkg)

for module, packages in by_mod.items():
    print(f"vet {packages} in {module}")
    cmd = f"go vet {' '.join(packages)}"

    try:
        subprocess.run(cmd, shell=True, check=True, cwd=module)
    except subprocess.CalledProcessError:
        # Signal failure to pre-commit
        sys.exit(-1)
