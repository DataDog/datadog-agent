#!/usr/bin/env python3

import os
import subprocess
import sys

if os.name != 'nt':
    # Don't run on Linux
    sys.exit(0)


def find_clang_format(search_dirs):
    for search_dir in search_dirs:
        for root, _, files in os.walk(search_dir):
            for basename in files:
                if basename == 'clang-format.exe':
                    return os.path.join(root, basename)


clang_format_path = os.environ.get('CLANG_FORMAT_PATH')
if clang_format_path is None:
    search_dirs = ['C:/Program Files/Microsoft Visual Studio', 'C:/Program Files (x86)/Microsoft Visual Studio']
    clang_format_path = find_clang_format(search_dirs)

print(clang_format_path)

try:
    subprocess.run(f'"{clang_format_path}" --dry-run --Werror {",".join(sys.argv[1:])}', shell=True, check=True)
except subprocess.CalledProcessError:
    # Signal failure to pre-commit
    sys.exit(-1)
