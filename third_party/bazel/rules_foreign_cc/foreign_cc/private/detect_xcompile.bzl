# buildifier: disable=module-docstring
load(
    "//foreign_cc/private/framework:platform.bzl",
    "arch_name",
    "os_name",
    "target_arch_name",
    "target_os_name",
    "triplet_name",
)

def detect_xcompile(ctx):
    """A helper function for detecting and setting autoconf-style xcompile flags

    Args:
        ctx (ctx): The current rule's context object

    Returns:
        list(str): The flags to set, or None if xcompiling is disabled
    """

    if not ctx.attr.configure_xcompile:
        return None

    host_triplet = triplet_name(
        os_name(ctx),
        arch_name(ctx),
    )

    if host_triplet == "unknown":
        # buildifier: disable=print
        print("host is unknown; please update foreign_cc/private/framework/platform.bzl; triggered by", ctx.label)
        return None

    target_triplet = triplet_name(
        target_os_name(ctx),
        target_arch_name(ctx),
    )

    if target_triplet == "unknown":
        # buildifier: disable=print
        print("target is unknown; please update foreign_cc/private/framework/platform.bzl; triggered by", ctx.label)
        return None

    if target_triplet == host_triplet:
        return None

    # We pass both --host and --build here, even though we technically only
    # need to pass --host. This is because autotools compares them (without
    # normalization) to determine if a build is a cross-compile
    #
    # If we don't pass --build, autotools will populate it itself, and it might
    # be normalized such that autotools thinks it's a cross-compile, but it
    # shouldn't be.
    #
    # An example of this is if we pass --host=x86_64-pc-linux-gnu but the
    # target compiler thinks it's for x86_64-unknown-linux-gnu; if we don't
    # pass --build, that will incorrectly be considered a cross-compile.
    #
    # Also, no, this isn't backwards. --host means target
    # https://www.gnu.org/software/automake/manual/html_node/Cross_002dCompilation.html
    return ["--host=" + target_triplet, "--build=" + host_triplet]
