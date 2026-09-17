"""check_for_symbols_test: verify a binary's symbol table via `nm`.

Generalizes the legacy Ruby `fips_check_binary_for_expected_symbol` check
(omnibus/lib/fips.rb), which confirmed that a FIPS-tagged build actually
produced a binary containing the expected cgo symbol -- a successful build is
not sufficient proof that the intended code path was compiled in.

Usage:

    check_for_symbols_test(
        name = "installer_fips_symbols_test",
        binary = ":installer",
        must_include = select({
            "//packages/agent:linux_fips": ["_Cfunc__mkcgo_OPENSSL"],
            "//conditions:default": [],
        }),
        must_not_include = select({
            "//packages/agent:linux_fips": [],
            "//conditions:default": ["_Cfunc__mkcgo_OPENSSL"],
        }),
    )

The `binary` under test is built with `//:release` forced to False so its
symbol table survives the release strip flags (`-s -w`) that `dd_agent_go_binary`
otherwise applies -- without this, `nm` would see an empty symbol table and
every check would be meaningless.
"""

load("@bazel_skylib//rules:build_test.bzl", "build_test")

def _unstripped_binary_transition_impl(settings, attr):
    # Force an unstripped build of the binary under test: dd_agent_go_binary
    # adds "-s -w" gc_linkopts whenever //:release is True (which is the
    # default for every invocation in this repo, see .bazelrc), which removes
    # the symbol table that `nm` depends on.
    return {"//:release": False}

_unstripped_binary_transition = transition(
    implementation = _unstripped_binary_transition_impl,
    inputs = [],
    outputs = ["//:release"],
)

def _check_for_symbols_impl(ctx):
    binary = ctx.file.binary

    if not ctx.attr.must_include and not ctx.attr.must_not_include:
        fail("check_for_symbols_test: at least one of 'must_include' or 'must_not_include' must be non-empty")

    out = ctx.actions.declare_file(ctx.label.name + ".ok")

    args = ctx.actions.args()
    args.add("--nm", ctx.executable._nm)
    args.add("--binary", binary)
    for pattern in ctx.attr.must_include:
        args.add("--must-include", pattern)
    for pattern in ctx.attr.must_not_include:
        args.add("--must-not-include", pattern)
    args.add("--output", out)

    ctx.actions.run(
        executable = ctx.executable._checker,
        arguments = [args],
        inputs = [binary],
        tools = [ctx.executable._nm],
        outputs = [out],
        mnemonic = "CheckForSymbols",
        progress_message = "Checking symbols in %s" % binary.short_path,
    )

    return [DefaultInfo(files = depset([out]))]

check_for_symbols = rule(
    implementation = _check_for_symbols_impl,
    attrs = {
        "binary": attr.label(
            mandatory = True,
            allow_single_file = True,
            doc = "The (Go) binary target whose symbol table should be inspected. Must produce exactly one default output file.",
            cfg = _unstripped_binary_transition,
        ),
        "must_include": attr.string_list(
            default = [],
            doc = "Regex patterns that must each match at least one line of `nm` output.",
        ),
        "must_not_include": attr.string_list(
            default = [],
            doc = "Regex patterns that must not match any line of `nm` output.",
        ),
        "_checker": attr.label(
            default = "//bazel/rules/check_for_symbols:checker",
            executable = True,
            cfg = "exec",
        ),
        "_nm": attr.label(
            default = "@llvm_toolchain_llvm//:nm",
            executable = True,
            cfg = "exec",
        ),
        "_allowlist_function_transition": attr.label(
            default = "@bazel_tools//tools/allowlists/function_transition_allowlist",
        ),
    },
)

def check_for_symbols_test(name, binary, must_include = None, must_not_include = None, **kwargs):
    """Verifies a binary's symbol table via `nm`.

    Fails if any `must_include` pattern is absent from the symbol table, or
    any `must_not_include` pattern is present. Generalizes the legacy
    `fips_check_binary_for_expected_symbol` Ruby check (omnibus/lib/fips.rb).

    Args:
      name: test target name.
      binary: label of the Go binary to inspect. Must produce exactly one
        default output file.
      must_include: list (or select()) of regex patterns that must each be
        present in the symbol table.
      must_not_include: list (or select()) of regex patterns that must each
        be absent from the symbol table.
      **kwargs: forwarded to the underlying `build_test` (e.g.
        target_compatible_with, tags).
    """
    check_name = name + "_check"
    check_kwargs = {}
    if "target_compatible_with" in kwargs:
        check_kwargs["target_compatible_with"] = kwargs["target_compatible_with"]

    check_for_symbols(
        name = check_name,
        binary = binary,
        must_include = must_include or [],
        must_not_include = must_not_include or [],
        testonly = True,
        tags = ["manual"],
        **check_kwargs
    )
    build_test(
        name = name,
        targets = [":" + check_name],
        **kwargs
    )
