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
    modules["_bz2"]["dynamic_deps"] = ["@bzip2//:libbz2_so"]
    modules["_lzma"]["deps"] = ["@xz//:liblzma"]
    modules["_lzma"]["dynamic_deps"] = ["@xz//:lzma"]
    modules["_decimal"]["deps"] = [":mpdec"]
    modules["zlib"]["deps"] = ["@zlib//:zlib"]
    modules["zlib"]["dynamic_deps"] = ["@zlib//:z"]
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
    modules["_hashlib"]["deps"] = ["@openssl//:openssl"]
    # modules["_hashlib"]["dynamic_deps"] = ["@openssl//:openssl"]
    modules["_ssl"]["deps"] = ["@openssl//:openssl"]
    # modules["_ssl"]["dynamic_deps"] = ["@openssl//:openssl"]
    modules["_ssl"]["textual_hdrs"] = ["Modules/_ssl/debughelpers.c", "Modules/_ssl/misc.c", "Modules/_ssl/cert.c"]
    del modules["_curses"]
    del modules["_curses_panel"]


def read_makefile(makefile: str):
    core_modules = []
    modules = {}
    frozen_modules = {}
    with open(makefile) as m:
        lines = m.readlines()
    for idx in range(len(lines)):
        line = lines[idx]
        if line.startswith('MODOBJS='):
            objects = line[len('MODOBJS=') :].strip().split()
            core_modules = [str(pathlib.Path(o).with_suffix('.c')) for o in objects]
        # We are looking for lines such as these:
        # Python/frozen_modules/abc.h: Lib/abc.py $(FREEZE_MODULE_DEPS)
        #     $(FREEZE_MODULE) abc $(srcdir)/Lib/abc.py Python/frozen_modules/abc.h
        # with $(FREEZE_MODULE) being the tool built early in the build process
        # abc is the module name
        # abc.py is the input file to be frozen
        # abc.h is the output file containing the frozen module.
        # The FREEZE_MODULE parameters are positional so we can easily fetch them
        # by spliti the command line
        elif line.rstrip().endswith('$(FREEZE_MODULE_DEPS)'):
            idx += 1
            rule = lines[idx]
            params = rule.split()
            src = params[2].replace('$(srcdir)/', '')
            frozen_modules[params[1]] = {"src": src, "output": params[3]}
        else:
            match = re.search(r"^Modules/([\w-]+)\$\(EXT_SUFFIX\):\s+([\w\s\./-]+).*$", line)
            if match is None:
                continue
            module_name = match.group(1)
            sources = [str(pathlib.Path(f).with_suffix('.c')) for f in match.group(2).split(' ')]
            modules[module_name] = {"srcs": sources}

    postprocess(modules)
    return core_modules, modules, frozen_modules


def main(argv):
    if len(argv) != 3:
        print(f'usage: {argv[0]} /path/to/python/makefile output.bzl', file=sys.stderr)
        sys.exit(1)
    makefile = sys.argv[1]
    output = sys.argv[2]
    core_modules, modules, frozen_modules = read_makefile(makefile)

    with open(output, 'w') as o:
        o.write(f"#Generated with {' '.join(sys.argv)}\n")
        o.write("PYTHON_CORE_MODULES_SRCS =")
        json.dump(core_modules, o, indent=4)
        o.write("\nPYTHON_MODULES = ")
        json.dump(modules, o, indent=4)
        o.write("\nPYTHON_FROZEN_MODULES = ")
        json.dump(frozen_modules, o, indent=4)


if __name__ == "__main__":
    main(sys.argv)
