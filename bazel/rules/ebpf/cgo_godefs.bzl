"""Bazel rule for generating Go type definitions from C headers via cgo -godefs."""

load("@bazel_lib//lib:write_source_files.bzl", "write_source_file")
load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")
load("@rules_go//go:def.bzl", "go_context")
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
    src_dir = src.dirname

    # Filter out bazel-out and root dirs; keep only source-tree includes.
    # CcInfo adds generated output dirs that don't affect correctness but
    # would pollute the cgo -godefs comment line embedded in the output.
    def _filter_and_rel(dirs):
        result = []
        for d in dirs:
            if d.startswith("bazel-out") or d == ".":
                continue
            r = _relpath(d, src_dir)
            if r not in result:
                result.append(r)
        return result

    include_flags = " ".join(
        ["-I " + d for d in _filter_and_rel(inc.includes)] +
        ["-isystem " + d for d in _filter_and_rel(inc.system_includes)] +
        ["-iquote " + d for d in _filter_and_rel(inc.quote_includes)],
    )

    genpost_args = ""
    test_out = None
    if platform == "linux":
        test_name = "_" + base + "_" + platform + "_test.go"
        test_out = ctx.actions.declare_file(test_name)
        outputs.append(test_out)
        test_path_no_ext = test_out.path.removesuffix(".go")
        package_name = ctx.label.package.split("/")[-1]
        genpost_args = "$ROOT/{test} {pkg}".format(test = test_path_no_ext, pkg = package_name)

    # TODO(ABLD-410): uses the system clang rather than a hermetic toolchain.
    # On Windows, Go defaults to gcc (MinGW) — no CC override needed, matching
    # the old ninja behavior.
    cc_prefix = "CC=clang " if platform == "linux" else ""

    cmd = (
        "ROOT=$PWD && cd {src_dir} && " +
        "GOROOT=$ROOT/{goroot} {cc_prefix}$ROOT/{go} tool cgo -godefs -- {includes} -fsigned-char {src_file} | " +
        "$ROOT/{genpost} {genpost_args} > $ROOT/{out}"
    ).format(
        cc_prefix = cc_prefix,
        goroot = go.sdk.root_file.dirname,
        src_dir = src.dirname,
        go = go.go.path,
        includes = include_flags,
        src_file = src.basename,
        genpost = genpost.path,
        genpost_args = genpost_args,
        out = out.path,
    )

    env = dict(go.env)
    env["CGO_ENABLED"] = "1"

    # run_shell needed: genpost reads stdin and writes stdout, so the pipeline
    # `go tool cgo -godefs | genpost > out` requires shell pipe/redirect.
    ctx.actions.run_shell(
        outputs = outputs,
        inputs = depset(
            [src, go.go],
            transitive = [headers, go.sdk.tools, go.sdk.srcs, go.sdk.libs, go.cc_toolchain_files],
        ),
        tools = [genpost],
        command = cmd,
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

_cgo_godefs = rule(
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
        "_genpost": attr.label(
            default = "//pkg/ebpf/cgo:genpost",
            executable = True,
            cfg = "exec",
        ),
        "_go_context_data": attr.label(
            default = "@rules_go//:go_context_data",
        ),
    },
    toolchains = ["@rules_go//go:toolchain"],
)

_STD_LINUX_DEPS = [
    "//pkg/ebpf/c:ebpf_c_headers",
]

def _cgo_godefs_macro_impl(name, visibility, src, deps, hdrs, platform):
    all_deps = deps + (_STD_LINUX_DEPS if platform == "linux" else [])

    gen = name + "_gen"
    _cgo_godefs(
        name = gen,
        src = src,
        deps = all_deps,
        hdrs = hdrs,
        target_compatible_with = select({
            "@platforms//os:{}".format(platform): [],
            "//conditions:default": ["@platforms//:incompatible"],
        }),
    )

    base = src.name.removesuffix(".go")
    main_file = base + "_" + platform + ".go"

    native.filegroup(
        name = name + "_main_out",
        srcs = [":" + gen],
        output_group = "main",
    )
    write_source_file(
        name = name,
        visibility = visibility,
        in_file = ":" + name + "_main_out",
        out_file = main_file,
        check_that_out_file_exists = False,
    )

    if platform == "linux":
        test_file = base + "_" + platform + "_test.go"
        native.filegroup(
            name = name + "_test_out",
            srcs = [":" + gen],
            output_group = "test_file",
        )
        write_source_file(
            name = name + "_test_file",
            visibility = visibility,
            in_file = ":" + name + "_test_out",
            out_file = test_file,
            check_that_out_file_exists = False,
        )

INTERNAL_FOR_TESTING = {"relpath": _relpath}

cgo_godefs = macro(
    doc = """Generate Go type definitions from a CGo source file.

    Runs `go tool cgo -godefs` on the source file, post-processes with genpost,
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
