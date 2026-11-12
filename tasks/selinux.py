"""
SELinux namespaced tasks
"""

from invoke import task

from tasks.libs.build.bazel import bazel


@task
def compile_system_probe_policy_file(_):
    """
    Compiles cmd/agent/selinux/system_probe_policy.te into system_probe_policy.pp next to it.

    Wraps `bazel run //cmd/agent/selinux:write_system_probe_policy`; prefer calling Bazel directly.
    """
    bazel("run", "//cmd/agent/selinux:write_system_probe_policy")
