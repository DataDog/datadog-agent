#!/usr/bin/env python3

import re
import subprocess
import sys
from os.path import dirname, exists, join, relpath

# modules -> packages that should not be vetted, each with a reason
EXCLUDED_PACKAGES = {
    '.': {
        './pkg/ebpf/compiler': "requires C libraries not available everywhere",
        './cmd/py-launcher': "requires building rtloader",
        './pkg/collector/python': "requires building rtloader",
    },
}


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
    module, package = go_module_for_package(package_path)
    reason = EXCLUDED_PACKAGES.get(module, {}).get(package, None)
    if reason:
        print(f"Skipping {package} in {module}: {reason}")
        continue
    by_mod.setdefault(module, set()).add(package)

# now, for each module, we use 'go list' to list all of the *valid* packages
# (those with at least one .go file included by the current build tags), and
# use that to skip packages that do not have any files included, which will
# otherwise cause go vet to fail.
for module, packages in by_mod.items():
    try:
        proc = subprocess.run("go list ./...", shell=True, check=True, cwd=module, capture_output=True)
    except subprocess.CalledProcessError as e:
        print(e.stderr.decode('utf-8'))
        sys.exit(-1)

    mod_re = re.compile(('github.com/DataDog/datadog-agent/' + module.lstrip('./')).rstrip('/') + '/')
    valid_packages = set(mod_re.sub('./', line.strip()) for line in proc.stdout.decode('utf-8').split('\n'))
    for package in packages - valid_packages:
        print(f"Skipping {package} in {module}: build constraints exclude all Go files")
        packages.remove(package)

for module, packages in by_mod.items():
    try:
        subprocess.run(f"go vet {' '.join(packages)}", shell=True, check=True, cwd=module)
    except subprocess.CalledProcessError:
        # Signal failure to pre-commit
        sys.exit(-1)
