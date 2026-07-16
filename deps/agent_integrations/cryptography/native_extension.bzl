"""Helpers for shaping cryptography's native extension for wheels."""

_SHARED_LIBRARY_EXTENSIONS = [
    ".dll",
    ".dylib",
    ".so",
]

def _is_shared_library(file):
    for extension in _SHARED_LIBRARY_EXTENSIONS:
        if file.basename.endswith(extension):
            return True
    return False

def _pick_shared_library(files, output_basename):
    if output_basename.endswith(".pyd"):
        candidates = [file for file in files if file.basename.endswith(".dll")]
    else:
        candidates = [file for file in files if _is_shared_library(file)]

    if len(candidates) != 1:
        fail("Expected exactly one shared library for {}, got: {}".format(
            output_basename,
            [file.short_path for file in files],
        ))

    return candidates[0]

def _copy_native_extension_impl(ctx):
    output = ctx.actions.declare_file(ctx.attr.out)
    input = _pick_shared_library(ctx.files.src, output.basename)

    ctx.actions.run(
        executable = ctx.executable._copy_tool,
        inputs = [input],
        outputs = [output],
        arguments = [input.path, output.path],
        mnemonic = "CopyNativeExtension",
        progress_message = "Copying native extension {}".format(output.short_path),
    )

    return DefaultInfo(files = depset([output]))

copy_native_extension = rule(
    implementation = _copy_native_extension_impl,
    attrs = {
        "src": attr.label(
            mandatory = True,
            allow_files = True,
            doc = "Target producing the platform shared library to copy/rename.",
        ),
        "out": attr.string(
            mandatory = True,
            doc = "Output basename for the wheel extension, e.g. _rust.abi3.so or _rust.pyd.",
        ),
        "_copy_tool": attr.label(
            default = Label("//deps/agent_integrations/cryptography:copy_file_tool"),
            executable = True,
            cfg = "exec",
        ),
    },
)
