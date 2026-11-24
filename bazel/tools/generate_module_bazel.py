"""Generate MODULE.bazel files from overlay directory structure."""

import argparse
import sys
from pathlib import Path
from urllib.parse import urlparse


def make_target_source_map(package, overlay_files):
    files = {}
    package_path = Path(package)
    overlay_dir = Path(package) / "overlay"
    for path in overlay_files:
        if not path.startswith(package):
            raise ValueError(f"'{path}' does not start with '{package}'")
        rel_path = Path(path).relative_to(overlay_dir)
        if rel_path.name == "overlay.BUILD.bazel":
            dest_path = rel_path.parent / "BUILD.bazel"
        else:
            dest_path = rel_path
        overlay_ref = f"//{package_path.as_posix()}:overlay/{rel_path.as_posix()}"
        files[dest_path.as_posix()] = overlay_ref
    return files


def generate_module_bazel(args, files):
    """Generate MODULE.bazel content."""
    lines = [
        """# This file is generated. Do not hand edit.""",
        "",
        """http_archive = use_repo_rule("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")""",
        "",
    ]
    if args.constants:
        lines.extend(
            [
                """constants_repo = use_repo_rule("//bazel/repo:constants_repo.bzl", "constants_repo")""",
                "",
                "constants_repo(",
                f'    name = "{args.module}_constants",',
                "    items = {",
            ]
        )
        for constant in sorted(args.constants):
            key, _, value = constant.partition("=")
            lines.append(f'        "{key}": "{value}",')
        lines.extend(["    },", ")", ""])
    lines.extend(
        [
            "http_archive(",
            f'    name = "{args.module}",',
            "    files = {",
        ]
    )

    # Add files entries
    for path in sorted(files.keys()):
        lines.append(f'        "{path}": "{files[path]}",')

    lines.append("    },")
    if args.patches:
        lines.append(f"    patch_strip = {args.patch_strip},")
        lines.append("    patches = [")
        lines.extend(f'        "{patch}",' for patch in args.patches)
        lines.append("    ],")
    lines.extend(
        [
            f'    sha256 = "{args.sha256}",',
            f'    strip_prefix = "{args.strip_prefix}",',
        ]
    )
    if len(args.urls) == 1:
        lines.append(f'    url = "{args.urls[0]}",')
    else:
        lines.append("    urls = [")
        lines.extend(f'        "{url}",' for url in args.urls)
        lines.append("    ],")
    lines.extend([")", ""])
    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description="Generate MODULE.bazel file from overlay directory structure")
    parser.add_argument("--files", help="File containing the list of files in the overlay")
    parser.add_argument("--module", help="Name of the module")
    parser.add_argument("--package", help="path to this package")
    parser.add_argument("--url", action="append", dest="urls", required=True, help="URLs for http_archive")
    parser.add_argument("--sha256", help="SHA256 hash")
    parser.add_argument("--strip_prefix", help="Strip prefix for archive")
    parser.add_argument("--patch", action="append", dest="patches", default=[], help="Patch label to apply")
    parser.add_argument("--patch_strip", type=int, default=0, help="Leading path components to strip from patches")
    parser.add_argument(
        "--constant", action="append", dest="constants", default=[], help="KEY=VALUE for the constants repo"
    )
    parser.add_argument(
        "--output", nargs="?", default=None, help="Path to write output MODULE.bazel file (default: stdout)"
    )
    args = parser.parse_args()

    # Validate that all URLs have the same basename
    basenames = {Path(urlparse(url).path).name for url in args.urls}
    if len(basenames) > 1:
        parser.error(f"All URLs must have the same basename, found: {basenames}")

    # Get overlay files
    with open(args.files) as inp:
        overlay_files = inp.read().split("\n")
    files = make_target_source_map(args.package, overlay_files)

    # Generate content
    content = generate_module_bazel(args, files)

    # Write output
    if args.output:
        output_path = Path(args.output)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        output_path.write_text(content, newline="\n")
        print(f"Generated {args.output}", file=sys.stderr)
    else:
        sys.stdout.write(content)


if __name__ == "__main__":
    main()
