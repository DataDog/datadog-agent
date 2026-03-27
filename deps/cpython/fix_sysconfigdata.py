"""Fix sandbox-absolute tool paths in Python's _sysconfigdata__*.py.

Replaces tool paths recorded by CPython's build system with just the
tool's basename (stripping Bazel sandbox prefixes), and clears build
flags that are meaningless outside of Bazel.

The resulting file will lose original formatting and comments, but this
is acceptable given the file's usage.
"""

import argparse
import ast
import os
import re
import sys

# Names of tool to strip to their basenames
TOOL_BASENAMES = frozenset(
    [
        "ar",
        "gcc",
        "g++",
        "ld",
    ]
)

# Build-environment flags that are meaningless outside of Bazel
FLAGS_TO_CLEAR = frozenset(
    [
        "CFLAGS",
        "CPPFLAGS",
        "CXXFLAGS",
        "LDFLAGS",
    ]
)

# Regexp to split into alternating path candidates and other tokens
# Needs to account for paths being preceded by `=` or quotes.
_PATH_SPLITTER_RE = re.compile(r"""(/[^\s='"]+)""")


def _fix_tool_path(segment):
    basename = os.path.basename(segment)
    return basename if basename in TOOL_BASENAMES else segment


def _fix_value(key, value):
    if key in FLAGS_TO_CLEAR:
        return ""
    if isinstance(value, str):
        parts = _PATH_SPLITTER_RE.split(value)
        # The split leaves path-candidates (non-separators) at odd-indexed positions
        return "".join(_fix_tool_path(p) if i % 2 == 1 else p for i, p in enumerate(parts))
    return value


def fix_file(path):
    with open(path) as f:
        source = f.read()

    tree = ast.parse(source)

    # Walk the AST and edit the "build_time_vars" dictionary in-place with
    # the "fixed" values.
    for node in ast.walk(tree):
        if isinstance(node, ast.Assign) and any(
            isinstance(t, ast.Name) and t.id == "build_time_vars" for t in node.targets
        ):
            build_time_vars = ast.literal_eval(node.value)
            fixed = {k: _fix_value(k, v) for k, v in build_time_vars.items()}
            node.value = ast.Dict(
                keys=[ast.Constant(value=k) for k in fixed],
                values=[ast.Constant(value=v) for v in fixed.values()],
            )
            break
    else:
        sys.exit(f"error: build_time_vars not found in {path}")

    with open(path, "w") as f:
        f.write(ast.unparse(tree) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("files", nargs="+", metavar="FILE", help="_sysconfigdata__*.py file(s) to fix")
    args = parser.parse_args()
    for path in args.files:
        fix_file(path)


if __name__ == "__main__":
    main()
