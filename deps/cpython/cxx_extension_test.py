import importlib
import os
import shlex
import subprocess
import sys
import sysconfig


def main(source):
    out_dir = os.environ["TEST_TMPDIR"]
    obj = os.path.join(out_dir, "cxx_extension.o")
    ext = os.path.join(out_dir, "cxx_extension" + sysconfig.get_config_var("EXT_SUFFIX"))

    def var(name):
        return shlex.split(sysconfig.get_config_var(name))

    include = sysconfig.get_paths()["include"]
    subprocess.run(var("CXX") + var("CFLAGS") + var("CCSHARED") + ["-I", include, "-c", source, "-o", obj], check=True)
    subprocess.run(var("LDCXXSHARED") + [obj, "-o", ext], check=True)

    sys.path.insert(0, out_dir)
    module = importlib.import_module("cxx_extension")
    assert module.greet("world") == "hello world"


if __name__ == "__main__":
    main(sys.argv[1])
