#!/bin/env python3

import json
import pathlib
import re
import sys


def postprocess(modules):
    modules["_testcapi"]["extra_files"] = ["Modules/_testcapi_feature_macros.inc"]
    modules["pyexpat"]["includes"] = ["Modules/expat"]
    for m in ["_md5", "_sha1", "_sha2", "_sha3"]:
        modules[m]["includes"] = ["Modules/_hacl/include"]
    modules["_bz2"]["deps"] = ["@bzip2//:bz2"]
    modules["_lzma"]["deps"] = ["@xz//:liblzma"]
    modules["_decimal"]["deps"] = [":mpdec"]


def main(argv):
    if len(argv) != 3:
        print(f'usage: {argv[0]} /path/to/python/makefile output.bzl', file=sys.stderr)
        sys.exit(1)
    makefile = sys.argv[1]
    output = sys.argv[2]
    modules = {}
    with open(makefile) as m:
        for line in m.readlines():
            match = re.search(r"^Modules/([\w-]+)\$\(EXT_SUFFIX\):\s+([\w\s\./-]+).*$", line)
            if match is None:
                continue
            module_name = match.group(1)
            sources = [str(pathlib.Path(f).with_suffix('.c')) for f in match.group(2).split(' ')]
            modules[module_name] = {"srcs": sources}

    postprocess(modules)

    with open(output, 'w') as o:
        o.write(f"#Generated with {' '.join(sys.argv)}\n")
        o.write("PYTHON_MODULES = ")
        json.dump(modules, o, indent=4)


if __name__ == "__main__":
    main(sys.argv)
