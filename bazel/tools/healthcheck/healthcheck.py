"""CLI entry point for the standalone Datadog Agent install-tree healthcheck.

Reimplementation of the omnibus-ruby healthcheck step (lib/omnibus/health_check.rb)
that can be invoked from Bazel.
Every backend reads each binary's own dependency metadata (DT_NEEDED/RPATH,
LC_LOAD_DYLIB/LC_RPATH, or PE headers) directly from wherever Bazel already
put the file, and resolves candidates against the packaging manifest
(destination path -> source file).

Usage:
    # Direct: point at a manifest file on disk.
    healthcheck --manifest-path /path/to/manifest.json --target-os linux --install-prefix /opt/datadog-agent

    # From a Bazel test: point at a runfiles-resolvable label.
    healthcheck --manifest-rlocation _main/path/to/manifest.json --target-os macos
"""

import argparse
import sys

import manifest as manifest_lib
from result import Failures


def _format_failures(failures: Failures) -> str:
    lines = [f"Healthcheck failed: {len(failures)} bad linkages", ""]
    for f in sorted(failures):
        lines.append(f"  {f.current_library}: {f.dep_name} -> {f.resolved_path}")
    return "\n".join(lines)


def _format_build_info_problems(problems: dict[str, str]) -> str:
    lines = [f"Healthcheck failed: {len(problems)} Go executables with bad build info", ""]
    for executable, problem in sorted(problems.items()):
        lines.append(f"  {executable}: {problem}")
    return "\n".join(lines)


def _format_conflicts(conflicts: dict[str, dict[str, object]]) -> str:
    lines = ["WARNING: overlapping DLL base addresses (FIPS-relevant only):"]
    for lib_name, data in conflicts.items():
        base = data["base"]
        size = data["size"]
        lines.append(f"  {lib_name}: base=0x{base:016x} size=0x{size:x}")
        for c in data["conflicts"]:  # type: ignore[union-attr]
            lines.append(f"    conflicts with {c}")
    return "\n".join(lines)


def _resolve_rlocation(rlocation: str) -> str:
    from python.runfiles import runfiles

    r = runfiles.Create()
    if r is None:
        raise SystemExit("runfiles library could not initialise")
    resolved = r.Rlocation(rlocation)
    if not resolved:
        raise SystemExit(f"manifest not found at rlocation {rlocation!r}")
    return resolved


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description="Datadog Agent install-tree healthcheck")
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument(
        "--manifest-path",
        help="Path to the packaging manifest (destination path -> source file mapping).",
    )
    group.add_argument(
        "--manifest-rlocation",
        help="Runfiles-relative path to the manifest (used from Bazel test runners).",
    )
    parser.add_argument(
        "--target-os",
        required=True,
        choices=["linux", "macos", "windows"],
        help="OS the checked package targets; selects which backend runs.",
    )
    parser.add_argument(
        "--install-prefix",
        help="Configured final install location (e.g. /opt/datadog-agent). Required for "
        "--target-os=linux, to recognise absolute RPATH entries as pointing into the manifest.",
    )
    parser.add_argument(
        "--readelf-path",
        default="readelf",
        help="Path to a readelf binary (--target-os=linux only). Defaults to a $PATH lookup.",
    )
    parser.add_argument(
        "--gobuildinfo-path",
        help="Path to the gobuildinfo helper. When set, Go executables' build info is checked too.",
    )
    args = parser.parse_args(argv)

    if args.target_os == "linux" and not args.install_prefix:
        parser.error("--install-prefix is required for --target-os=linux")

    manifest_path = args.manifest_path or _resolve_rlocation(args.manifest_rlocation)
    manifest = manifest_lib.load(manifest_path)

    ok, exit_code = True, 0
    if args.target_os == "windows":
        import windows

        conflicts = windows.check(manifest)
        if conflicts:
            print(_format_conflicts(conflicts), file=sys.stderr)
            # Warning-only, matches omnibus behavior: never fails the run.
            ok = False
    else:
        if args.target_os == "linux":
            import linux

            failures = linux.check(manifest, args.install_prefix, args.readelf_path)
        else:
            import macos

            failures = macos.check(manifest)
        if failures:
            print(_format_failures(failures), file=sys.stderr)
            ok, exit_code = False, 1

    if args.gobuildinfo_path:
        import gobuildinfo

        problems = gobuildinfo.check(manifest, args.gobuildinfo_path)
        if problems:
            print(_format_build_info_problems(problems), file=sys.stderr)
            ok, exit_code = False, 1

    if ok:
        print("Healthcheck: OK")
    return exit_code


if __name__ == "__main__":
    sys.exit(main())
