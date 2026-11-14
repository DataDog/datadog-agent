"""minimal_cc_binary: a cc_binary-like rule that injects no toolchain default flags.

The stock `cc_binary` runs its link action through the C++ feature pipeline,
which in this repo means the toolchain's `default_link_flags` feature quietly
appends `-lm -ldl -lrt -pthread` to every link. That is fine for normal
binaries but unacceptable for something like the LD_PRELOAD `nosys.so` shim,
which must not force those shared libraries onto the loader.

`minimal_cc_binary` still resolves the C toolchain (so cc toolchain selection,
sandboxing, and the compiler binary itself all work), but it constructs the
gcc command line by hand rather than driving it through
`cc_common.get_memory_inefficient_command_line`. Only the user's `copts` and
`linkopts` reach gcc, plus `-I / -iquote / -isystem / -D` derived from each
dep's `CcInfo.compilation_context`.

**`deps` is compile-only.** Unlike `cc_binary`, `minimal_cc_binary` never
auto-links libraries reported by a dep's `linking_context`. Deps contribute
headers, include paths, and defines; nothing else. Any library that should be
linked in must be listed explicitly as an `additional_linker_inputs` entry and
referenced from `linkopts` — typically positionally, e.g.
`"$(location :libseccomp.so.2)"`. This keeps every symbol added to the output
traceable to a line in the BUILD file.

The rule intentionally omits everything the feature pipeline would normally
add: `default_compile_flags`, `default_link_flags`, `unfiltered_compile_flags`,
`redacted_dates`, sysroot handling, `-iquote` for the exec root, and the
built-in `-isystem` entries. If you need any of those, use `cc_binary`.

Example:

    load("//bazel/rules/minimal_cc_binary:minimal_cc_binary.bzl", "minimal_cc_binary")

    minimal_cc_binary(
        name = "nosys.so",
        srcs = ["nosys.c"],
        # @libseccomp provides <seccomp.h>; its static archive is deliberately
        # not linked in. We link against a shared-library stub instead so that
        # DT_NEEDED records "libseccomp.so.2".
        deps = ["@libseccomp"],
        additional_linker_inputs = [
            "nosys.sym",
            ":libseccomp.so.2",
        ],
        copts = ["-O2", "-fPIC"],
        linkopts = [
            "-Wl,--version-script=$(location nosys.sym)",
            "$(location :libseccomp.so.2)",
        ],
        linkshared = True,
    )
"""

load("@rules_cc//cc:action_names.bzl", "C_COMPILE_ACTION_NAME")
load("@rules_cc//cc:find_cc_toolchain.bzl", "CC_TOOLCHAIN_ATTRS", "find_cc_toolchain", "use_cc_toolchain")
load("@rules_cc//cc/common:cc_common.bzl", "cc_common")
load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")

def _minimal_cc_binary_impl(ctx):
    cc_toolchain = find_cc_toolchain(ctx)

    # We only need the toolchain for its compiler binary + input files. We
    # deliberately do NOT use configure_features / get_memory_inefficient_command_line,
    # so no feature (default_link_flags, unfiltered_compile_flags, etc.) has a
    # chance to add flags behind the caller's back.
    feature_configuration = cc_common.configure_features(
        ctx = ctx,
        cc_toolchain = cc_toolchain,
        requested_features = ctx.features,
        unsupported_features = ctx.disabled_features,
    )
    cc_bin = cc_common.get_tool_for_action(
        feature_configuration = feature_configuration,
        action_name = C_COMPILE_ACTION_NAME,
    )

    cc_infos = [dep[CcInfo] for dep in ctx.attr.deps]
    merged = cc_common.merge_cc_infos(cc_infos = cc_infos) if cc_infos else None

    output = ctx.actions.declare_file(ctx.label.name)

    args = ctx.actions.args()
    if ctx.attr.linkshared:
        args.add("-shared")
    args.add_all(ctx.attr.copts)

    header_depsets = []
    if merged:
        cctx = merged.compilation_context
        header_depsets.append(cctx.headers)
        args.add_all(cctx.quote_includes, before_each = "-iquote")
        args.add_all(cctx.includes, format_each = "-I%s")
        args.add_all(cctx.system_includes, before_each = "-isystem")
        if hasattr(cctx, "external_includes"):
            args.add_all(cctx.external_includes, before_each = "-isystem")
        args.add_all(cctx.defines, format_each = "-D%s")

    args.add("-o", output.path)
    args.add_all(ctx.files.srcs)
    for opt in ctx.attr.linkopts:
        args.add(ctx.expand_location(opt, targets = ctx.attr.additional_linker_inputs))

    inputs = depset(
        direct = ctx.files.srcs + ctx.files.deps + ctx.files.additional_linker_inputs,
        transitive = [cc_toolchain.all_files] + header_depsets,
    )

    ctx.actions.run(
        mnemonic = "MinimalCcLink",
        progress_message = "MinimalCcLink %{output}",
        executable = cc_bin,
        arguments = [args],
        inputs = inputs,
        outputs = [output],
    )

    return [DefaultInfo(files = depset([output]))]

minimal_cc_binary = rule(
    implementation = _minimal_cc_binary_impl,
    doc = "Bare-bones cc_binary that never injects toolchain default flags. " +
          "See module docstring for details and trade-offs.",
    attrs = {
        "srcs": attr.label_list(
            doc = "C source files to compile and link in a single gcc invocation.",
            allow_files = [".c", ".S", ".s"],
            mandatory = True,
        ),
        "deps": attr.label_list(
            doc = "cc_library-style dependencies used for **headers only**. " +
                  "Their compilation_context supplies -I/-iquote/-isystem/-D. " +
                  "Any library that should be linked in must be listed in " +
                  "`additional_linker_inputs` and named from `linkopts`.",
            providers = [CcInfo],
        ),
        "copts": attr.string_list(
            doc = "Compiler flags passed verbatim, in the order given.",
        ),
        "linkopts": attr.string_list(
            doc = "Linker flags passed verbatim after copts. `$(location LABEL)` " +
                  "is expanded against `additional_linker_inputs`.",
        ),
        "additional_linker_inputs": attr.label_list(
            doc = "Files (version scripts, shared-library stubs, static archives) " +
                  "made available to the link action and to `$(location ...)` " +
                  "expansion in `linkopts`.",
            allow_files = True,
        ),
        "linkshared": attr.bool(
            doc = "When True, pass `-shared` to gcc.",
            default = False,
        ),
    } | CC_TOOLCHAIN_ATTRS,
    toolchains = use_cc_toolchain(),
    fragments = ["cpp"],
)
