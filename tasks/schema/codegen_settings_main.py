"""
Bazel entry point: replicates `dda inv schema.codegen`.

Usage: codegen_settings_main.py <output_dir> [constants]

Intended to be called from a Bazel run_binary action. The working directory
must be the Bazel execroot so that workspace-relative paths such as
  pkg/config/schema/yaml/core_schema.yaml
resolve to their declared inputs.

Writes generated *_settings.go files into <output_dir>; with "constants", writes
only pkg/config/setup/constants/generated.go instead.
"""

import sys

from tasks.schema.codegen_init_settings import run_codegen, run_constants_codegen, run_core_constant_codegen
from tasks.schema.merge_schema import resolve_schema


def main():
    if len(sys.argv) not in (2, 3):
        print(f"Usage: {sys.argv[0]} <output_dir> [constants]", file=sys.stderr)
        sys.exit(1)

    output_dir = sys.argv[1]
    only_constants = len(sys.argv) == 3 and sys.argv[2] == "constants"

    # Paths are workspace-relative; they resolve correctly when the cwd is the
    # Bazel execroot, where all declared srcs are available as symlinks.
    core_schema = resolve_schema("pkg/config/schema/yaml/core_schema.yaml")
    system_probe_schema = resolve_schema("pkg/config/schema/yaml/system-probe_schema.yaml")

    if only_constants:
        run_constants_codegen(core_schema, system_probe_schema, output_dir)
        return

    run_codegen(core_schema, output_dir)
    run_codegen(system_probe_schema, output_dir, sysprobe=True)
    run_core_constant_codegen(core_schema, output_dir)


if __name__ == "__main__":
    main()
