"workspace_root: a rule for executing an executable in the BUILD_WORKSPACE_DIRECTORY"

load("//bazel/multitool/private:run_in.bzl", "run_in", "run_in_attrs")

def _workspace_root_impl(ctx):
    return run_in(ctx, "BUILD_WORKSPACE_DIRECTORY")

workspace_root = rule(
    implementation = _workspace_root_impl,
    attrs = run_in_attrs,
    executable = True,
)
