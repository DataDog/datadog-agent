""" Macro for building CMake from sources. """

load("//foreign_cc:defs.bzl", "cmake_variant", "configure_make")

def cmake_tool(name, srcs, **kwargs):
    """Macro for building CMake

    Args:
        name: A unique name for this target
        srcs: The target containing the build tool's sources
        **kwargs: Remaining args
    """
    tags = ["manual"] + kwargs.pop("tags", [])

    native.alias(
        name = "{}.build".format(name),
        actual = select({
            ":msvc_compiler": "{}_msvc".format(name),
            "//conditions:default": "{}_default".format(name),
        }),
    )

    cmake_variant(
        name = "{}_msvc".format(name),
        # to prevent errors with incompatible _WIN32_WINNT in cmlibarchive
        # override NTDDI_VERSION to match _WIN32_WINNT set in the default cc_toolchain
        copts = ["/D NTDDI_VERSION=0x06010000"],
        lib_source = srcs,
        out_binaries = ["cmake.exe"],
        toolchain = "@rules_foreign_cc//toolchains:preinstalled_cmake_toolchain",
        tags = tags,
        **kwargs
    )

    configure_make(
        name = "{}_default".format(name),
        configure_command = "bootstrap",
        # On macOS at least -DDEBUG gets set for a fastbuild
        copts = ["-UDEBUG"],
        lib_source = srcs,
        out_binaries = select({
            "@platforms//os:windows": ["cmake.exe"],
            "//conditions:default": ["cmake"],
        }),
        out_static_libs = [],
        out_shared_libs = [],
        tags = tags,
        **kwargs
    )

    native.filegroup(
        name = name,
        srcs = ["{}.build".format(name)],
        output_group = "gen_dir",
        tags = tags,
    )
