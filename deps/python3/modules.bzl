#Generated with deps/python3/gen_modules.py /home/hugo.beauzee/dev/cpython/build-native/Makefile deps/python3/modules.bzl
PYTHON_CORE_MODULES_SRCS =[
    "Modules/atexitmodule.c",
    "Modules/faulthandler.c",
    "Modules/posixmodule.c",
    "Modules/signalmodule.c",
    "Modules/_tracemalloc.c",
    "Modules/_suggestions.c",
    "Modules/_codecsmodule.c",
    "Modules/_collectionsmodule.c",
    "Modules/errnomodule.c",
    "Modules/_io/_iomodule.c",
    "Modules/_io/iobase.c",
    "Modules/_io/fileio.c",
    "Modules/_io/bytesio.c",
    "Modules/_io/bufferedio.c",
    "Modules/_io/textio.c",
    "Modules/_io/stringio.c",
    "Modules/itertoolsmodule.c",
    "Modules/_sre/sre.c",
    "Modules/_sysconfig.c",
    "Modules/_threadmodule.c",
    "Modules/timemodule.c",
    "Modules/_typingmodule.c",
    "Modules/_weakref.c",
    "Modules/_abc.c",
    "Modules/_functoolsmodule.c",
    "Modules/_localemodule.c",
    "Modules/_operator.c",
    "Modules/_stat.c",
    "Modules/symtablemodule.c",
    "Modules/pwdmodule.c"
]
PYTHON_MODULES = {
    "array": {
        "srcs": [
            "Modules/arraymodule.c"
        ]
    },
    "_asyncio": {
        "srcs": [
            "Modules/_asynciomodule.c"
        ]
    },
    "_bisect": {
        "srcs": [
            "Modules/_bisectmodule.c"
        ]
    },
    "_contextvars": {
        "srcs": [
            "Modules/_contextvarsmodule.c"
        ]
    },
    "_csv": {
        "srcs": [
            "Modules/_csv.c"
        ]
    },
    "_heapq": {
        "srcs": [
            "Modules/_heapqmodule.c"
        ]
    },
    "_json": {
        "srcs": [
            "Modules/_json.c"
        ]
    },
    "_lsprof": {
        "srcs": [
            "Modules/_lsprof.c",
            "Modules/rotatingtree.c"
        ]
    },
    "_opcode": {
        "srcs": [
            "Modules/_opcode.c"
        ]
    },
    "_pickle": {
        "srcs": [
            "Modules/_pickle.c"
        ]
    },
    "_queue": {
        "srcs": [
            "Modules/_queuemodule.c"
        ]
    },
    "_random": {
        "srcs": [
            "Modules/_randommodule.c"
        ]
    },
    "_struct": {
        "srcs": [
            "Modules/_struct.c"
        ]
    },
    "_interpreters": {
        "srcs": [
            "Modules/_interpretersmodule.c"
        ]
    },
    "_interpchannels": {
        "srcs": [
            "Modules/_interpchannelsmodule.c"
        ]
    },
    "_interpqueues": {
        "srcs": [
            "Modules/_interpqueuesmodule.c"
        ]
    },
    "_zoneinfo": {
        "srcs": [
            "Modules/_zoneinfo.c"
        ]
    },
    "math": {
        "srcs": [
            "Modules/mathmodule.c"
        ]
    },
    "cmath": {
        "srcs": [
            "Modules/cmathmodule.c"
        ]
    },
    "_statistics": {
        "srcs": [
            "Modules/_statisticsmodule.c"
        ]
    },
    "_datetime": {
        "srcs": [
            "Modules/_datetimemodule.c"
        ]
    },
    "_decimal": {
        "srcs": [
            "Modules/_decimal/_decimal.c"
        ],
        "deps": [
            ":mpdec"
        ],
        "force_cc_binary": "yes"
    },
    "binascii": {
        "srcs": [
            "Modules/binascii.c"
        ]
    },
    "_bz2": {
        "srcs": [
            "Modules/_bz2module.c"
        ],
        "deps": [
            "@bzip2//:bz2"
        ]
    },
    "_lzma": {
        "srcs": [
            "Modules/_lzmamodule.c"
        ],
        "deps": [
            "@xz//:liblzma"
        ]
    },
    "zlib": {
        "srcs": [
            "Modules/zlibmodule.c"
        ],
        "deps": [
            "@zlib//:zlib"
        ]
    },
    "_md5": {
        "srcs": [
            "Modules/md5module.c",
            "Modules/_hacl/Hacl_Hash_MD5.c"
        ],
        "includes": [
            "Modules/_hacl/include"
        ]
    },
    "_sha1": {
        "srcs": [
            "Modules/sha1module.c",
            "Modules/_hacl/Hacl_Hash_SHA1.c"
        ],
        "includes": [
            "Modules/_hacl/include"
        ]
    },
    "_sha2": {
        "srcs": [
            "Modules/sha2module.c"
        ],
        "includes": [
            "Modules/_hacl/include"
        ]
    },
    "_sha3": {
        "srcs": [
            "Modules/sha3module.c",
            "Modules/_hacl/Hacl_Hash_SHA3.c"
        ],
        "includes": [
            "Modules/_hacl/include"
        ]
    },
    "_blake2": {
        "srcs": [
            "Modules/_blake2/blake2module.c",
            "Modules/_blake2/blake2b_impl.c",
            "Modules/_blake2/blake2s_impl.c"
        ],
        "textual_hdrs": [
            ":blake2_hdrs"
        ]
    },
    "pyexpat": {
        "srcs": [
            "Modules/pyexpat.c"
        ],
        "extra_files": [
            ":libexpat_srcs"
        ],
        "textual_hdrs": [
            ":libexpat_textual_hdrs"
        ],
        "includes": [
            "Modules/expat"
        ]
    },
    "_elementtree": {
        "srcs": [
            "Modules/_elementtree.c"
        ],
        "extra_files": [
            ":libexpat_srcs"
        ],
        "textual_hdrs": [
            ":libexpat_textual_hdrs"
        ],
        "includes": [
            "Modules/expat"
        ]
    },
    "_codecs_cn": {
        "srcs": [
            "Modules/cjkcodecs/_codecs_cn.c"
        ]
    },
    "_codecs_hk": {
        "srcs": [
            "Modules/cjkcodecs/_codecs_hk.c"
        ]
    },
    "_codecs_iso2022": {
        "srcs": [
            "Modules/cjkcodecs/_codecs_iso2022.c"
        ]
    },
    "_codecs_jp": {
        "srcs": [
            "Modules/cjkcodecs/_codecs_jp.c"
        ]
    },
    "_codecs_kr": {
        "srcs": [
            "Modules/cjkcodecs/_codecs_kr.c"
        ]
    },
    "_codecs_tw": {
        "srcs": [
            "Modules/cjkcodecs/_codecs_tw.c"
        ]
    },
    "_multibytecodec": {
        "srcs": [
            "Modules/cjkcodecs/multibytecodec.c"
        ]
    },
    "unicodedata": {
        "srcs": [
            "Modules/unicodedata.c"
        ]
    },
    "fcntl": {
        "srcs": [
            "Modules/fcntlmodule.c"
        ]
    },
    "grp": {
        "srcs": [
            "Modules/grpmodule.c"
        ]
    },
    "mmap": {
        "srcs": [
            "Modules/mmapmodule.c"
        ]
    },
    "_posixsubprocess": {
        "srcs": [
            "Modules/_posixsubprocess.c"
        ]
    },
    "resource": {
        "srcs": [
            "Modules/resource.c"
        ]
    },
    "select": {
        "srcs": [
            "Modules/selectmodule.c"
        ]
    },
    "_socket": {
        "srcs": [
            "Modules/socketmodule.c"
        ]
    },
    "syslog": {
        "srcs": [
            "Modules/syslogmodule.c"
        ]
    },
    "termios": {
        "srcs": [
            "Modules/termios.c"
        ]
    },
    "_posixshmem": {
        "srcs": [
            "Modules/_multiprocessing/posixshmem.c"
        ]
    },
    "_multiprocessing": {
        "srcs": [
            "Modules/_multiprocessing/multiprocessing.c",
            "Modules/_multiprocessing/semaphore.c"
        ]
    },
    "_ctypes": {
        "srcs": [
            "Modules/_ctypes/_ctypes.c",
            "Modules/_ctypes/callbacks.c",
            "Modules/_ctypes/callproc.c",
            "Modules/_ctypes/stgdict.c",
            "Modules/_ctypes/cfield.c"
        ],
        "deps": [
            "@libffi//:ffi"
        ]
    },
    "xxsubtype": {
        "srcs": [
            "Modules/xxsubtype.c"
        ]
    },
    "_xxtestfuzz": {
        "srcs": [
            "Modules/_xxtestfuzz/_xxtestfuzz.c",
            "Modules/_xxtestfuzz/fuzzer.c"
        ]
    },
    "_testbuffer": {
        "srcs": [
            "Modules/_testbuffer.c"
        ]
    },
    "_testinternalcapi": {
        "srcs": [
            "Modules/_testinternalcapi.c",
            "Modules/_testinternalcapi/test_lock.c",
            "Modules/_testinternalcapi/pytime.c",
            "Modules/_testinternalcapi/set.c",
            "Modules/_testinternalcapi/test_critical_sections.c"
        ]
    },
    "_testcapi": {
        "srcs": [
            "Modules/_testcapimodule.c",
            "Modules/_testcapi/vectorcall.c",
            "Modules/_testcapi/heaptype.c",
            "Modules/_testcapi/abstract.c",
            "Modules/_testcapi/unicode.c",
            "Modules/_testcapi/dict.c",
            "Modules/_testcapi/set.c",
            "Modules/_testcapi/list.c",
            "Modules/_testcapi/tuple.c",
            "Modules/_testcapi/getargs.c",
            "Modules/_testcapi/datetime.c",
            "Modules/_testcapi/docstring.c",
            "Modules/_testcapi/mem.c",
            "Modules/_testcapi/watchers.c",
            "Modules/_testcapi/long.c",
            "Modules/_testcapi/float.c",
            "Modules/_testcapi/complex.c",
            "Modules/_testcapi/numbers.c",
            "Modules/_testcapi/structmember.c",
            "Modules/_testcapi/exceptions.c",
            "Modules/_testcapi/code.c",
            "Modules/_testcapi/buffer.c",
            "Modules/_testcapi/pyatomic.c",
            "Modules/_testcapi/run.c",
            "Modules/_testcapi/file.c",
            "Modules/_testcapi/codec.c",
            "Modules/_testcapi/immortal.c",
            "Modules/_testcapi/gc.c",
            "Modules/_testcapi/hash.c",
            "Modules/_testcapi/time.c",
            "Modules/_testcapi/bytes.c",
            "Modules/_testcapi/object.c",
            "Modules/_testcapi/monitoring.c"
        ],
        "extra_files": [
            "Modules/_testcapi_feature_macros.inc"
        ]
    },
    "_testlimitedcapi": {
        "srcs": [
            "Modules/_testlimitedcapi.c",
            "Modules/_testlimitedcapi/abstract.c",
            "Modules/_testlimitedcapi/bytearray.c",
            "Modules/_testlimitedcapi/bytes.c",
            "Modules/_testlimitedcapi/complex.c",
            "Modules/_testlimitedcapi/dict.c",
            "Modules/_testlimitedcapi/eval.c",
            "Modules/_testlimitedcapi/float.c",
            "Modules/_testlimitedcapi/heaptype_relative.c",
            "Modules/_testlimitedcapi/import.c",
            "Modules/_testlimitedcapi/list.c",
            "Modules/_testlimitedcapi/long.c",
            "Modules/_testlimitedcapi/object.c",
            "Modules/_testlimitedcapi/pyos.c",
            "Modules/_testlimitedcapi/set.c",
            "Modules/_testlimitedcapi/sys.c",
            "Modules/_testlimitedcapi/tuple.c",
            "Modules/_testlimitedcapi/unicode.c",
            "Modules/_testlimitedcapi/vectorcall_limited.c",
            "Modules/_testlimitedcapi/file.c"
        ]
    },
    "_testclinic": {
        "srcs": [
            "Modules/_testclinic.c"
        ]
    },
    "_testclinic_limited": {
        "srcs": [
            "Modules/_testclinic_limited.c"
        ]
    },
    "_testimportmultiple": {
        "srcs": [
            "Modules/_testimportmultiple.c"
        ]
    },
    "_testmultiphase": {
        "srcs": [
            "Modules/_testmultiphase.c"
        ]
    },
    "_testsinglephase": {
        "srcs": [
            "Modules/_testsinglephase.c"
        ]
    },
    "_testexternalinspection": {
        "srcs": [
            "Modules/_testexternalinspection.c"
        ]
    },
    "_ctypes_test": {
        "srcs": [
            "Modules/_ctypes/_ctypes_test.c"
        ]
    },
    "xxlimited": {
        "srcs": [
            "Modules/xxlimited.c"
        ]
    },
    "xxlimited_35": {
        "srcs": [
            "Modules/xxlimited_35.c"
        ]
    },
    "atexit": {
        "srcs": [
            "Modules/atexitmodule.c"
        ]
    },
    "faulthandler": {
        "srcs": [
            "Modules/faulthandler.c"
        ]
    },
    "posix": {
        "srcs": [
            "Modules/posixmodule.c"
        ]
    },
    "_signal": {
        "srcs": [
            "Modules/signalmodule.c"
        ]
    },
    "_tracemalloc": {
        "srcs": [
            "Modules/_tracemalloc.c"
        ]
    },
    "_suggestions": {
        "srcs": [
            "Modules/_suggestions.c"
        ]
    },
    "_codecs": {
        "srcs": [
            "Modules/_codecsmodule.c"
        ]
    },
    "_collections": {
        "srcs": [
            "Modules/_collectionsmodule.c"
        ]
    },
    "errno": {
        "srcs": [
            "Modules/errnomodule.c"
        ]
    },
    "_io": {
        "srcs": [
            "Modules/_io/_iomodule.c",
            "Modules/_io/iobase.c",
            "Modules/_io/fileio.c",
            "Modules/_io/bytesio.c",
            "Modules/_io/bufferedio.c",
            "Modules/_io/textio.c",
            "Modules/_io/stringio.c"
        ]
    },
    "itertools": {
        "srcs": [
            "Modules/itertoolsmodule.c"
        ]
    },
    "_sre": {
        "srcs": [
            "Modules/_sre/sre.c"
        ]
    },
    "_sysconfig": {
        "srcs": [
            "Modules/_sysconfig.c"
        ]
    },
    "_thread": {
        "srcs": [
            "Modules/_threadmodule.c"
        ]
    },
    "time": {
        "srcs": [
            "Modules/timemodule.c"
        ]
    },
    "_typing": {
        "srcs": [
            "Modules/_typingmodule.c"
        ]
    },
    "_weakref": {
        "srcs": [
            "Modules/_weakref.c"
        ]
    },
    "_abc": {
        "srcs": [
            "Modules/_abc.c"
        ]
    },
    "_functools": {
        "srcs": [
            "Modules/_functoolsmodule.c"
        ]
    },
    "_locale": {
        "srcs": [
            "Modules/_localemodule.c"
        ]
    },
    "_operator": {
        "srcs": [
            "Modules/_operator.c"
        ]
    },
    "_stat": {
        "srcs": [
            "Modules/_stat.c"
        ]
    },
    "_symtable": {
        "srcs": [
            "Modules/symtablemodule.c"
        ]
    },
    "pwd": {
        "srcs": [
            "Modules/pwdmodule.c"
        ]
    }
}
PYTHON_FROZEN_MODULES = {
    "abc": {
        "src": "Lib/abc.py",
        "output": "Python/frozen_modules/abc.h"
    },
    "codecs": {
        "src": "Lib/codecs.py",
        "output": "Python/frozen_modules/codecs.h"
    },
    "io": {
        "src": "Lib/io.py",
        "output": "Python/frozen_modules/io.h"
    },
    "_collections_abc": {
        "src": "Lib/_collections_abc.py",
        "output": "Python/frozen_modules/_collections_abc.h"
    },
    "_sitebuiltins": {
        "src": "Lib/_sitebuiltins.py",
        "output": "Python/frozen_modules/_sitebuiltins.h"
    },
    "genericpath": {
        "src": "Lib/genericpath.py",
        "output": "Python/frozen_modules/genericpath.h"
    },
    "ntpath": {
        "src": "Lib/ntpath.py",
        "output": "Python/frozen_modules/ntpath.h"
    },
    "posixpath": {
        "src": "Lib/posixpath.py",
        "output": "Python/frozen_modules/posixpath.h"
    },
    "os": {
        "src": "Lib/os.py",
        "output": "Python/frozen_modules/os.h"
    },
    "site": {
        "src": "Lib/site.py",
        "output": "Python/frozen_modules/site.h"
    },
    "stat": {
        "src": "Lib/stat.py",
        "output": "Python/frozen_modules/stat.h"
    },
    "importlib.util": {
        "src": "Lib/importlib/util.py",
        "output": "Python/frozen_modules/importlib.util.h"
    },
    "importlib.machinery": {
        "src": "Lib/importlib/machinery.py",
        "output": "Python/frozen_modules/importlib.machinery.h"
    },
    "runpy": {
        "src": "Lib/runpy.py",
        "output": "Python/frozen_modules/runpy.h"
    },
    "__hello__": {
        "src": "Lib/__hello__.py",
        "output": "Python/frozen_modules/__hello__.h"
    },
    "__phello__": {
        "src": "Lib/__phello__/__init__.py",
        "output": "Python/frozen_modules/__phello__.h"
    },
    "__phello__.ham": {
        "src": "Lib/__phello__/ham/__init__.py",
        "output": "Python/frozen_modules/__phello__.ham.h"
    },
    "__phello__.ham.eggs": {
        "src": "Lib/__phello__/ham/eggs.py",
        "output": "Python/frozen_modules/__phello__.ham.eggs.h"
    },
    "__phello__.spam": {
        "src": "Lib/__phello__/spam.py",
        "output": "Python/frozen_modules/__phello__.spam.h"
    },
    "frozen_only": {
        "src": "Tools/freeze/flag.py",
        "output": "Python/frozen_modules/frozen_only.h"
    }
}