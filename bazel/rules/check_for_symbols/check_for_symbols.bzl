"""check_for_symbols_test: verify a binary's symbol table.

Checks a single binary target for the presense or absence of a one or
more symbols.

It may be used to determine that a specific library is linked in or not.

Based on the legacy Ruby `fips_check_binary_for_expected_symbol` check
(omnibus/lib/fips.rb), which confirmed that a FIPS-tagged build actually
produced a binary containing the expected cgo symbol.

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
"""

load("@bazel_skylib//rules:build_test.bzl", "build_test")

def _check_for_symbols_impl(ctx):
    binary = ctx.file.binary

    out = ctx.actions.declare_file(ctx.label.name + ".status")

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
            doc = "The binary target whose symbol table should be inspected. Must produce exactly one default output file.",
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
