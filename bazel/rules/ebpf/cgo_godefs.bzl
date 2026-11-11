"""Bazel rule for generating Go type definitions from C headers via cgo -godefs."""

load("@bazel_lib//lib:write_source_files.bzl", "write_source_file")
load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")
load("@rules_go//go:def.bzl", "go_context", "go_rule")
load("//bazel/rules/ebpf:cc_helpers.bzl", "collect_headers", "collect_include_dirs")

def _relpath(target, base):
    """Compute a relative path from base to target (both relative to execroot)."""
    target_parts = target.split("/") if target else []
    base_parts = base.split("/") if base else []

    # Find common prefix length
    common = 0
    for i in range(min(len(target_parts), len(base_parts))):
        if target_parts[i] != base_parts[i]:
            break
        common = i + 1

    ups = len(base_parts) - common
    remainder = target_parts[common:]
    parts = [".."] * ups + remainder
    return "/".join(parts) if parts else "."

def _cgo_godefs_impl(ctx):
    go = go_context(ctx)

    src = ctx.file.src
    genpost = ctx.executable._genpost

    platform = go.sdk.goos
    base = src.basename.removesuffix(".go")

    # Prefix with _ to avoid runfiles path collision with the committed source
    # file of the same name. Without this, write_source_file's diff_test
    # resolves both the generated and committed files to the same runfiles path,
    # causing the test to compare a file against itself (always passes).
    out_name = "_" + base + "_" + platform + ".go"
    out = ctx.actions.declare_file(out_name)
    outputs = [out]

    all_deps = list(ctx.attr.deps)
    inc = collect_include_dirs(all_deps)
    headers = collect_headers(all_deps + list(ctx.attr.hdrs))

    # Filter out bazel-out and root dirs; keep only source-tree includes.
    # CcInfo adds generated output dirs that don't affect correctness but
    # would pollute the cgo -godefs comment line embedded in the output.
    def _filter_and_rel(dirs):
        result = []
        for d in dirs:
            if d.startswith("bazel-out") or d == ".":
                continue
            r = _relpath(d, src.dirname)
            if r not in result:
                result.append(r)
        return result

    cgo_flags = []
    for flag, dirs in (
        ("-I", inc.includes),
        ("-isystem", inc.system_includes),
        ("-iquote", inc.quote_includes),
    ):
        for d in _filter_and_rel(dirs):
            cgo_flags.append(flag)
            cgo_flags.append(d)
    cgo_flags.append("-fsigned-char")

    test_out = None
    package_name = ""
    if platform == "linux":
        test_name = "_" + base + "_" + platform + "_test.go"
        test_out = ctx.actions.declare_file(test_name)
        outputs.append(test_out)
        package_name = ctx.label.package.split("/")[-1]

    # genpost runs cgo itself and writes the outputs, so this does not need a
    # shell to pipe cgo's stdout. Paths are execroot-relative; genpost makes
    # CC and GOROOT absolute before changing to the source directory.
    args = ctx.actions.args()
    args.add("-out", out)
    args.add("-src", src)
    args.add("-go", go.sdk.go)
    args.add("-cc", ctx.file.c_compiler)
    args.add("-goroot", go.sdk.root_file.dirname)
    if test_out:
        args.add("-test", test_out)
        args.add("-pkg", package_name)
    args.add("--")
    args.add_all(cgo_flags)

    env = dict(go.env)
    env["CGO_ENABLED"] = "1"

    ctx.actions.run(
        outputs = outputs,
        inputs = depset(
            [src, go.sdk.go, ctx.file.c_compiler] + ctx.files.c_compiler_data,
            transitive = [headers, go.sdk.tools, go.sdk.srcs, go.sdk.libs],
        ),
        executable = genpost,
        arguments = [args],
        env = env,
        mnemonic = "CgoGodefs",
        progress_message = "Generating Go types from %{label}",
    )

    output_groups = {"main": depset([out])}
    if test_out:
        output_groups["test_file"] = depset([test_out])

    return [
        DefaultInfo(files = depset(outputs)),
        OutputGroupInfo(**output_groups),
    ]

_cgo_godefs = go_rule(
    implementation = _cgo_godefs_impl,
    attrs = {
        "src": attr.label(
            mandatory = True,
            allow_single_file = [".go"],
            doc = """The Go source file containing C type references (import "C").""",
        ),
        "deps": attr.label_list(
            providers = [CcInfo],
            doc = "cc_library targets providing C headers and -I include paths needed by the cgo source.",
        ),
        "hdrs": attr.label_list(
            providers = [CcInfo],
            doc = "cc_library targets whose headers are needed in the sandbox but whose include dirs should not appear as -I flags.",
        ),
        "c_compiler": attr.label(
            allow_single_file = True,
            cfg = "exec",
        ),
        "c_compiler_data": attr.label_list(
            allow_files = True,
            cfg = "exec",
            doc = "Files the C compiler needs at run time, e.g. clang's builtin resource directory (stddef.h, ...).",
        ),
        "_genpost": attr.label(
            default = "//pkg/ebpf/cgo:genpost",
            executable = True,
            cfg = "exec",
        ),
        "_go_context_data": attr.label(
            default = "@rules_go//:go_context_data",
        ),
    },
)

_STD_LINUX_DEPS = [
    "//pkg/ebpf/c:ebpf_c_headers",
]

def _cgo_godefs_macro_impl(name, visibility, src, deps, hdrs, platform):
    all_deps = deps + (_STD_LINUX_DEPS if platform == "linux" else [])

    # Applied to every target the macro creates so the diff_test stamped out by
    # write_source_file is skipped on incompatible OSes — incompatibility-by-
    # association does not propagate reliably when tests are named explicitly
    # (e.g. via the verify_generated_files test_suite).
    compat = select({
        "@platforms//os:{}".format(platform): [],
        "//conditions:default": ["@platforms//:incompatible"],
    })

    gen = name + "_gen"
    _cgo_godefs(
        name = gen,
        src = src,
        deps = all_deps,
        hdrs = hdrs,
        c_compiler = select({
            "@platforms//os:windows": "@winlibs_mingw64//:gcc",
            "//conditions:default": "@llvm//tools:clang",
        }),
        # The minimal LLVM distribution ships builtin headers apart from the
        # binary; clang finds them via bin/../lib/clang/<ver> only if staged.
        c_compiler_data = select({
            "@platforms//os:windows": [],
            "//conditions:default": ["@llvm//:builtin_resource_dir"],
        }),
        target_compatible_with = compat,
    )

    base = src.name.removesuffix(".go")
    main_file = base + "_" + platform + ".go"

    native.filegroup(
        name = name + "_main_out",
        srcs = [":" + gen],
        output_group = "main",
        target_compatible_with = compat,
    )
    write_source_file(
        name = name,
        visibility = visibility,
        in_file = ":" + name + "_main_out",
        out_file = main_file,
        check_that_out_file_exists = False,
        target_compatible_with = compat,
    )

    if platform == "linux":
        test_file = base + "_" + platform + "_test.go"
        native.filegroup(
            name = name + "_test_out",
            srcs = [":" + gen],
            output_group = "test_file",
            target_compatible_with = compat,
        )
        write_source_file(
            name = name + "_test_file",
            visibility = visibility,
            in_file = ":" + name + "_test_out",
            out_file = test_file,
            check_that_out_file_exists = False,
            target_compatible_with = compat,
        )

INTERNAL_FOR_TESTING = {"relpath": _relpath}

cgo_godefs = macro(
    doc = """Generate Go type definitions from a CGo source file.

    genpost runs `go tool cgo -godefs` on the source file and post-processes it,
    and on Linux also generates alignment test stubs.

    On Linux, the base eBPF headers (pkg/ebpf/c) are prepended
    automatically. Use deps/hdrs for additional headers
    or all headers on Windows.

    The main target (`name`) verifies the generated output matches the
    committed file. Use `bazel test` to verify, `bazel run` to update.

    The committed output files (e.g. `c_types_linux.go`) must be listed
    in `exports_files()` in the same BUILD so that `write_source_file`
    can reference them for comparison.
    """,
    attrs = {
        "src": attr.label(mandatory = True, allow_single_file = [".go"], configurable = False),
        "deps": attr.label_list(default = [], configurable = False),
        "hdrs": attr.label_list(default = [], configurable = False),
        "platform": attr.string(default = "linux", configurable = False, values = ["linux", "windows"]),
    },
    implementation = _cgo_godefs_macro_impl,
)
