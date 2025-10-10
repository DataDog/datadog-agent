load("@rules_pkg//pkg:mappings.bzl", "pkg_filegroup", "pkg_files", "pkg_mklink")

def so_symlink(name, src, real_name, prefix = "lib/"):
    """Creates shared library symlink chain following Unix conventions.

    Generates the common multilevel symlink hierarchy for shared libraries, for reference:
    - `real name`: actual file with full version (e.g., libreadline.so.3.0)
    - `soname`: major version symlink, for runtime ABI compatibility (e.g., libreadline.so.3)
    - `linker name`: unversioned symlink, for development/linking (e.g., libreadline.so)

    Ref: Program Library HOWTO by David Wheeler, https://tldp.org/HOWTO/Program-Library-HOWTO/shared-libraries.html

    Args:
        name: Name of the generated pkg_filegroup
        src: Label of the cc_shared_library to package
        real_name: Full versioned filename (e.g., "libreadline.so.3.0")
        prefix: Installation directory prefix (default: "lib/")
    """
    targets = [name + "_real_name"]
    pkg_files(
        name = targets[-1],
        srcs = [src],
        prefix = prefix,
        renames = {src: real_name},
    )

    parts = real_name.split(".")
    for i in range(1, len(parts) - 1):
        targets.append(targets[-1] + "_link")
        pkg_mklink(
            name = targets[-1],
            link_name = prefix + ".".join(parts[:len(parts) - i]),
            target = ".".join(parts[:len(parts) - i + 1]),
        )

    pkg_filegroup(name = name, srcs = targets)
