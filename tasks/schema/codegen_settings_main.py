"""
Bazel entry point: replicates `dda inv schema.codegen`.

Usage: codegen_settings_main.py <output_dir>

Intended to be called from a Bazel run_binary action. The working directory
must be the Bazel execroot so that workspace-relative paths such as
  pkg/config/schema/yaml/core_schema.yaml
resolve to their declared inputs.

Writes generated *_settings.go files into <output_dir>.  The caller
(run_binary) must declare those files as outputs in its `outs` list.
"""

import sys

from tasks.schema.codegen_init_settings import run_codegen, run_constant_codegen
from tasks.schema.merge_schema import resolve_schema


def _filter(expect, filename):
    """Return a predicate that accepts (expect=True) or rejects (expect=False) filename."""

    def comparator(othername):
        return (filename == othername) == expect

    return comparator


def main():
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <output_dir>", file=sys.stderr)
        sys.exit(1)

    output_dir = sys.argv[1]

    # Paths are workspace-relative; they resolve correctly when the cwd is the
    # Bazel execroot, where all declared srcs are available as symlinks.
    core_schema = resolve_schema("pkg/config/schema/yaml/core_schema.yaml")
    system_probe_schema = resolve_schema("pkg/config/schema/yaml/system-probe_schema.yaml")

    # Core agent settings: every file except system_probe_settings.go
    run_codegen(core_schema, _filter(False, "system_probe_settings.go"), output_dir)
    # System-probe settings: only system_probe_settings.go
    run_codegen(system_probe_schema, _filter(True, "system_probe_settings.go"), output_dir)
    run_constant_codegen(core_schema, system_probe_schema, output_dir)


if __name__ == "__main__":
    main()
