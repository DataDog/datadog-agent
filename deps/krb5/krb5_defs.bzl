"""Compiler flags shared by the krb5 overlay packages."""

KRB5_COPTS = [
    "-std=gnu11",
    "-Wno-deprecated-declarations",
    "-fmacro-prefix-map=$(BINDIR)/=",
]

KRB5_PROGRAM_LOCAL_DEFINES = [
    "HAVE_CONFIG_H",
    "KRB5_DEPRECATED=1",
    "KRB5_PRIVATE",
]

KRB5_LOCAL_DEFINES = KRB5_PROGRAM_LOCAL_DEFINES + ["SHARED"]
