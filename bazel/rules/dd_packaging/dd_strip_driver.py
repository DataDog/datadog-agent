#!/usr/bin/env python3
"""Driver for dd_pkg_strip_transform: strips a binary and splits off debug info.

One invocation always produces BOTH the "stripped" and "debug" outputs for a
single input file -- dd_pkg_strip_transform.bzl registers exactly one of
these actions per file and lets its "stripped"/"debug_only" rule instances
pick which declared output they reference, so the actual strip/objcopy/
dsymutil work never runs twice for the same file.

File-type detection is a runtime decision on purpose: Starlark cannot read
file contents during the analysis phase, so this script -- which inspects
actual magic bytes -- is the authoritative source of truth for "is this
really an ELF/Mach-O/PE object file". The Starlark side only applies a cheap
extension/mode-bit heuristic to avoid spawning this action at all for files
that are obviously not object code (configs, licenses, docs); anything that
heuristic lets through but that turns out not to be a recognized binary
format is passed through unchanged here.

Platform semantics (matching omnibus's lib/omnibus/stripper.rb):
  - ELF (Linux):   objcopy --only-keep-debug -> debug-out
                   strip --strip-debug --strip-unneeded (on a copy)
                   objcopy --add-gnu-debuglink=debug-out -> stripped-out
  - Mach-O (macOS): dsymutil -> debug-out (a .dSYM bundle directory)
                   strip -x (in place on a copy) -> stripped-out
  - PE (Windows):   debug-out = the unstripped original (no split-DWARF story
                   for this toolchain, matching omnibus's
                   windows_symbol_stripping_file); stripped-out = strip'd copy.
  - Anything else: passthrough. stripped-out is a copy of the input;
                   debug-out is an empty marker file (there is no debug info
                   to extract from e.g. a shell script or config file).
"""

import argparse
import os
import shutil
import subprocess
import sys

_ELF_MAGIC = b"\x7fELF"
_MACHO_MAGICS = (
    b"\xfe\xed\xfa\xce",  # 32-bit, big-endian
    b"\xce\xfa\xed\xfe",  # 32-bit, little-endian
    b"\xfe\xed\xfa\xcf",  # 64-bit, big-endian
    b"\xcf\xfa\xed\xfe",  # 64-bit, little-endian
    b"\xca\xfe\xba\xbe",  # fat/universal, big-endian
    b"\xbe\xba\xfe\xca",  # fat/universal, little-endian
)
_PE_MAGIC = b"MZ"


def _detect_format(path):
    try:
        with open(path, "rb") as f:
            head = f.read(4)
    except OSError:
        return None
    if head.startswith(_ELF_MAGIC):
        return "elf"
    if head in _MACHO_MAGICS:
        return "macho"
    if head[:2] == _PE_MAGIC:
        return "pe"
    return None


def _run(cmd):
    subprocess.run(cmd, check=True)


def _make_writable(path):
    # Bazel declares action outputs read-only until the action completes
    # (and the source input itself may be non-writable, e.g. a read-only
    # `pkg_files`-collected binary). `copymode` below faithfully carries that
    # bit over, which then makes in-place tools like `strip` fail with
    # "Permission denied" on the copy. Force the write bit before invoking
    # any tool that modifies the copy in place; final permissions are always
    # reset from the original input via `shutil.copymode` afterwards.
    os.chmod(path, os.stat(path).st_mode | 0o200)


def _passthrough(input_path, stripped_out, debug_out, debug_out_is_dir):
    # Reached whenever the input isn't a format this driver recognizes as
    # strippable -- either because it genuinely isn't (a script, a config
    # file that slipped past the Starlark-side heuristic) or because it's an
    # object file for a platform the *current* dd_strip toolchain doesn't
    # know how to strip (e.g. a Linux ELF prebuilt binary being packaged from
    # a macOS host during local iteration, where the toolchain has no
    # objcopy). Either way there is nothing to split off, so ship the input
    # unmodified and leave an empty placeholder as the "debug" artifact.
    # `debug_out`'s declared shape (file vs. directory) is fixed by the
    # Starlark side before the input is ever inspected, based solely on
    # which toolchain is in play -- so this has to honor whichever shape was
    # requested rather than assuming a file.
    shutil.copyfile(input_path, stripped_out)
    shutil.copymode(input_path, stripped_out)
    if debug_out_is_dir:
        os.makedirs(debug_out, exist_ok=True)
    else:
        open(debug_out, "wb").close()


def _strip_elf(input_path, stripped_out, debug_out, strip, objcopy):
    if not objcopy:
        sys.exit("dd_strip_driver: ELF input but no objcopy tool is configured")
    if not strip:
        sys.exit("dd_strip_driver: ELF input but no strip tool is configured")
    tmp_stripped = stripped_out + ".tmp"
    _run([objcopy, "--only-keep-debug", input_path, debug_out])
    shutil.copyfile(input_path, tmp_stripped)
    _make_writable(tmp_stripped)
    _run([strip, "--strip-debug", "--strip-unneeded", tmp_stripped])
    _run([objcopy, "--add-gnu-debuglink=" + debug_out, tmp_stripped, stripped_out])
    shutil.copymode(input_path, stripped_out)
    os.remove(tmp_stripped)


def _strip_macho(input_path, stripped_out, debug_out, strip):
    # dsymutil is invoked by the caller before this script runs on macOS --
    # see the note in dd_pkg_strip_transform.bzl about declare_directory
    # outputs needing to not already exist when dsymutil starts.
    if not strip:
        sys.exit("dd_strip_driver: Mach-O input but no strip tool is configured")
    shutil.copyfile(input_path, stripped_out)
    _make_writable(stripped_out)
    _run([strip, "-x", stripped_out])
    shutil.copymode(input_path, stripped_out)
    _ = debug_out  # already populated by the caller's dsymutil invocation


def _strip_pe(input_path, stripped_out, debug_out, strip):
    shutil.copyfile(input_path, debug_out)
    shutil.copymode(input_path, debug_out)
    shutil.copyfile(input_path, stripped_out)
    if strip:
        _make_writable(stripped_out)
        _run([strip, stripped_out])
    shutil.copymode(input_path, stripped_out)


def main(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True)
    parser.add_argument("--stripped-out", required=True)
    parser.add_argument("--debug-out", required=True)
    parser.add_argument("--debug-out-is-dir", action="store_true")
    parser.add_argument("--strip", default="")
    parser.add_argument("--objcopy", default="")
    parser.add_argument("--dsymutil", default="")
    args = parser.parse_args(argv)

    fmt = _detect_format(args.input)

    if fmt == "macho" and args.dsymutil:
        if not args.debug_out_is_dir:
            sys.exit("dd_strip_driver: Mach-O debug output must be a directory (.dSYM)")
        # Bazel pre-creates declared directories empty; dsymutil wants to
        # create the bundle itself.
        if os.path.isdir(args.debug_out):
            os.rmdir(args.debug_out)
        _run([args.dsymutil, args.input, "-o", args.debug_out])
        _strip_macho(args.input, args.stripped_out, args.debug_out, args.strip)
    elif fmt == "elf" and args.objcopy:
        _strip_elf(args.input, args.stripped_out, args.debug_out, args.strip, args.objcopy)
    elif fmt == "pe":
        _strip_pe(args.input, args.stripped_out, args.debug_out, args.strip)
    else:
        _passthrough(args.input, args.stripped_out, args.debug_out, args.debug_out_is_dir)


if __name__ == "__main__":
    main(sys.argv[1:])
