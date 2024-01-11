import os

from .kmt_os import get_kmt_os
from .stacks import find_ssh_key
from .tool import Exit, error, info


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
            res = run_cmd_remote(self.ctx, cmd, self.remote_ip, self.vm.ip, self.remote_ssh_key, log_debug)

        if not res.ok:
            error(f"[-] Failed: {self.vm.ip} -> {cmd}")
            if not allow_fail:
                print_failed(res.stderr)
                raise Exit("command failed")

    def copy_files(self, path, dest=None):
        ddvm_rsa = os.path.join(get_kmt_os().kmt_dir, "ddvm_rsa")
        if self.vm.arch == "local" and dest is None:
            self.ctx.run(f"cp {path} {get_kmt_os().shared_dir}")
        elif self.vm.arch == "local" and dest is not None:
            self.ctx.run(f"scp -o StrictHostKeyChecking=no -i {ddvm_rsa} {path} root@{self.vm.ip}:{dest}")
        else:
            if self.remote_ssh_key == "" or self.remote_ip == "":
                raise Exit("remote ssh key and remote ip are required to run command on remote VMs")
            self.run_cmd(
                f"scp -o StrictHostKeyChecking=no -i {self.remote_ssh_key} {path} ubuntu@{self.remote_ip}:/opt/kernel-version-testing/",
                False,
            )

    def sync_source(self, source, target):
        sync_source(self.ctx, source, target, self.remote_ip, self.remote_ssh_key, self.vm.ip, self.vm.arch)


def sync_source(ctx, source, target, instance_ip, ssh_key, ip, arch):
    kmt_dir = get_kmt_os().kmt_dir
    vm_copy = f"rsync -e \\\"ssh -o StrictHostKeyChecking=no -i {kmt_dir}/ddvm_rsa\\\" --chmod=F644 --chown=root:root -rt --exclude='.git*' --filter=':- .gitignore' ./ root@{ip}:{target}"
    if arch == "local":
        ctx.run(
            f"rsync -e \"ssh -o StrictHostKeyChecking=no -i {kmt_dir}/ddvm_rsa\" -p --chown=root:root -rt --exclude='.git*' --filter=':- .gitignore' {source} root@{ip}:{target}"
        )
    elif arch == "x86_64" or arch == "arm64":
        ctx.run(
            f"rsync -e \"ssh -o StrictHostKeyChecking=no -i {ssh_key}\" -p --chown=root:root -rt --exclude='.git*' --filter=':- .gitignore' {source} ubuntu@{instance_ip}:/home/ubuntu/datadog-agent"
        )
        ctx.run(
            f"ssh -i {ssh_key} -o StrictHostKeyChecking=no ubuntu@{instance_ip} \"cd /home/ubuntu/datadog-agent && {vm_copy}\""
        )
    else:
        raise Exit(f"Unsupported arch {arch}")


def run_cmd_local(ctx, cmd, ip, log_debug):
    ddvm_rsa = os.path.join(get_kmt_os().kmt_dir, "ddvm_rsa")
    return ctx.run(
        f"ssh -o StrictHostKeyChecking=no -i {ddvm_rsa} root@{ip} '{cmd}'",
        warn=True,
        hide=(not log_debug),
    )


def run_cmd_remote(ctx, cmd, remote_ip, ip, ssh_key, log_debug):
    if ssh_key == "" or remote_ip == "":
        raise Exit("remote ssh key and remote ip are required to run command on remote VMs")
    return ctx.run(
        f"ssh -o StrictHostKeyChecking=no -i {ssh_key} ubuntu@{remote_ip} \"ssh -o StrictHostKeyChecking=no -i /home/kernel-version-testing/ddvm_rsa root@{ip} '{cmd}'\"",
        warn=True,
        hide=(not log_debug),
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
        ssh_key_path = find_ssh_key(ssh_key)

    return ssh_key_path
