from __future__ import annotations

import glob
import json
import os
from typing import TYPE_CHECKING, Dict, List, Optional

from invoke.context import Context

from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.tool import Exit, ask, error

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import ArchOrLocal, PathOrStr, StackOutput

# Common SSH options for all SSH commands
SSH_OPTIONS = {
    # Disable host key checking, the IPs of the QEMU machines are reused and we don't want constant
    # warnings about changed host keys. We need the combination of both options, if we just set
    # StrictHostKeyChecking to no, it will still check the known hosts file and disable some options
    # and print out scary warnings if the key doesn't match.
    "StrictHostKeyChecking": "accept-new",
    "UserKnownHostsFile": "/dev/null",
}


def ssh_options_command(extra_opts: Optional[Dict[str, str]] = None):
    opts = SSH_OPTIONS.copy()
    if extra_opts is not None:
        opts.update(extra_opts)

    return " ".join([f"-o {k}={v}" for k, v in opts.items()])


class LocalCommandRunner:
    @staticmethod
    def run_cmd(ctx: Context, _: 'HostInstance', cmd: str, allow_fail: bool, verbose: bool):
        res = ctx.run(cmd.format(proxy_cmd=""), hide=(not verbose), warn=allow_fail)
        if res is not None and res.ok:
            return True

        error(f"[-] Failed: {cmd}")
        if allow_fail:
            return False
        if res is not None:
            print_failed(res.stderr)
        raise Exit("command failed")

    @staticmethod
    def move_to_shared_directory(
        ctx: Context, _: 'HostInstance', source: PathOrStr, subdir: Optional[PathOrStr] = None
    ):
        recursive = ""
        if os.path.isdir(source):
            recursive = "-R"

        full_target = get_kmt_os().shared_dir
        if subdir is not None:
            full_target = os.path.join(get_kmt_os().shared_dir, subdir)
            ctx.run(f"mkdir -p {full_target}")
        ctx.run(f"cp {recursive} {source} {full_target}")


class RemoteCommandRunner:
    @staticmethod
    def run_cmd(ctx: Context, instance: 'HostInstance', cmd: str, allow_fail: bool, verbose: bool):
        res = ctx.run(
            cmd.format(
                proxy_cmd=f"-o ProxyCommand='ssh {ssh_options_command()} -i {instance.ssh_key} -W %h:%p ubuntu@{instance.ip}'"
            ),
            hide=(not verbose),
            warn=allow_fail,
        )
        if res is not None and res.ok:
            return True

        error(f"[-] Failed: {cmd}")
        if allow_fail:
            return False
        if res is not None:
            print_failed(res.stderr)
        raise Exit("command failed")

    @staticmethod
    def move_to_shared_directory(
        ctx: Context, instance: 'HostInstance', source: PathOrStr, subdir: Optional[PathOrStr] = None
    ):
        full_target = get_kmt_os().shared_dir
        if subdir is not None:
            full_target = os.path.join(get_kmt_os().shared_dir, subdir)
            RemoteCommandRunner.run_cmd(ctx, instance, f"mkdir -p {full_target}", False, False)

        ctx.run(
            f"rsync -e \"ssh {ssh_options_command()} -i {instance.ssh_key}\" -p -rt --exclude='.git*' --filter=':- .gitignore' {source} ubuntu@{instance.ip}:{full_target}"
        )


def get_instance_runner(arch: ArchOrLocal):
    if arch == "local":
        return LocalCommandRunner
    else:
        return RemoteCommandRunner


def print_failed(output: str):
    out = list()
    for line in output.split("\n"):
        out.append(f"\t{line}")
    error('\n'.join(out))


class LibvirtDomain:
    def __init__(
        self,
        ip: str,
        domain_id: str,
        tag: str,
        vmset_tags: List[str],
        ssh_key_path: Optional[str],
        instance: 'HostInstance',
    ):
        self.ip = ip
        self.name = domain_id
        self.tag = tag
        self.vmset_tags = vmset_tags
        self.ssh_key = ssh_key_path
        self.instance = instance

    def run_cmd(self, ctx: Context, cmd: str, allow_fail=False, verbose=False, timeout_sec=None):
        if timeout_sec is not None:
            extra_opts = {"ConnectTimeout": str(timeout_sec)}
        else:
            extra_opts = None

        run = f"ssh {ssh_options_command(extra_opts)} -i {self.ssh_key} root@{self.ip} {{proxy_cmd}} '{cmd}'"
        return self.instance.runner.run_cmd(ctx, self.instance, run, allow_fail, verbose)

    def copy(self, ctx: Context, source: PathOrStr, target: PathOrStr):
        run = f"rsync -e \"ssh {ssh_options_command()} {{proxy_cmd}} -i {self.ssh_key}\" -p -rt --exclude='.git*' --filter=':- .gitignore' {source} root@{self.ip}:{target}"
        return self.instance.runner.run_cmd(ctx, self.instance, run, False, False)

    def __repr__(self):
        return f"<LibvirtDomain> {self.name} {self.ip}"

    def check_reachable(self, ctx: Context) -> bool:
        return self.run_cmd(ctx, "true", allow_fail=True, timeout_sec=2)


class HostInstance:
    def __init__(self, ip: str, arch: ArchOrLocal, ssh_key: Optional[str]):
        self.ip: str = ip
        self.arch: ArchOrLocal = arch
        self.ssh_key: Optional[str] = ssh_key
        self.microvms: List[LibvirtDomain] = []
        self.runner = get_instance_runner(arch)

    def add_microvm(self, domain: LibvirtDomain):
        self.microvms.append(domain)

    def copy_to_all_vms(self, ctx: Context, path: PathOrStr, subdir: Optional[PathOrStr] = None):
        self.runner.move_to_shared_directory(ctx, self, path, subdir)

    def __repr__(self):
        return f"<HostInstance> {self.ip} {self.arch}"


def build_infrastructure(stack: str, remote_ssh_key: Optional[str] = None):
    stack_output = os.path.join(get_kmt_os().stacks_dir, stack, "stack.output")
    if not os.path.exists(stack_output):
        raise Exit(f"no stack.output file present at {stack_output}")

    with open(stack_output, 'r') as f:
        try:
            infra_map: StackOutput = json.load(f)
        except json.decoder.JSONDecodeError:
            raise Exit(f"{stack_output} file is not a valid json file")

    infra: Dict[ArchOrLocal, HostInstance] = dict()
    for arch in infra_map:
        if arch != "local" and remote_ssh_key is None:
            if ask_for_ssh():
                raise Exit("No ssh key provided. Pass with '--ssh-key=<key-name>'")

        key = None
        if remote_ssh_key is not None:
            key = ssh_key_to_path(remote_ssh_key)
        instance = HostInstance(infra_map[arch]["ip"], arch, key)
        for vm in infra_map[arch]["microvms"]:
            # We use the local ddvm_rsa key as the path to the key stored in the pulumi output JSON
            # file refers to the location in the remote instance, which might not be the same as the
            # location in the local machine.
            instance.add_microvm(
                LibvirtDomain(
                    vm["ip"], vm["id"], vm["tag"], vm["vmset-tags"], os.fspath(get_kmt_os().ddvm_rsa), instance
                )
            )

        infra[arch] = instance

    return infra


def ssh_key_to_path(ssh_key: str) -> str:
    ssh_key_path = ""
    if ssh_key != "":
        ssh_key.rstrip(".pem")
        ssh_key_path = find_ssh_key(ssh_key)

    return ssh_key_path


def ask_for_ssh() -> bool:
    return (
        ask(
            "You may want to provide ssh key, since the given config launches a remote instance.\nContinue without a ssh key?[Y/n]"
        )
        != "y"
    )


def find_ssh_key(ssh_key: str) -> str:
    possible_paths = [f"~/.ssh/{ssh_key}", f"~/.ssh/{ssh_key}.pem"]

    # Try direct files
    for path in possible_paths:
        if os.path.exists(os.path.expanduser(path)):
            return path

    # Ok, no file found with that name. However, maybe we can identify the key by the key name
    # that's present in the corresponding pub files

    for pubkey in glob.glob(os.path.expanduser("~/.ssh/*.pub")):
        privkey = pubkey[:-4]
        possible_paths.append(privkey)  # Keep track of paths we've checked

        with open(pubkey) as f:
            parts = f.read().split()

            # Public keys have three "words": key type, public key, name
            if len(parts) == 3 and parts[2] == ssh_key:
                return privkey

    raise Exit(f"Could not find file for ssh key {ssh_key}. Looked in {possible_paths}")
