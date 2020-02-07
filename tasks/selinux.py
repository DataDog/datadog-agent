"""
SELinux namespaced tasks
"""
import os

import invoke
from invoke import task

@task
def compile_policy_file(ctx):
  policy_dir = os.path.join(".", "cmd", "agent", "selinux")
  policy_name = "system_probe_policy"
  command = "checkmodule -M -m -o {0}.mod {0}.te".format(os.path.join(policy_dir, policy_name))
  ctx.run(command)
  command = "semodule_package -o {0}.pp -m {0}.mod".format(os.path.join(policy_dir, policy_name))
  ctx.run(command)