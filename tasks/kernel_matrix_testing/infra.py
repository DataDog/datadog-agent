import glob
import json
import os

from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.tool import Exit, ask, error


class LocalCommandRunner:
    @staticmethod
    def run_cmd(ctx, _, cmd, allow_fail, verbose):
        res = ctx.run(cmd.format(proxy_cmd=""), hide=(not verbose), warn=allow_fail)
        if not res.ok:
            error(f"[-] Failed: {cmd}")
            if allow_fail:
                return False
            print_failed(res.stderr)
            raise Exit("command failed")

        return True

    @staticmethod
    def move_to_shared_directory(ctx, _, source, subdir=None):
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
    def run_cmd(ctx, instance, cmd, allow_fail, verbose):
        res = ctx.run(
            cmd.format(
                proxy_cmd=f"-o ProxyCommand='ssh -o StrictHostKeyChecking=no -i {instance.ssh_key} -W %h:%p ubuntu@{instance.ip}'"
            ),
            hide=(not verbose),
            warn=allow_fail,
        )
        if not res.ok:
            error(f"[-] Failed: {cmd}")
            if allow_fail:
                return False
            print_failed(res.stderr)
            raise Exit("command failed")

        return True

    @staticmethod
    def move_to_shared_directory(ctx, instance, source, subdir=None):
        full_target = get_kmt_os().shared_dir
        if subdir is not None:
            full_target = os.path.join(get_kmt_os().shared_dir, subdir)
            RemoteCommandRunner.run_cmd(ctx, instance, f"mkdir -p {full_target}", False, False)

        ctx.run(
            f"rsync -e \"ssh -o StrictHostKeyChecking=no -i {instance.ssh_key}\" -p -rt --exclude='.git*' --filter=':- .gitignore' {source} ubuntu@{instance.ip}:{full_target}"
        )


def get_instance_runner(arch):
    if arch == "local":
        return LocalCommandRunner
    else:
        return RemoteCommandRunner


def print_failed(output):
    out = list()
    for line in output.split("\n"):
        out.append(f"\t{line}")
    error('\n'.join(out))


class LibvirtDomain:
    def __init__(self, ip, domain_id, tag, vmset_tags, ssh_key_path, instance):
        self.ip = ip
        self.name = domain_id
        self.tag = tag
        self.vmset_tags = vmset_tags
        self.ssh_key = ssh_key_path
        self.instance = instance

    def run_cmd(self, ctx, cmd, allow_fail=False, verbose=False):
        run = f"ssh -o StrictHostKeyChecking=no -i {self.ssh_key} root@{self.ip} {{proxy_cmd}} '{cmd}'"
        return self.instance.runner.run_cmd(ctx, self.instance, run, allow_fail, verbose)

    def copy(self, ctx, source, target):
        run = f"rsync -e \"ssh -o StrictHostKeyChecking=no {{proxy_cmd}} -i {self.ssh_key}\" -p -rt --exclude='.git*' --filter=':- .gitignore' {source} root@{self.ip}:{target}"
        return self.instance.runner.run_cmd(ctx, self.instance, run, False, False)

    def __repr__(self):
        return f"<LibvirtDomain> {self.name} {self.ip}"


class HostInstance:
    def __init__(self, ip, arch, ssh_key):
        self.ip = ip
        self.arch = arch
        self.ssh_key = ssh_key
        self.microvms = []
        self.runner = get_instance_runner(arch)

    def add_microvm(self, domain):
        self.microvms.append(domain)

    def copy_to_all_vms(self, ctx, path, subdir=None):
        self.runner.move_to_shared_directory(ctx, self, path, subdir)

    def __repr__(self):
        return f"<HostInstance> {self.ip} {self.arch}"


def build_infrastructure(stack, remote_ssh_key=None):
    stack_output = os.path.join(get_kmt_os().stacks_dir, stack, "stack.output")
    if not os.path.exists(stack_output):
        raise Exit("no stack.output file present")

    with open(stack_output, 'r') as f:
        infra_map = json.load(f)

    infra = dict()
    for arch in infra_map:
        if arch != "local" and remote_ssh_key is None:
            if ask_for_ssh():
                raise Exit("No ssh key provided. Pass with '--ssh-key=<key-name>'")

        key = None
        if remote_ssh_key is not None:
            key = ssh_key_to_path(remote_ssh_key)
        instance = HostInstance(infra_map[arch]["ip"], arch, key)
        for vm in infra_map[arch]["microvms"]:
            instance.add_microvm(
                LibvirtDomain(vm["ip"], vm["id"], vm["tag"], vm["vmset-tags"], vm["ssh-key-path"], instance)
            )

        infra[arch] = instance

    return infra


def ssh_key_to_path(ssh_key):
    ssh_key_path = ""
    if ssh_key != "":
        ssh_key.rstrip(".pem")
        ssh_key_path = find_ssh_key(ssh_key)

    return ssh_key_path


def ask_for_ssh():
    return (
        ask(
            "You may want to provide ssh key, since the given config launches a remote instance.\nContinue without a ssh key?[Y/n]"
        )
        != "y"
    )


def find_ssh_key(ssh_key):
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
