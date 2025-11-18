"""A helper module containing tools for detecting platform information"""

SUPPORTED_CPU = [
    "aarch64",
    "ppc64le",
    "s390x",
    "wasm32",
    "wasm64",
    "x86_64",
]

SUPPORTED_OS = [
    "android",
    "emscripten",
    "freebsd",
    "ios",
    "linux",
    "macos",
    "none",
    "openbsd",
    "qnx",
    "tvos",
    "wasi",
    "watchos",
    "windows",
]

PLATFORM_CONSTRAINTS_RULE_ATTRIBUTES = {
    "_{}_constraint".format(i): attr.label(default = Label("@platforms//os:{}".format(i)))
    for i in SUPPORTED_OS
}

# this would be cleaner as x | y, but that's not supported in bazel 5.4.0
PLATFORM_CONSTRAINTS_RULE_ATTRIBUTES.update({
    "_{}_constraint".format(i): attr.label(default = Label("@platforms//cpu:{}".format(i)))
    for i in SUPPORTED_CPU
})

ForeignCcPlatformInfo = provider(
    doc = "A provider containing information about the current platform",
    fields = {
        "cpu": "The platform cpu",
        "os": "The platform os",
    },
)

def _framework_platform_info_impl(ctx):
    """The implementation of `framework_platform_info`

    Args:
        ctx (ctx): The rule's context object

    Returns:
        list: A provider containing platform info
    """
    return [ForeignCcPlatformInfo(
        os = ctx.attr.os,
        cpu = ctx.attr.cpu,
    )]

_framework_platform_info = rule(
    doc = "A rule defining platform information used by the foreign_cc framework",
    implementation = _framework_platform_info_impl,
    attrs = {
        "cpu": attr.string(
            doc = "The platform's cpu",
        ),
        "os": attr.string(
            doc = "The platform's operating system",
        ),
    },
)

def framework_platform_info(name = "platform_info"):
    """Define a target containing platform information used in the foreign_cc framework

    Args:
      name: A unique name for this target.
    """

    # this would be cleaner as x | y, but that's not supported in bazel 5.4.0
    select_os = {
        "@platforms//os:{}".format(i): i
        for i in SUPPORTED_OS
    }
    select_os.update({
        "//conditions:default": "unknown",
    })

    select_cpu = {
        "@platforms//cpu:{}".format(i): i
        for i in SUPPORTED_CPU
    }
    select_cpu.update({
        "//conditions:default": "unknown",
    })

    _framework_platform_info(
        name = name,
        os = select(select_os),
        cpu = select(select_cpu),
        visibility = ["//visibility:public"],
    )

def os_name(ctx):
    """A helper function for getting the operating system name from a `ForeignCcPlatformInfo` provider

    Args:
        ctx (ctx): The current rule's context object

    Returns:
        str: The string of the current platform
    """
    platform_info = getattr(ctx.attr, "_foreign_cc_framework_platform")
    if not platform_info:
        return "unknown"

    return platform_info[ForeignCcPlatformInfo].os

def arch_name(ctx):
    """A helper function for getting the arch name from a `ForeignCcPlatformInfo` provider

    Args:
        ctx (ctx): The current rule's context object

    Returns:
        str: The string of the current platform
    """
    platform_info = getattr(ctx.attr, "_foreign_cc_framework_platform")
    if not platform_info:
        return "unknown"

    return platform_info[ForeignCcPlatformInfo].cpu

def target_arch_name(ctx):
    """A helper function for getting the target architecture name based on the constraints

    Args:
        ctx (ctx): The current rule's context object

    Returns:
        str: The string of the current platform
    """
    for arch in SUPPORTED_CPU:
        constraint = getattr(ctx.attr, "_{}_constraint".format(arch))
        if constraint and ctx.target_platform_has_constraint(constraint[platform_common.ConstraintValueInfo]):
            return arch

    return "unknown"

def target_os_name(ctx):
    """A helper function for getting the target operating system name based on the constraints

    Args:
        ctx (ctx): The current rule's context object

    Returns:
        str: The string of the current platform
    """
    for os in SUPPORTED_OS:
        constraint = getattr(ctx.attr, "_{}_constraint".format(os))
        if constraint and ctx.target_platform_has_constraint(constraint[platform_common.ConstraintValueInfo]):
            return os

    return "unknown"

def triplet_name(os, arch):
    """A helper function for getting the platform triplet from the results of the above arch/os functions

    Args:
        os (str): the os
        arch (str): the arch

    Returns:
        str: The string of the current platform
    """

    # This is like a simplified config.guess / config.sub from autotools
    if os == "linux":
        # The linux values here were what config.guess returns on ubuntu:22.04;
        # specifically, this version:
        # https://git.savannah.gnu.org/gitweb/?p=config.git;a=blob;f=config.guess;hb=00b15927496058d23e6258a28d8996f87cf1f191
        #
        # bazel doesn't have common libc constraints, which makes it difficult
        # to guess what the correct value for the last field might be (it would
        # be musl on alpine, for example). This doesn't change what the
        # compiler itself will do, though, so as long as we normalize
        # consistently, I don't think this will break alpine.
        if arch == "aarch64":
            return "aarch64-unknown-linux-gnu"
        elif arch == "ppc64le":
            return "powerpc64le-unknown-linux-gnu"
        elif arch == "s390x":
            return "s390x-ibm-linux-gnu"
        elif arch == "x86_64":
            return "x86_64-pc-linux-gnu"

    elif os == "macos":
        # These are _not_ what config.guess would return for darwin;
        # config.guess puts the release version (the result of uname -r) in the
        # field, e.g.  darwin23.4.0.
        #
        # The OS field is unnormalized and any dev can write a check that does
        # arbitrary inspection of it. Examples of these:
        # - libffi has a custom macro
        #   (https://github.com/libffi/libffi/blob/8e3ef965c2d0015ed129a06d0f11f30c2120a413/acinclude.m4#L40)
        #   that doesn't handle macos, just darwin, so that's unsafe
        # - some versions of libtool (like this version in the gcc tree:
        #   https://github.com/gcc-mirror/gcc/blob/3f1e15e885185ad63a67c7fe423d2a0b4d8da101/libtool.m4#L1071)
        #   check for darwin2*, not just darwin, so returning it without the version isn't good either.
        #
        # Currently, this returns darwin21, which is Monterey, the current
        # oldest non-eol version of darwin. (You can look that up here:
        # https://en.wikipedia.org/wiki/Darwin_(operating_system)

        if arch == "aarch64":
            return "aarch64-apple-darwin21"
        elif arch == "x86_64":
            return "x86_64-apple-darwin21"

    elif os == "emscripten":
        if arch == "wasm32":
            return "wasm32-unknown-emscripten"
        elif arch == "wasm64":
            return "wasm64-unknown-emscripten"

    return "unknown"
