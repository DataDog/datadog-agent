# Generated library contents

# Header files from source tree
HDRS = [
    "expat_config.h",
    "lib/ascii.h",
    "lib/asciitab.h",
    "lib/expat.h",
    "lib/expat_external.h",
    "lib/iasciitab.h",
    "lib/internal.h",
    "lib/latin1tab.h",
    "lib/nametab.h",
    "lib/siphash.h",
    "lib/utf8tab.h",
    "lib/winconfig.h",
    "lib/xmlrole.h",
    "lib/xmltok.h",
    "lib/xmltok_impl.h",
    "tests/chardata.h",
    "tests/memcheck.h",
    "tests/minicheck.h",
    "tests/structdata.h",
    "xmlwf/codepage.h",
    "xmlwf/filemap.h",
    "xmlwf/xmlfile.h",
    "xmlwf/xmlmime.h",
    "xmlwf/xmltchar.h",
]

# Header files from generated tree (mac_arm)
HDRS_MAC_ARM = [
    "mac_arm/expat_config.h",
]

LIBEXPATINTERNAL_OBJS = [
    "xmlparse.o",
    "xmlrole.o",
    "xmltok.o",
]

# Textual headers (included .c files and other non-.h files)
TEXTUAL_HEADERS = [
    "lib/xmltok_impl.c",
    "lib/xmltok_ns.c",
    "tests/runtests.c",
]

# Complete filename to path mapping
NAME_TO_PATH = {
    "ascii.h": "lib/ascii.h",
    "asciitab.h": "lib/asciitab.h",
    "benchmark.c": "tests/benchmark/benchmark.c",
    "chardata.c": "tests/chardata.c",
    "chardata.h": "tests/chardata.h",
    "codepage.c": "xmlwf/codepage.c",
    "codepage.h": "xmlwf/codepage.h",
    "ct.c": "xmlwf/ct.c",
    "elements.c": "examples/elements.c",
    "expat.h": "lib/expat.h",
    "expat_config.h": "expat_config.h",
    "expat_external.h": "lib/expat_external.h",
    "filemap.h": "xmlwf/filemap.h",
    "iasciitab.h": "lib/iasciitab.h",
    "internal.h": "lib/internal.h",
    "latin1tab.h": "lib/latin1tab.h",
    "memcheck.c": "tests/memcheck.c",
    "memcheck.h": "tests/memcheck.h",
    "minicheck.c": "tests/minicheck.c",
    "minicheck.h": "tests/minicheck.h",
    "nametab.h": "lib/nametab.h",
    "outline.c": "examples/outline.c",
    "readfilemap.c": "xmlwf/readfilemap.c",
    "runtests.c": "tests/runtests.c",
    "siphash.h": "lib/siphash.h",
    "structdata.c": "tests/structdata.c",
    "structdata.h": "tests/structdata.h",
    "unixfilemap.c": "xmlwf/unixfilemap.c",
    "utf8tab.h": "lib/utf8tab.h",
    "win32filemap.c": "xmlwf/win32filemap.c",
    "winconfig.h": "lib/winconfig.h",
    "xml_parse_fuzzer.c": "fuzz/xml_parse_fuzzer.c",
    "xml_parsebuffer_fuzzer.c": "fuzz/xml_parsebuffer_fuzzer.c",
    "xmlfile.c": "xmlwf/xmlfile.c",
    "xmlfile.h": "xmlwf/xmlfile.h",
    "xmlmime.c": "xmlwf/xmlmime.c",
    "xmlmime.h": "xmlwf/xmlmime.h",
    "xmlparse.c": "lib/xmlparse.c",
    "xmlrole.c": "lib/xmlrole.c",
    "xmlrole.h": "lib/xmlrole.h",
    "xmltchar.h": "xmlwf/xmltchar.h",
    "xmltok.c": "lib/xmltok.c",
    "xmltok.h": "lib/xmltok.h",
    "xmltok_impl.c": "lib/xmltok_impl.c",
    "xmltok_impl.h": "lib/xmltok_impl.h",
    "xmltok_ns.c": "lib/xmltok_ns.c",
    "xmlwf.c": "xmlwf/xmlwf.c",
}

# Object file to source file mapping
OBJ_TO_SRC = {
    "xmlparse.o": "lib/xmlparse.c",
    "xmlrole.o": "lib/xmlrole.c",
    "xmltok.o": "lib/xmltok.c",
}
