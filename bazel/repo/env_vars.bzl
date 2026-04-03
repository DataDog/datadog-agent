"""Import selected build environment variables to a repo where we can use them.

This method allows us to pull in an isolated set of variables without
polluting the build configuration and breaking caching.

Bazel will treat changes to the value of these variables across builds
as a change to the repository, and cause the repo to be rebuilt, so it
is important to depend on these from as few places, and as high in the
build graph, as possible.  That is, it is acceptable to use them from
final packaging steps, but not to use for select clauses, or as input
to libraries and binaries.

Usage:
    env_vars = use_repo_rule("//bazel/repo:env_vars.bzl", "env_vars")
    env_vars(
        name = "agent_volatile",
        variables = [
            "FORCED_PACKAGE_COMPRESSION_LEVEL",
            "PACKAGE_VERSION",
        ],
    )

Future ideas:

1. Add a rule in the .bzl file that provides rules_pkg PackageVariables. That
   makes them immediately usable for things like the .deb file name.

2. If we find that we need this technique more often, and it becomes a caching problem,
we could consider emitting one .bzl file per variable. so PACKAGE_VERSION would be
found in package_version.bzl. But that is overly complex until we prove we need it.
"""

BUILD_FILE_CONTENT = """
exports_files(
    ["env_vars.bzl"],
    # We may expand this in the future, but let's limit inventiveness for now.
    visibility = [
        "@agent//bazel/rules:__subpackages__",
        "@agent//packages:__subpackages__",
    ],
)
"""

VARIABLE_FILE_CONTENT = """

env_vars = struct(
%s
)
"""

def _env_vars_impl(rctx):
    rctx.file("BUILD.bazel", BUILD_FILE_CONTENT)
    var_list = []
    for var in rctx.attr.variables:
        value = rctx.getenv(var)
        rctx.report_progress("Importing %s='%s'" % (var, value))

        # Not importing floats.  If we are doing that, we are thinking wrong.
        if value == None:
            var_list.append("    %s = None" % var)
        elif type(value) == "int":
            var_list.append("    %s = %d" % (var, value))
        else:
            var_list.append("    %s = \"%s\"" % (var, value))

    rctx.file("env_vars.bzl", VARIABLE_FILE_CONTENT % ",\n".join(var_list))

env_vars = repository_rule(
    implementation = _env_vars_impl,
    attrs = {
        "variables": attr.string_list(
            doc = "List of environment variables to export.",
        ),
    },
    configure = True,
    doc = """Import environment variables into a build.""",
)
