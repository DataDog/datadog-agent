from __future__ import annotations

import glob
import json
import os
from pathlib import Path
from typing import TYPE_CHECKING, Dict, List, Optional

from invoke.context import Context

from tasks.kernel_matrix_testing.config import ConfigManager
from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.tool import Exit, ask, error, info

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import ArchOrLocal, PathOrStr, SSHKey, StackOutput

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
        ssh_key_arg = f"-i {instance.ssh_key_path}" if instance.ssh_key_path is not None else ""
        res = ctx.run(
            cmd.format(
                proxy_cmd=f"-o ProxyCommand='ssh {ssh_options_command()} {ssh_key_arg} -W %h:%p ubuntu@{instance.ip}'"
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

        ssh_key_arg = f"-i {instance.ssh_key_path}" if instance.ssh_key_path is not None else ""
        ctx.run(
            f"rsync -e \"ssh {ssh_options_command()} {ssh_key_arg}\" -p -rt --exclude='.git*' --filter=':- .gitignore' {source} ubuntu@{instance.ip}:{full_target}"
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
    def __init__(self, ip: str, arch: ArchOrLocal, ssh_key_path: Optional[str]):
        self.ip: str = ip
        self.arch: ArchOrLocal = arch
        self.ssh_key_path: Optional[str] = ssh_key_path
        self.microvms: List[LibvirtDomain] = []
        self.runner = get_instance_runner(arch)

    def add_microvm(self, domain: LibvirtDomain):
        self.microvms.append(domain)

    def copy_to_all_vms(self, ctx: Context, path: PathOrStr, subdir: Optional[PathOrStr] = None):
        self.runner.move_to_shared_directory(ctx, self, path, subdir)

    def __repr__(self):
        return f"<HostInstance> {self.ip} {self.arch}"


def build_infrastructure(stack: str, ssh_key_obj: Optional[SSHKey] = None):
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
        key = ssh_key_obj['path'] if ssh_key_obj is not None else None
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


def ask_for_ssh() -> bool:
    return (
        ask(
            "You may want to provide ssh key, since the given config launches a remote instance.\nContinue without a ssh key?[Y/n]"
        )
        != "y"
    )


def get_ssh_key_name(pubkey: Path) -> Optional[str]:
    parts = pubkey.read_text().split()
    if len(parts) != 3:
        return None
    return parts[2]


def get_ssh_agent_key_names(ctx: Context) -> List[str]:
    """Return the key names found in the SSH agent"""
    agent_output = ctx.run("ssh-add -l")
    if agent_output is None or not agent_output.ok:
        raise Exit("Cannot find any keys in the SSH agent")
    output_parts = [line.split() for line in agent_output.stdout.split("\n")]
    return [parts[2] for parts in output_parts if len(parts) >= 3]


def try_get_ssh_key(ctx: Context, key_hint: Optional[str]) -> Optional[SSHKey]:
    """Return a SSHKey object, either using the hint provided
    or using the configuration.

    The hint can either be a file path, a key name or a name of a file in ~/.ssh
    """
    if key_hint is not None:
        checked_paths: List[str] = []
        possible_paths = map(Path, [key_hint, f"~/.ssh/{key_hint}", f"~/.ssh/{key_hint}.pem"])
        for path in possible_paths:
            checked_paths.append(os.fspath(path))
            if not path.is_file():
                continue

            # Try to get the public key
            if path.suffix == '.pub':
                pubkey = path
                privkey = path.with_suffix("")
            else:
                # Try replacing and adding the .pub suffix
                possible_pubkeys = [path.with_suffix(".pub"), Path(f"{os.fspath(path)}.pub")]
                pubkey = next((p for p in possible_pubkeys if p.is_file()), None)
                privkey = path

            keyname = get_ssh_key_name(pubkey) if pubkey is not None else None
            if keyname is None:
                raise Exit(f"Cannot find a key name in {path}")
            return {'path': os.fspath(privkey), 'name': keyname, 'aws_key_name': keyname}

        # Key hint is not a file, see if it's a key name
        for pubkey in glob.glob(os.path.expanduser("~/.ssh/*.pub")):
            privkey = pubkey[:-4]
            checked_paths.append(privkey)
            key_name = get_ssh_key_name(Path(pubkey))
            if key_name == key_hint:
                return {'path': privkey, 'name': key_hint, 'aws_key_name': key_hint}

        # Check if it's a key name that's there in the agent
        agent_keys = get_ssh_agent_key_names(ctx)
        if key_hint in agent_keys:
            return {'path': None, 'name': key_hint, 'aws_key_name': key_hint}

        raise Exit(
            f"Could not find file for ssh key {key_hint}. Looked in {possible_paths}, it's not a path, not a file name nor a key name"
        )

    cm = ConfigManager()
    return cm.config.get("ssh")


def ensure_key_in_agent(ctx: Context, key: SSHKey):
    info(f"[+] Checking that key {key} is in the SSH agent...")
    res = ctx.run(f"ssh-add -l | grep {key['name']}")
    if res is None or not res.ok:
        if key['path'] is None:
            raise Exit(f"Key {key} not found in the agent and no path provided to add it")

        info(f"[+] Key {key} not present in the agent, adding it")
        res = ctx.run(f"ssh-add {key['path']}")
        if res is None or not res.ok:
            raise Exit(f"Could not add key {key} to the SSH agent")


def ensure_key_in_ec2(ctx: Context, key: SSHKey):
    info(f"[+] Checking that key {key} is in AWS...")
    res = ctx.run(
        f"aws-vault exec sso-sandbox-account-admin -- aws ec2 describe-key-pairs --key-names {key['aws_key_name']}"
    )
    if res is None or not res.ok:
        raise Exit(f"Couldn't retrieve {key} from AWS EC2")
