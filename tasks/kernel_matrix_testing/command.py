import platform

from .tool import Exit, ask, info, warn, error
from .download import arch_mapping
from .kmt_os import get_kmt_os

class CommandRunner:
    def __init__(self, ctx, local, vm, remote_ip, remote_ssh_key, log_debug):
        self.local = local
        self.remote_ip = remote_ip
        self.remote_ssh_key = ssh_key_to_path(remote_ssh_key)
        self.vm = vm
        self.ctx = ctx
        self.log_debug = log_debug

    def run_cmd(self, cmd, allow_fail=False, verbose=False):
        log_debug = self.log_debug or verbose
        info(f"[+] {self.vm.ip} -> {cmd}")
        if self.vm.arch == "local":
            res = run_cmd_local(self.ctx, cmd, self.vm.ip, log_debug)
        else:
            res = run_cmd_remote(ctx, cmd, self.remote_ip, self.vm.ip, self.remote_ssh_key, log_debug)

        if not res.ok:
            error(f"[-] Failed: {self.vm.ip} -> {cmd}")
            if not allow_fail:
                print_failed(res.stderr)
                raise Exit("command failed")

    def copy_files(self, stack, path):
        if self.vm.arch == "local":
            arch = arch_mapping[platform.machine()]
            self.ctx.run(f"cp kmt-deps/{stack}/dependencies-{arch}.tar.gz {get_kmt_os().shared_dir}")
        else:
            if self.remote_ssh_key == "" or self.remote_ip == "":
                raise Exit("remote ssh key and remote ip are required to run command on remote VMs")
            self.run_cmd(
                f"scp -o StrictHostKeyChecking=no -i {self.remote_ssh_key} kmt-deps/{stack}/dependencies-{self.vm.arch}.tar.gz ubuntu@{self.remote_ip}:/opt/kernel-version-testing/", False
            )

def run_cmd_local(ctx, cmd, ip, log_debug):
    return ctx.run(
        f"ssh -o StrictHostKeyChecking=no -i /home/kernel-version-testing/ddvm_rsa root@{ip} '{cmd}'", warn=True, hide=(not log_debug),
    )

def run_cmd_remote(ctx, cmd, remote_ip, ip, ssh_key, log_debug):
    if ssh_key == "" or remote_ip == "":
        raise Exit("remote ssh key and remote ip are required to run command on remote VMs")
    return ctx.run(
        f"ssh -o StrictHostKeyChecking=no -i {ssh_key} ubuntu@{remote_ip} \"ssh -o StrictHostKeyChecking=no -i /home/kernel-version-testing/ddvm_rsa root@{ip} '{cmd}'\"", warn=True, hide=(not log_debug),
    )

def print_failed(output):
    out = list()
    for line in output.split("\n"):
        out.append(f"\t{line}")
    error('\n'.join(out))

def ssh_key_to_path(ssh_key):
    ssh_key_path = ""
    if ssh_key != "":
        ssh_key.rstrip(".pem")
        ssh_key_path = stacks.find_ssh_key(ssh_key)

    return ssh_key_path
