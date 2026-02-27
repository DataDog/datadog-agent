load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")
load("@rules_cc//cc/common:cc_shared_library_info.bzl", "CcSharedLibraryInfo")

def _cc_shared_library_wrapper_impl(ctx):
    cc_info = ctx.attr.input[CcInfo]

    # We're only interested in the linker input owned by the target, and ignore those coming from deps.
    # This is what makes most sense for the motivating use case.
    selected_input = None
    for linker_input in cc_info.linking_context.linker_inputs.to_list():
        if linker_input.owner == ctx.attr.input.label:
            selected_input = linker_input
            break

    if selected_input == None:
        fail("No linker inputs found for input label `{}`".format(ctx.attr.input.label))

    return [
        # We could make this configurable by allowing the rule caller to control exports and dynamic_deps
        # for now we don't seem to have this need, so we're defaulting to the most reasonable thing possible.
        CcSharedLibraryInfo(
            exports = [ctx.attr.input.label],
            linker_input = selected_input,
            link_once_static_libs = [],
            dynamic_deps = depset([], order = "topological"),
        ),
    ]

cc_shared_library_wrapper = rule(
    implementation = _cc_shared_library_wrapper_impl,
    attrs = {
        "input": attr.label(
            mandatory = True,
            providers = [CcInfo],
            doc = "Input label from which to generate a `CcSharedLibraryInfo`",
        ),
    },
    doc = """Creates a `CcSharedLibraryInfo` provider out of a `CcInfo` one.
It's use case comes from wanting to wire a rules_foreign_cc target to a cc_shared_library's
dynamic_deps, particularly when said target produces several libraries.""",
)
