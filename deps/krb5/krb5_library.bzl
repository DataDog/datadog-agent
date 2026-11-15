"""krb5_shared_library: the cc_library, cc_shared_library and packaging targets of one krb5 library or plugin."""

load("@rules_cc//cc:defs.bzl", "cc_library", "cc_shared_library")
load("//bazel/rules/dd_packaging:dd_cc_packaged.bzl", "dd_cc_packaged")
load(":krb5_codegen.bzl", "krb5_version_script")
load(":krb5_defs.bzl", "KRB5_COPTS", "KRB5_LOCAL_DEFINES")

def _krb5_shared_library_impl(
        name,
        visibility,
        srcs,
        textual_hdrs,
        deps,
        implementation_deps,
        local_defines,
        linkopts,
        exports,
        version,
        plugin_dir,
        file_name,
        dynamic_deps,
        static_deps,
        pkg_data):
    base = file_name or name
    major = version.partition(".")[0] if version else "0"
    if plugin_dir:
        shared_lib_name = base + ".so"
        soname = "{}.so.{}".format(base, major)
    else:
        soname = "lib{}.so.{}".format(base, major)
        shared_lib_name = soname

    krb5_version_script(
        name = name + "_version_script",
        out = name + ".versions",
        exports = exports,
        libbase = base,
        libmajor = major,
    )
    cc_library(
        name = name + "_lib",
        srcs = srcs,
        textual_hdrs = textual_hdrs,
        copts = KRB5_COPTS,
        implementation_deps = implementation_deps,
        linkopts = linkopts,
        local_defines = KRB5_LOCAL_DEFINES + local_defines,
        visibility = visibility,
        deps = deps,
    )
    cc_shared_library(
        name = name,
        additional_linker_inputs = [":{}_version_script".format(name)],
        dynamic_deps = dynamic_deps,
        shared_lib_name = shared_lib_name,
        user_link_flags = [
            "-Wl,-soname," + soname,
            "-Wl,--version-script=$(location :{}_version_script)".format(name),
        ],
        visibility = visibility,
        deps = [":{}_lib".format(name)] + static_deps,
    )
    dd_cc_packaged(
        name = name + "_pkg",
        data = pkg_data,
        dest_dir = "embedded/lib",
        input = ":" + name,
        prefix = plugin_dir,
        version = "" if plugin_dir else version,
        visibility = visibility,
    )

krb5_shared_library = macro(
    doc = """Declares a krb5 library or plugin the way the upstream Makefiles build it.

    Creates `<name>_lib` (cc_library), `<name>` (cc_shared_library with the upstream soname and
    `<base>_<major>_MIT` symbol version script) and `<name>_pkg` (dd_cc_packaged).
    """,
    implementation = _krb5_shared_library_impl,
    attrs = {
        "srcs": attr.label_list(allow_files = True, mandatory = True, doc = "Sources and private headers."),
        "textual_hdrs": attr.label_list(allow_files = True, doc = "Files included textually by the sources."),
        "deps": attr.label_list(doc = "Libraries and headers, propagated to dependents."),
        "implementation_deps": attr.label_list(doc = "Private headers."),
        "local_defines": attr.string_list(doc = "Defines added to the common krb5 ones."),
        "linkopts": attr.string_list(doc = "Linker flags."),
        "exports": attr.label(allow_single_file = True, mandatory = True, doc = "The upstream .exports file."),
        "version": attr.string(configurable = False, doc = "Library version (e.g. 3.3). Unset for plugins, whose major is 0."),
        "plugin_dir": attr.string(configurable = False, doc = "Install subdirectory under embedded/lib for plugins. Empty for libraries."),
        "file_name": attr.string(configurable = False, doc = "Base file name when it differs from name (e.g. the preauth test plugin)."),
        "dynamic_deps": attr.label_list(doc = "Packaged shared libraries this one links against."),
        "static_deps": attr.label_list(doc = "Additional cc_library targets linked into the shared library."),
        "pkg_data": attr.label_list(doc = "Extra files packaged alongside the library."),
    },
)
