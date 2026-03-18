"""Shared helpers for extracting CcInfo from cc_library dependencies."""

load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")

def collect_includes(deps):
    """Collect include directories from cc_library dependencies.

    Args:
        deps: list of targets providing CcInfo.

    Returns:
        A list of include directory strings.
    """
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
