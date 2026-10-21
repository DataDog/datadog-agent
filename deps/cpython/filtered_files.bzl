"""Filters source files by path."""

def _filtered_files_impl(ctx):
    files = []
    for file in ctx.files.srcs:
        path = file.path[len(file.owner.workspace_root) + 1:] if file.owner.workspace_root else file.path
        if not any([path.startswith(prefix) for prefix in ctx.attr.exclude_prefixes]):
            files.append(file)
    return [DefaultInfo(files = depset(files))]

filtered_files = rule(
    implementation = _filtered_files_impl,
    doc = "Returns srcs minus the source files whose path in their repository starts with one of exclude_prefixes.",
    attrs = {
        "exclude_prefixes": attr.string_list(mandatory = True),
        "srcs": attr.label_list(allow_files = True, mandatory = True),
    },
)
