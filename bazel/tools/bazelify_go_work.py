"""Generate "bazelified" go.work.

The migration from Go toolchain to Gazelle is incremental: modules not yet converted are listed as `# gazelle:exclude`
in BUILD.bazel so that Gazelle ignores them. However, rules_go is unaware of those exclusions and would pull all go.work
entries into go_deps on every `bazel mod tidy`, causing unwanted churn for unconverted modules.

This script bridges the gap by producing a filtered copy of go.work that only retains `use` entries for
already-converted modules (i.e. those NOT covered by a `# gazelle:exclude` directive, with `.` always excluded). The
result is checked in as .bazelified.go.work and used as the go_deps.from_file source.

This file and its generated output are temporary and will be removed once all modules have been migrated to Gazelle.
"""

import argparse
import re


def _filter_lines(build_file, go_work):
    exclusions = {"."} | {m.group(1) for line in build_file if (m := re.search(r"# gazelle:exclude (\S+)", line))}

    def _is_excluded(path):
        return path in exclusions or any(path.startswith(exclusion + "/") for exclusion in exclusions)

    in_use_block = 0
    for line in go_work:
        stripped = line.strip()
        if stripped == "use (":
            in_use_block += 1
        elif stripped == ")":
            in_use_block -= 1
        elif in_use_block and stripped and not stripped.startswith("//") and _is_excluded(stripped):
            continue
        yield line


def main():
    """See module docstring."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--build-file", required=True, type=argparse.FileType("r"))
    parser.add_argument("--go-work", required=True, type=argparse.FileType("r"))
    parser.add_argument("--output", required=True, type=argparse.FileType("w"))
    args = parser.parse_args()
    args.output.writelines(_filter_lines(args.build_file, args.go_work))


if __name__ == "__main__":
    main()
