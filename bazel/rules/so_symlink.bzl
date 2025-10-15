load("@rules_pkg//pkg:mappings.bzl", "pkg_filegroup", "pkg_files", "pkg_mklink")

_SPECS = [
    struct(os = "linux", format = "{}.so.{}", link_suffix = ""),
    struct(os = "macos", format = "{}.{}.dylib", link_suffix = ".dylib"),
]

def _gen_targets(base_name, src, libname, version, prefix, spec):
    name = "{}_{}".format(base_name, spec.os)
    target_compatible_with = ["@platforms//os:{}".format(spec.os)]
    targets = ["{}_lib".format(name)]
    target = spec.format.format(libname, version)
    pkg_files(
        name = targets[-1],
        srcs = [src],
        prefix = prefix,
        renames = {src: target},
        target_compatible_with = target_compatible_with,
    )

    parts = target[:len(target) - len(spec.link_suffix)].split(".")
    for _ in range(version.count(".") + 1):
        parts = parts[:-1]
        targets.append("{}_link".format(targets[-1]))
        link = "{}{}".format(".".join(parts), spec.link_suffix)
        pkg_mklink(
            name = targets[-1],
            link_name = "{}{}".format(prefix, link),
            target = target,
            target_compatible_with = target_compatible_with,
        )
        target = link

    pkg_filegroup(name = name, srcs = targets, target_compatible_with = target_compatible_with)
    return ":{}".format(name)

def so_symlink(name, src, libname, version, prefix = "lib/"):
    """Creates shared library symlink chain following Unix conventions.

    Generates the common multilevel symlink hierarchy for shared libraries, for reference:
    - `real name`: actual file with full version (e.g., libreadline.so.3.0 / libreadline.3.0.dylib)
    - `soname`: major version symlink, for runtime ABI compatibility (e.g., libreadline.so.3 / libreadline.3.dylib)
    - `linker name`: unversioned symlink, for development/linking (e.g., libreadline.so / libreadline.dylib)

    See: `Program Library HOWTO` by David Wheeler, https://tldp.org/HOWTO/Program-Library-HOWTO/shared-libraries.html

    Args:
        name: Name of the generated pkg_filegroup
        src: Label of the cc_shared_library to package
        libname: Library name without extension (e.g., "libreadline")
        version: Full version string (e.g., "3.0")
        prefix: Installation directory prefix (default: "lib/")
    """
    native.alias(
        name = name,
        actual = select({
            "@platforms//os:{}".format(spec.os): _gen_targets(name, src, libname, version, prefix, spec)
            for spec in _SPECS
        }),
    )
