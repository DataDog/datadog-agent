"""Generate MODULE.bazel files from overlay directory structure."""

import argparse
import sys
from pathlib import Path


def make_target_source_map(package, overlay_files):
    files = {}
    package_path = Path(package)
    overlay_dir = Path(package) / "overlay"
    for path in overlay_files:
        assert path.startswith(package)
        rel_path = Path(path).relative_to(overlay_dir)
        if rel_path.name == "overlay.BUILD.bazel":
            dest_path = str(rel_path.parent / "BUILD.bazel")
        else:
            dest_path = str(rel_path)
        overlay_ref = f"//{package_path}:overlay/{rel_path}"
        files[dest_path] = overlay_ref
    return files


def generate_module_bazel(args, files):
    """Generate MODULE.bazel content."""
    lines = [
        """# This file is generated do not hand edit.""",
        "",
        """http_archive = use_repo_rule("//third_party/bazel/tools/build_defs/repo:http.bzl", "http_archive")""",
        "",
        "http_archive(",
        f'    name = "{args.module}",',
        "    files = {",
    ]

    # Add files entries
    for path in sorted(files.keys()):
        lines.append(f'        "{path}": "{files[path]}",')

    lines.extend(
        [
            "    },",
            f'    sha256 = "{args.sha256}",',
            f'    strip_prefix = "{args.strip_prefix}",',
            f'    url = "{args.url}",',
            ")",
            "",
        ]
    )
    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description="Generate MODULE.bazel file from overlay directory structure")
    parser.add_argument("--files", help="File containaing the list of files in the overlay")
    parser.add_argument("--module", help="Name of the module")
    parser.add_argument("--package", help="path to this package")
    parser.add_argument("--url", help="URL for http_archive")
    parser.add_argument("--sha256", help="SHA256 hash")
    parser.add_argument("--strip_prefix", help="Strip prefix for archive")
    parser.add_argument(
        "--output", nargs="?", default=None, help="Path to write output MODULE.bazel file (default: stdout)"
    )
    args = parser.parse_args()

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
        output_path.write_text(content)
        print(f"Generated {args.output}", file=sys.stderr)
    else:
        sys.stdout.write(content)


if __name__ == "__main__":
    main()
