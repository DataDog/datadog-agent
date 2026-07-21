"""Shared helpers for extracting CcInfo from cc_library dependencies."""

load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")

def collect_include_dirs(deps):
    """Collect include directories from cc_library dependencies.

    Args:
        deps: list of targets providing CcInfo.

    Returns:
        A struct with three fields (each a list of directory strings):
          includes, system_includes, quote_includes.
    """
    includes = []
    system_includes = []
    quote_includes = []
    for dep in deps:
        if CcInfo in dep:
            ctx = dep[CcInfo].compilation_context
            includes.extend(ctx.includes.to_list())
            system_includes.extend(ctx.system_includes.to_list())
            quote_includes.extend(ctx.quote_includes.to_list())
    return struct(
        includes = includes,
        system_includes = system_includes,
        quote_includes = quote_includes,
    )

def collect_headers(deps):
    """Collect header files from cc_library dependencies.

    Args:
        deps: list of targets providing CcInfo.

    Returns:
        A depset of header Files.
    """
    hdrs = []
    for dep in deps:
        if CcInfo in dep:
            cc_info = dep[CcInfo]
            hdrs.append(cc_info.compilation_context.headers)
    return depset(transitive = hdrs)
