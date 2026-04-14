"""Fix sandbox-absolute tool paths in Python's _sysconfigdata__*.py.

- Replaces tool paths recorded by CPython's build system with just the
tool's basename (stripping Bazel sandbox prefixes)
- Completely clears a set of predetermined fields
- Replaces known sandbox paths as well as paths with references to execroot (bazel-out)
  to use a predetermined replacement prefix.

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

# Subdirectory suffixes we can confidently map to the same suffix under the install prefix.
_KNOWN_SUFFIXES = frozenset(["lib", "lib64", "include"])

# Regexp to split into alternating path candidates and other tokens
# Needs to account for paths being preceded by `=` or quotes.
_PATH_SPLITTER_RE = re.compile(r"""(/[^\s='"]+)""")


def _fix_tool_path(segment):
    basename = os.path.basename(segment)
    return basename if basename in TOOL_BASENAMES else segment


def _fix_bazel_out_path(segment, install_prefix):
    """Best-effort: collapse bazel-out sandbox paths to the install prefix.

    Paths containing bazel-out are Bazel build artefacts that don't exist at
    install time. If the path ends with a known suffix (lib, include, ...) we
    keep that suffix; otherwise we map the whole path to the install prefix.
    """
    if "bazel-out" not in segment:
        return segment
    basename = os.path.basename(segment)
    if basename in _KNOWN_SUFFIXES:
        return install_prefix + "/" + basename
    return install_prefix


def _fix_value(key, value, install_prefix, sandbox_prefix):
    if key in FLAGS_TO_CLEAR:
        return ""
    if isinstance(value, str):
        # Fix tool paths to only refer to base names.
        # The RE-based split leaves path-candidates (non-separators) at odd-indexed positions.
        result = "".join(_fix_tool_path(p) if i % 2 == 1 else p for i, p in enumerate(_PATH_SPLITTER_RE.split(value)))
        # Replace known sandbox install prefix with the real install prefix.
        result = result.replace(sandbox_prefix, install_prefix)
        # Collapse remaining bazel-out paths to the install prefix.
        result = "".join(
            _fix_bazel_out_path(p, install_prefix) if i % 2 == 1 else p
            for i, p in enumerate(_PATH_SPLITTER_RE.split(result))
        )
        return result
    return value


def fix_file(path, install_prefix, sandbox_prefix):
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
            fixed = {k: _fix_value(k, v, install_prefix, sandbox_prefix) for k, v in build_time_vars.items()}
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
    parser.add_argument(
        "--install-prefix",
        metavar="DIR",
        required=True,
        help="Install prefix to substitute for sandbox paths (e.g. ##PREFIX##)",
    )
    parser.add_argument(
        "--sandbox-prefix",
        metavar="DIR",
        required=True,
        help="Bazel sandbox install root",
    )
    parser.add_argument("files", nargs="+", metavar="FILE", help="_sysconfigdata__*.py file(s) to fix")
    args = parser.parse_args()
    for path in args.files:
        fix_file(path, install_prefix=args.install_prefix, sandbox_prefix=args.sandbox_prefix)


if __name__ == "__main__":
    main()
