# Copyright 2015 The Bazel Authors. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Rules for making .tar files."""

load("//pkg:providers.bzl", "PackageVariablesInfo")
load(
    "//pkg/private:pkg_files.bzl",
    "add_directory",
    "add_empty_file",
    "add_label_list",
    "add_single_file",
    "add_symlink",
    "create_mapping_context_from_ctx",
    "write_manifest",
)
load("//pkg/private:util.bzl", "get_stamp_detect", "setup_output_files", "substitute_package_variables")

# TODO(aiuto): Figure  out how to get this from the python toolchain.
# See check for lzma in archive.py for a hint at a method.
HAS_XZ_SUPPORT = True

# Filetype to restrict inputs
tar_filetype = (
    [".tar", ".tar.gz", ".tgz", ".tar.bz2", "tar.xz", ".txz"] if HAS_XZ_SUPPORT else [".tar", ".tar.gz", ".tgz", ".tar.bz2"]
)
SUPPORTED_TAR_COMPRESSIONS = (
    ["", "gz", "bz2", "xz"] if HAS_XZ_SUPPORT else ["", "gz", "bz2"]
)
_DEFAULT_MTIME = -1

def _remap(remap_paths, path):
    """If path starts with a key in remap_paths, rewrite it."""
    for prefix, replacement in remap_paths.items():
        if path.startswith(prefix):
            return replacement + path[len(prefix):]
    return path

def _quote(filename, protect = "="):
    """Quote the filename, by escaping = by \\= and \\ by \\\\"""
    return filename.replace("\\", "\\\\").replace(protect, "\\" + protect)

def _pkg_tar_impl(ctx):
    """Implementation of the pkg_tar rule."""

    # Files needed by rule implementation at runtime
    files = []
    outputs, output_file, _ = setup_output_files(ctx)

    # Declare the md5sums sidecar output.
    md5sums_file = ctx.actions.declare_file(ctx.label.name + ".md5sums")

    # Start building the arguments.
    args = ctx.actions.args()
    args.add("--output", output_file.path)
    args.add("--md5sums_output", md5sums_file.path)
    args.add("--mode", ctx.attr.mode)
    args.add("--owner", ctx.attr.owner)
    args.add("--owner_name", ctx.attr.ownername)

    # Package dir can be specified by a file or inlined.
    if ctx.attr.package_dir_file:
        if ctx.attr.package_dir:
            fail("Both package_dir and package_dir_file attributes were specified")
        args.add("--directory", "@" + ctx.file.package_dir_file.path)
        files.append(ctx.file.package_dir_file)
    else:
        package_dir_expanded = substitute_package_variables(ctx, ctx.attr.package_dir)
        args.add("--directory", package_dir_expanded or "/")

    if ctx.executable.compressor:
        args.add("--compressor", "%s %s" % (ctx.executable.compressor.path, ctx.attr.compressor_args))
    else:
        extension = ctx.attr.extension
        if extension and extension != "tar":
            compression = None
            dot_pos = ctx.attr.extension.rfind(".")
            if dot_pos >= 0:
                compression = ctx.attr.extension[dot_pos + 1:]
            else:
                compression = ctx.attr.extension
            if compression == "tgz":
                compression = "gz"
            if compression == "txz":
                compression = "xz"
            if compression:
                if compression in SUPPORTED_TAR_COMPRESSIONS:
                    args.add("--compression", compression)
                else:
                    fail("Unsupported compression: '%s'" % compression)

    if ctx.attr.mtime != _DEFAULT_MTIME:
        if ctx.attr.portable_mtime:
            fail("You may not set both mtime and portable_mtime")
        args.add("--mtime", "%d" % ctx.attr.mtime)
    if ctx.attr.portable_mtime:
        args.add("--mtime", "portable")
    if ctx.attr.modes:
        for key in ctx.attr.modes:
            args.add("--modes", "%s=%s" % (_quote(key), ctx.attr.modes[key]))
    if ctx.attr.owners:
        for key in ctx.attr.owners:
            args.add("--owners", "%s=%s" % (_quote(key), ctx.attr.owners[key]))
    if ctx.attr.ownernames:
        for key in ctx.attr.ownernames:
            args.add(
                "--owner_names",
                "%s=%s" % (_quote(key), ctx.attr.ownernames[key]),
            )
    if ctx.attr.compression_level >= 0:
        args.add("--compression_level", str(ctx.attr.compression_level))

    # Now we begin processing the files.
    path_mapper = None
    if ctx.attr.remap_paths:
        path_mapper = lambda path: _remap(ctx.attr.remap_paths, path)

    mapping_context = create_mapping_context_from_ctx(
        ctx,
        label = ctx.label,
        include_runfiles = ctx.attr.include_runfiles,
        strip_prefix = ctx.attr.strip_prefix,
        # build_tar does the default modes. Consider moving attribute mapping
        # into mapping_context.
        default_mode = None,
        path_mapper = path_mapper,
    )

    add_label_list(mapping_context, srcs = ctx.attr.srcs)

    # The files attribute is a map of labels to destinations. We can add them
    # directly to the content map.
    for target, f_dest_path in ctx.attr.files.items():
        target_files = target[DefaultInfo].files.to_list()
        if len(target_files) != 1:
            fail("Each input must describe exactly one file.", attr = "files")
        mapping_context.file_deps_direct.append(target_files[0])
        add_single_file(
            mapping_context,
            f_dest_path,
            target_files[0],
            target.label,
        )

    for empty_file in ctx.attr.empty_files:
        add_empty_file(mapping_context, empty_file, ctx.label)
    for empty_dir in ctx.attr.empty_dirs or []:
        add_directory(mapping_context, empty_dir, ctx.label)
    for f in ctx.files.deps:
        args.add("--tar", f.path)
    for link in ctx.attr.symlinks:
        add_symlink(
            mapping_context,
            link,
            ctx.attr.symlinks[link],
            ctx.label,
        )
    if ctx.attr.stamp == 1 or (ctx.attr.stamp == -1 and
                               ctx.attr.private_stamp_detect):
        args.add("--stamp_from", ctx.version_file.path)
        files.append(ctx.version_file)

    manifest_file = ctx.actions.declare_file(ctx.label.name + ".manifest")
    files.append(manifest_file)
    write_manifest(ctx, manifest_file, mapping_context.content_map)
    args.add("--manifest", manifest_file.path)

    args.set_param_file_format("flag_per_line")
    args.use_param_file("@%s", use_always = False)

    if ctx.attr.create_parents:
        args.add("--create_parents")

    if ctx.attr.allow_duplicates_from_deps:
        args.add("--allow_dups_from_deps")

    if ctx.attr.preserve_mode:
        args.add("--preserve_mode")

    if ctx.attr.preserve_mtime:
        args.add("--preserve_mtime")

    inputs = depset(
        direct = mapping_context.file_deps_direct + ctx.files.deps + files,
        transitive = mapping_context.file_deps_transitive,
    )

    ctx.actions.run(
        mnemonic = "PackageTar",
        progress_message = "Writing: %s" % output_file.path,
        inputs = inputs,
        tools = [ctx.executable.compressor] if ctx.executable.compressor else [],
        executable = ctx.executable._build_tar,
        arguments = [args],
        outputs = [output_file, md5sums_file],
        env = {
            "LANG": "en_US.UTF-8",
            "LC_CTYPE": "UTF-8",
            "PYTHONIOENCODING": "UTF-8",
            "PYTHONUTF8": "1",
        },
        use_default_shell_env = True,
    )
    return [
        DefaultInfo(
            files = depset([output_file]),
            runfiles = ctx.runfiles(files = outputs),
        ),
        # NB: this is not a committed public API.
        # The format of this file is subject to change without notice,
        # or this OutputGroup might be totally removed.
        # Depend on it at your own risk!
        OutputGroupInfo(
            manifest = [manifest_file],
            md5sums = depset([md5sums_file]),
        ),
    ]

# A rule for creating a tar file, see README.md
pkg_tar_impl = rule(
    implementation = _pkg_tar_impl,
    attrs = {
        "strip_prefix": attr.string(
            doc = """(note: Use strip_prefix = "." to strip path to the package but preserve relative paths of sub directories beneath the package.)""",
        ),
        "package_dir": attr.string(
            doc = """Prefix to be prepend to all paths written.

            This is applied as a final step, while writing to the archive.
            Any other attributes (e.g. symlinks) which specify a path, must do so relative to package_dir.
            The value may contain variables. See [package_file_name](#package_file_name) for examples.
            """,
        ),
        "package_dir_file": attr.label(allow_single_file = True),
        "deps": attr.label_list(
            doc = """tar files which will be unpacked and repacked into the archive.""",
            allow_files = tar_filetype,
        ),
        "srcs": attr.label_list(
            doc = """Inputs which will become part of the tar archive.""",
            allow_files = True,
        ),
        "files": attr.label_keyed_string_dict(
            doc = """Obsolete. Do not use.""",
            allow_files = True,
        ),
        "mode": attr.string(default = "0555"),
        "modes": attr.string_dict(),
        "mtime": attr.int(default = _DEFAULT_MTIME),
        "portable_mtime": attr.bool(default = True),
        "owner": attr.string(
            doc = """Default numeric owner.group to apply to files when not set via pkg_attributes.""",
            default = "0.0",
        ),
        "ownername": attr.string(default = "."),
        "owners": attr.string_dict(),
        "ownernames": attr.string_dict(),
        "extension": attr.string(
            default = "tar",
            doc = """The extension of the generated file. If `"gz"`, `"bz2"`, or `"xz"`, the
tarball will also be compressed using that tool, and is mutually exclusive with `compressor`.
Note that `xz` may not be supported based on the Python toolchain.
""",
        ),
        "symlinks": attr.string_dict(),
        "empty_files": attr.string_list(),
        "include_runfiles": attr.bool(
            doc = ("""Include runfiles for executables. These appear as they would in bazel-bin.""" +
                   """ For example: 'path/to/myprog.runfiles/path/to/my_data.txt'."""),
        ),
        "empty_dirs": attr.string_list(),
        "remap_paths": attr.string_dict(),
        "compressor": attr.label(
            doc = """External tool which can compress the archive.""",
            executable = True,
            cfg = "exec",
        ),
        "compressor_args": attr.string(
            doc = """Arg list for `compressor`.""",
        ),
        "create_parents": attr.bool(default = True),
        "allow_duplicates_from_deps": attr.bool(default = False),
        "compression_level": attr.int(
            doc = """Specify the numeric compression level in gzip mode; may be 0-9 or -1 (default to 6).""",
            default = -1,
        ),

        # Common attributes
        "out": attr.output(mandatory = True),
        "package_file_name": attr.string(doc = "See [Common Attributes](#package_file_name)"),
        "package_variables": attr.label(
            doc = "See [Common Attributes](#package_variables)",
            providers = [PackageVariablesInfo],
        ),
        "allow_duplicates_with_different_content": attr.bool(
            default = True,
            doc = """If true, will allow you to reference multiple pkg_* which conflict
(writing different content or metadata to the same destination).
Such behaviour is always incorrect, but we provide a flag to support it in case old
builds were accidentally doing it. Never explicitly set this to true for new code.
""",
        ),
        "preserve_mode": attr.bool(
            default = False,
            doc = """If true, will add file to archive with preserved file permissions.""",
        ),
        "preserve_mtime": attr.bool(
            default = False,
            doc = """If true, will add file to archive with preserved file mtime.""",
        ),
        "stamp": attr.int(
            doc = """Enable file time stamping.  Possible values:
<li>stamp = 1: Use the time of the build as the modification time of each file in the archive.
<li>stamp = 0: Use an "epoch" time for the modification time of each file. This gives good build result caching.
<li>stamp = -1: Control the chosen modification time using the --[no]stamp flag.
@since(0.5.0)
""",
            default = 0,
        ),
        # Is --stamp set on the command line?
        # TODO(https://github.com/bazelbuild/rules_pkg/issues/340): Remove this.
        "private_stamp_detect": attr.bool(default = False),

        # Implicit dependencies.
        # Points to dd_tar_writer, a Go binary that replaces the upstream Python
        # build_tar tool and additionally emits a .md5sums sidecar file.
        # Label uses @@// (canonical root-module prefix in bzlmod) so that this
        # reference resolves to the main repository even though this file lives
        # inside the rules_pkg module.
        "_build_tar": attr.label(
            default = Label("@@//bazel/rules/dd_tar_writer:dd_tar_writer"),
            cfg = "exec",
            executable = True,
            allow_files = True,
        ),
    },
)

# buildifier: disable=function-docstring-args
def pkg_tar(name, **kwargs):
    """Creates a .tar file. See pkg_tar_impl.

    @wraps(pkg_tar_impl)
    """

    # Compatibility with older versions of pkg_tar that define files as
    # a flat list of labels.
    if "srcs" not in kwargs:
        if "files" in kwargs:
            if not hasattr(kwargs["files"], "items"):
                label = "%s//%s:%s" % (native.repository_name(), native.package_name(), name)

                # buildifier: disable=print
                print("%s: you provided a non dictionary to the pkg_tar `files` attribute. " % (label,) +
                      "This attribute was renamed to `srcs`. " +
                      "Consider renaming it in your BUILD file.")
                kwargs["srcs"] = kwargs.pop("files")
    extension = kwargs.get("extension") or "tar"
    if extension[0] == ".":
        extension = extension[1:]
    pkg_tar_impl(
        name = name,
        out = kwargs.pop("out", None) or (name + "." + extension),
        private_stamp_detect = get_stamp_detect(kwargs.get("stamp", 0)),
        **kwargs
    )
