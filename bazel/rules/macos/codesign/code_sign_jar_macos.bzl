"""Bazel rule for code-signing JAR files on macOS.

This rule signs native libraries (.so, .dylib, .jnilib) inside a JAR file
using the macOS codesign utility. It supports optional hardened runtime
entitlements and can skip signing entirely if configured.

The signing identity and entitlements file default to values from
bazel/constants.bzl but can be overridden per-rule.

Environment variables:
  - SIGN_MAC: If "true", performs code signing. If not set or "false", just copies the JAR
  - HARDENED_RUNTIME_MAC: If "true", applies hardened runtime entitlements
"""

load("@agent_volatile//:env_vars.bzl", "env_vars")
load("//bazel:constants.bzl", "apple_signing_identity")

def _code_sign_jar_macos_impl(ctx):
    """Implementation of code_sign_jar_macos rule."""

    input_jar = ctx.file.jar
    output_jar = ctx.outputs.output

    # Get signing parameters, using defaults from constants if not provided
    signing_identity = ctx.attr.signing_identity or apple_signing_identity

    # Check if signing should be performed
    do_signing = env_vars.SIGN_MAC == "true"
    hardened_runtime = env_vars.HARDENED_RUNTIME_MAC == "true"

    if not do_signing:
        # Just copy the JAR without signing
        args = ctx.actions.args()
        args.add(input_jar)
        args.add(output_jar)

        ctx.actions.run(
            inputs = [input_jar],
            outputs = [output_jar],
            executable = "/bin/cp",
            arguments = [args],
            mnemonic = "CopyJar",
            progress_message = "Copying JAR (signing skipped): {}".format(input_jar.short_path),
        )
    else:
        # Sign the JAR
        sign_script = ctx.executable._sign_script

        # Build arguments for the signing script
        args = ctx.actions.args()
        args.add(input_jar)
        args.add(output_jar)
        args.add(signing_identity)
        if hardened_runtime:
            args.add(ctx.file.entitlements_file.path)

        ctx.actions.run(
            inputs = [input_jar, ctx.file.entitlements_file],
            outputs = [output_jar],
            tools = [sign_script],
            executable = sign_script,
            arguments = [args],
            mnemonic = "CodeSignJarMacOS",
            progress_message = "Code-signing JAR: {}".format(input_jar.short_path),
        )

    return [DefaultInfo(files = depset([output_jar]))]

code_sign_jar_macos = rule(
    implementation = _code_sign_jar_macos_impl,
    attrs = {
        "jar": attr.label(
            mandatory = True,
            allow_single_file = [".jar"],
            doc = "The JAR file to sign.",
        ),
        "output": attr.output(
            mandatory = True,
            doc = "The output file path for the signed JAR.",
        ),
        "signing_identity": attr.string(
            doc = "The macOS code signing identity (e.g., 'Developer ID Application: ...'). " +
                  "If not provided, defaults to apple_signing_identity from bazel/constants.bzl.",
        ),
        "entitlements_file": attr.label(
            doc = "Path to entitlements file for hardened runtime. " +
                  "If not provided, defaults to apple_entitlements_file from bazel/constants.bzl. " +
                  "Only used if HARDENED_RUNTIME_MAC environment variable is set to 'true'.",
            default = Label("//bazel/rules/macos/codesign:Entitlements.plist"),
            allow_single_file = True,
        ),
        "_sign_script": attr.label(
            default = Label("//bazel/rules/macos/codesign:sign_jar_macos"),
            executable = True,
            cfg = "exec",
        ),
    },
    doc = """Code-signs native libraries within a JAR file on macOS.

    This rule unpacks a JAR, signs all native libraries (.so, .dylib, .jnilib),
    and repacks the JAR. The original JAR is never modified.

    Environment variables:
      - SIGN_MAC: Set to "true" to sign (otherwise just copy the JAR)
      - HARDENED_RUNTIME_MAC: Set to "true" to apply hardened runtime entitlements

    Example:
        code_sign_jar_macos(
            name = "signed_jar",
            jar = ":mylib.jar",
            output = "signed/mylib.jar",
        )
    """,
)
