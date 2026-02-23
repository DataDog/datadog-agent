load("@rules_cc//cc/common:cc_common.bzl", "cc_common")
load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")
load("@rules_cc//cc/common:cc_shared_library_info.bzl", "CcSharedLibraryInfo")

def _foreign_cc_shared_wrapper_impl(ctx):
    cc_info = ctx.attr.input[CcInfo]

    # rules_foreign_cc's configure_make carries the full transitive CcInfo of all
    # its own deps.  We only want the linker_input that is owned by the target itself
    # i.e. the shared library it actually produced and nothing from its build-time dependencies.
    selected_input = None
    for linker_input in cc_info.linking_context.linker_inputs.to_list():
        if linker_input.owner == ctx.attr.input.label:
            selected_input = linker_input
            break

    if selected_input == None:
        fail("No linker input owned by `{}` found".format(ctx.attr.input.label))

    return [
        CcInfo(
            compilation_context = cc_info.compilation_context,
            linking_context = cc_common.create_linking_context(
                linker_inputs = depset([selected_input]),
            ),
        ),
        CcSharedLibraryInfo(
            exports = [ctx.attr.input.label],
            linker_input = selected_input,
            link_once_static_libs = [],
            dynamic_deps = depset([], order = "topological"),
        ),
    ]

foreign_cc_shared_wrapper = rule(
    implementation = _foreign_cc_shared_wrapper_impl,
    attrs = {
        "input": attr.label(
            mandatory = True,
            providers = [CcInfo],
            doc = "A rules_foreign_cc target (e.g. configure_make) to wrap.",
        ),
    },
    doc = """Wraps a rules_foreign_cc shared-library target into native Bazel CC providers.

This produces two providers from the target:
  CcInfo
    - Full compilation_context preserved (headers, include paths, defines).
    - linking_context contains only the linker_input owned by the target itself;
      transitive linker inputs from the foreign library's deps are discarded.
    - Use this in deps of cc_library / cc_shared_library to compile and link
      against the foreign library without pulling in its transitive deps.

  CcSharedLibraryInfo
    - Wraps the same owned linker_input.
    - Use this in dynamic_deps of cc_shared_library.
""",
)
