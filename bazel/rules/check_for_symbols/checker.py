"""Verifies a binary's symbol table contains/omits expected symbol patterns.

Runs `nm` on a binary and checks the resulting symbol list against a set of
"must include" and "must not include" regex patterns, failing (and printing a
diagnostic to stderr) if any expectation is violated.

This generalizes the legacy Ruby `fips_check_binary_for_expected_symbol`
check (omnibus/lib/fips.rb / omnibus/lib/symbols_inspectors.rb), which used
`go tool nm` to confirm that a FIPS-tagged build actually produced a binary
containing the expected cgo symbol -- a successful build is not sufficient
proof that the intended code path was compiled in.
"""

import argparse
import re
import subprocess
import sys


def parse_args(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--nm", required=True, help="Path to the `nm` tool binary.")
    parser.add_argument("--binary", required=True, help="Path to the binary to inspect.")
    parser.add_argument(
        "--must-include",
        action="append",
        default=[],
        help="Regex pattern that must match at least one symbol. May be repeated.",
    )
    parser.add_argument(
        "--must-not-include",
        action="append",
        default=[],
        help="Regex pattern that must not match any symbol. May be repeated.",
    )
    parser.add_argument("--output", required=True, help="Marker file written on success.")
    return parser.parse_args(argv)


def dump_symbols(nm, binary):
    result = subprocess.run(
        [nm, binary],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise RuntimeError("`nm %s` failed (rc=%d):\n%s" % (binary, result.returncode, result.stderr))
    return result.stdout


def check_symbols(symbols, must_include, must_not_include):
    failures = []
    for pattern in must_include:
        if not re.search(pattern, symbols, re.MULTILINE):
            failures.append("expected a symbol matching %r, but none was found in the symbol table" % pattern)
    for pattern in must_not_include:
        if re.search(pattern, symbols, re.MULTILINE):
            failures.append("found a symbol matching %r, but it must not be present" % pattern)
    return failures


def main(argv):
    args = parse_args(argv)

    try:
        symbols = dump_symbols(args.nm, args.binary)
    except RuntimeError as e:
        print(str(e), file=sys.stderr)
        return 1

    failures = check_symbols(symbols, args.must_include, args.must_not_include)
    if failures:
        print(
            "check_for_symbols failed for %s:\n%s" % (args.binary, "\n".join(failures)),
            file=sys.stderr,
        )
        return 1

    with open(args.output, "w") as f:
        f.write("ok\n")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
