#!/bin/env python3

import json
import pathlib
import re
import sys


def postprocess(modules):
    modules["_testcapi"]["extra_files"] = ["Modules/_testcapi_feature_macros.inc"]
    for m in ["_md5", "_sha1", "_sha2", "_sha3"]:
        modules[m]["includes"] = ["Modules/_hacl/include"]
    modules["_bz2"]["deps"] = ["@bzip2//:bz2"]
    modules["_lzma"]["deps"] = ["@xz//:liblzma"]
    modules["_decimal"]["deps"] = [":mpdec"]
    modules["_decimal"]["force_cc_binary"] = "yes"  # boolean causes issues with the json/starlark conversion
    modules["zlib"]["deps"] = ["@zlib//:zlib"]
    del modules["readline"]
    modules["_blake2"]["textual_hdrs"] = [":blake2_hdrs"]
    del modules["_uuid"]
    del modules["_sqlite3"]  # tmp until the target is merged in main
    for expat_module in ["pyexpat", "_elementtree"]:
        modules[expat_module]["extra_files"] = [":libexpat_srcs"]
        modules[expat_module]["textual_hdrs"] = [":libexpat_textual_hdrs"]
        modules[expat_module]["includes"] = ["Modules/expat"]
    del modules["_tkinter"]
    modules["_ctypes"]["deps"] = ["@libffi//:ffi"]
    del modules["_hashlib"]  # tmp until openssl is merged in main
    del modules["_ssl"]  # tmp until openssl is merged in main
    del modules["_curses"]
    del modules["_curses_panel"]


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
