#!/usr/bin/env python3

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

GO_TAGS = ["test"]


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
        # -find skips listing package dependencies
        # -f {{.Dir}} outputs the absolute dir containing the package
        proc = subprocess.run(
            "go list -find -f '{{.Dir}}' ./...", shell=True, check=True, cwd=module, capture_output=True
        )
    except subprocess.CalledProcessError as e:
        print(e.stderr.decode('utf-8'))
        sys.exit(-1)

    valid_packages = set()
    for line in proc.stdout.decode('utf-8').split('\n'):
        if line:
            relative = relpath(line, module)
            if relative != '.':
                relative = './' + relative
        valid_packages.add(relative)
    for package in packages - valid_packages:
        print(f"Skipping {package} in {module}: not a valid package or all files are excluded by build tags")
        packages.remove(package)

go_tags_arg = '-tags ' + ','.join(GO_TAGS) if GO_TAGS else ''
for module, packages in by_mod.items():
    if not packages:
        continue
    try:
        subprocess.run(f"go vet {go_tags_arg} {' '.join(packages)}", shell=True, check=True, cwd=module)
    except subprocess.CalledProcessError:
        # Signal failure to pre-commit
        sys.exit(-1)
