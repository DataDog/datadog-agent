#!/usr/bin/env python3

import subprocess
import sys

clang_format = 'C:\\Program Files (x86)\\Microsoft Visual Studio\\2019\\Community\\vc\\Tools\\Llvm\\bin\\clang-format.exe'

try:
    subprocess.run(f'{clang_format} {",".join(sys.argv[1:])}', shell=True, check=True)
except subprocess.CalledProcessError:
    # Signal failure to pre-commit
    sys.exit(-1)
