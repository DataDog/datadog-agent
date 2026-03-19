load("@rules_pkg//pkg:mappings.bzl", "pkg_filegroup", "pkg_files", "pkg_mklink")

_SPECS = [
    struct(os = "linux", prefix = "lib/", format = "{}.so{}"),
    struct(os = "macos", prefix = "lib/", format = "{}{}.dylib"),
    struct(os = "windows", prefix = "bin/", format = "{}.dll"),
]

def _gen_targets(base_name, src, libname, version, prefix, spec):
    name = "{}_{}".format(base_name, spec.os)
    platform = "@platforms//os:{}".format(spec.os)
    dest_prefix = (prefix + "/" + spec.prefix) if prefix else spec.prefix

    # Windows: no symlinks, no renaming - just copy the DLL as-is
    if spec.os == "windows":
        pkg_files(
            name = name,
            srcs = [src],
            prefix = dest_prefix,
            target_compatible_with = [platform],
            package_metadata = [],
        )
        return platform, [":{}".format(name)]

    # Unix: create symlink chain with versioning
    target = spec.format.format(libname, ".{}".format(version))
    targets = ["{}_real_name".format(name)]
    pkg_files(
        name = targets[-1],
        srcs = [src],
        prefix = dest_prefix,
        renames = {src: target},
        target_compatible_with = [platform],
        package_metadata = [],
    )

    major = version.partition(".")[0]
    for link_name, link_version in (("soname", ".{}".format(major)), ("linker_name", "")):
        link = spec.format.format(libname, link_version)
        if link == target:
            continue
        targets.append("{}_{}".format(name, link_name))
        pkg_mklink(
            name = targets[-1],
            link_name = "{}{}".format(dest_prefix, link),
            target = target,
            target_compatible_with = [platform],
            package_metadata = [],
        )
        target = link

    pkg_filegroup(name = name, srcs = targets, target_compatible_with = [platform], package_metadata = [])
    return platform, [":{}".format(name)]

def _so_symlink_impl(name, src, libname, version, prefix, visibility):
    src_str = ":{}".format(src.name)
    pkg_filegroup(
        name = name,
        srcs = select(dict([_gen_targets(name, src_str, libname, version, prefix, spec) for spec in _SPECS])),
        package_metadata = [],
        visibility = visibility,
    )

so_symlink = macro(
    doc = """Creates shared library symlink chain following Unix conventions.

    Unix (Linux/macOS): Generates the common multilevel symlink hierarchy for shared libraries:
    - `real name`: actual file with full version (e.g., libreadline.so.3.0 / libreadline.3.0.dylib)
    - `soname`: major version symlink, for runtime ABI compatibility (e.g., libreadline.so.3 / libreadline.3.dylib)
    - `linker name`: unversioned symlink, for development/linking (e.g., libreadline.so / libreadline.dylib)

    Windows: Simply copies the DLL to bin/ without renaming or creating symlinks.

    See: `Program Library HOWTO` by David Wheeler, https://tldp.org/HOWTO/Program-Library-HOWTO/shared-libraries.html
    """,
    attrs = {
        "src": attr.label(
            doc = "Label of the cc_shared_library to package",
            mandatory = True,
            configurable = False,
        ),
        "libname": attr.string(
            doc = "Library name without extension (e.g., \"libreadline\")",
            mandatory = True,
            configurable = False,
        ),
        "version": attr.string(
            doc = "Full version string (e.g., \"3.0\", ignored on Windows)",
            mandatory = True,
            configurable = False,
        ),
        "prefix": attr.string(
            doc = "Installation directory prefix (default: \"\")",
            default = "",
            configurable = False,
        ),
    },
    implementation = _so_symlink_impl,
)
