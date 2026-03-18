"""Bazel rule for generating Go type definitions from C headers via cgo -godefs."""

load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")
load("@rules_go//go:def.bzl", "go_context")

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

def _collect_includes(deps):
    """Collect include directories from cc_library dependencies."""
    dirs = []
    for dep in deps:
        if CcInfo in dep:
            cc_info = dep[CcInfo]
            for inc in cc_info.compilation_context.includes.to_list():
                dirs.append(inc)
            for inc in cc_info.compilation_context.system_includes.to_list():
                dirs.append(inc)
            for inc in cc_info.compilation_context.quote_includes.to_list():
                dirs.append(inc)
    return dirs

def _collect_headers(deps):
    """Collect header files from cc_library dependencies."""
    hdrs = []
    for dep in deps:
        if CcInfo in dep:
            cc_info = dep[CcInfo]
            hdrs.append(cc_info.compilation_context.headers)
    return depset(transitive = hdrs)

def _cgo_godefs_impl(ctx):
    go = go_context(ctx)

    src = ctx.file.src
    genpost = ctx.executable._genpost

    platform = go.sdk.goos
    base = src.basename.removesuffix(".go")
    out_name = base + "_" + platform + ".go"
    out = ctx.actions.declare_file(out_name)
    outputs = [out]

    include_dirs = _collect_includes(ctx.attr.deps)
    headers = _collect_headers(ctx.attr.deps)
    src_dir = src.dirname

    # Filter out bazel-out and root dirs; keep only source-tree includes.
    # CcInfo adds generated output dirs that don't affect correctness but
    # would pollute the cgo -godefs comment line embedded in the output.
    source_includes = [d for d in include_dirs if not d.startswith("bazel-out") and d != "."]
    rel_includes = []
    for d in source_includes:
        r = _relpath(d, src_dir)
        if r not in rel_includes:
            rel_includes.append(r)
    include_flags = " ".join(["-I " + d for d in rel_includes])

    genpost_args = ""
    if platform == "linux":
        test_name = base + "_" + platform + "_test.go"
        test_out = ctx.actions.declare_file(test_name)
        outputs.append(test_out)
        test_path_no_ext = test_out.path.removesuffix(".go")
        package_name = ctx.label.package.split("/")[-1]
        genpost_args = "$ROOT/{test} {pkg}".format(test = test_path_no_ext, pkg = package_name)

    cmd = (
        "ROOT=$PWD && cd {src_dir} && " +
        "GOROOT=$ROOT/{goroot} CC=clang $ROOT/{go} tool cgo -godefs -- {includes} -fsigned-char {src_file} | " +
        "$ROOT/{genpost} {genpost_args} > $ROOT/{out}"
    ).format(
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

    return [DefaultInfo(files = depset(outputs))]

_cgo_godefs = rule(
    implementation = _cgo_godefs_impl,
    attrs = {
        "src": attr.label(
            mandatory = True,
            allow_single_file = [".go"],
            doc = "The Go source file containing C type references (import \"C\").",
        ),
        "deps": attr.label_list(
            providers = [CcInfo],
            doc = "cc_library targets providing the C headers included by src.",
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

def cgo_godefs(name, src, deps = [], **kwargs):
    """Generate Go type definitions from a CGo source file.

    Creates a target that runs `go tool cgo -godefs` on the source file,
    post-processes the output with genpost, and on Linux also generates
    alignment test stubs.

    Args:
        name: Target name.
        src: The Go source file with `import "C"` and C type references.
        deps: cc_library targets providing the required C headers.
        **kwargs: Additional arguments passed to the underlying rule.
    """
    _cgo_godefs(
        name = name,
        src = src,
        deps = deps,
        target_compatible_with = select({
            "@platforms//os:linux": [],
            "@platforms//os:windows": [],
            "//conditions:default": ["@platforms//:incompatible"],
        }),
        **kwargs
    )
