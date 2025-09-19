"cwd: a rule for executing an executable in the BUILD_WORKING_DIRECTORY"

load("//bazel/multitool/private:run_in.bzl", "run_in", "run_in_attrs")

def _cwd_impl(ctx):
    return run_in(ctx, "BUILD_WORKING_DIRECTORY")

cwd = rule(
    implementation = _cwd_impl,
    attrs = run_in_attrs,
    executable = True,
)
