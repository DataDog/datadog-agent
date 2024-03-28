from __future__ import annotations

import os
import platform
from pathlib import Path
from typing import TYPE_CHECKING, List, Optional, cast

from invoke.context import Context
from invoke.runners import Result

from tasks.kernel_matrix_testing.infra import ask_for_ssh, build_infrastructure, find_ssh_key
from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.libvirt import (
    delete_domains,
    delete_networks,
    delete_pools,
    delete_volumes,
    pause_domains,
    resource_in_stack,
    resume_domains,
)
from tasks.kernel_matrix_testing.tool import Exit, NoLibvirt, error, info
from tasks.kernel_matrix_testing.vars import VMCONFIG

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import PathOrStr

try:
    import libvirt
except ImportError:
    libvirt = None

X86_INSTANCE_TYPE = "m5d.metal"
ARM_INSTANCE_TYPE = "m6gd.metal"


def _get_active_branch_name() -> str:
    head_dir = Path(".") / ".git" / "HEAD"
    with head_dir.open() as f:
        content = f.read().splitlines()

    for line in content:
        if line.startswith("ref:"):
            return line.partition("refs/heads/")[2].replace("/", "-")

    raise Exit("Could not find active branch name")


def check_and_get_stack(stack: Optional[str]) -> str:
    if stack is None:
        stack = _get_active_branch_name()

    if not stack.endswith("-ddvm"):
        return f"{stack}-ddvm"
    else:
        return stack


def stack_exists(stack: str):
    return os.path.exists(f"{get_kmt_os().stacks_dir}/{stack}")


def vm_config_exists(stack: str):
    return os.path.exists(f"{get_kmt_os().stacks_dir}/{stack}/{VMCONFIG}")


def create_stack(ctx: Context, stack: Optional[str] = None):
    if not os.path.exists(f"{get_kmt_os().stacks_dir}"):
        raise Exit("Kernel matrix testing environment not correctly setup. Run 'inv kmt.init'.")

    stack = check_and_get_stack(stack)

    stack_dir = f"{get_kmt_os().stacks_dir}/{stack}"
    if os.path.exists(stack_dir):
        raise Exit(f"Stack {stack} already exists")

    ctx.run(f"mkdir {stack_dir}")


def remote_vms_in_config(vmconfig: PathOrStr):
    # Import here to avoid an import loop
    from tasks.kernel_matrix_testing.vmconfig import get_vmconfig

    data = get_vmconfig(vmconfig)

    for s in data["vmsets"]:
        if 'arch' in s and s["arch"] != "local":
            return True

    return False


def local_vms_in_config(vmconfig: PathOrStr):
    # Import here to avoid an import loop
    from tasks.kernel_matrix_testing.vmconfig import get_vmconfig

    data = get_vmconfig(vmconfig)

    for s in data["vmsets"]:
        if "arch" not in s:
            raise Exit("Invalid VMSet, arch field not found")

        if s["arch"] == "local":
            return True

    return False


def get_all_vms_in_stack(stack: PathOrStr):
    # Import here to avoid an import loop
    from tasks.kernel_matrix_testing.vmconfig import get_vmconfig

    data = get_vmconfig(f"{get_kmt_os().stacks_dir}/{stack}/{VMCONFIG}")
    vms: list[str] = list()

    for vmset in data["vmsets"]:
        for kernel in vmset.get("kernels", []):
            if 'recipe' not in vmset:
                raise Exit("Invalid VMSet, recipe field not found")
            vms.append(f"{vmset['recipe']}-{kernel['tag']}")

    return vms


def kvm_ok() -> None:
    if not os.path.exists("/dev/kvm"):
        error("[-] /dev/kvm not found. KVM not available on system")
        raise Exit("KVM not available")


def check_user_in_group(ctx: Context, group: str) -> bool:
    res = ctx.run(
        f"cat /proc/$$/status | grep '^Groups:' | grep $(cat /etc/group | grep '{group}:' | cut -d ':' -f 3)",
        warn=True,
    )
    if res is not None and res.ok:
        return True

    return False


def check_user_in_kvm(ctx: Context) -> None:
    if not check_user_in_group(ctx, "kvm"):
        error("You must add user '{os.getlogin()}' to group 'kvm'")
        raise Exit("User '{os.getlogin()}' not in group 'kvm'")

    info(f"[+] User '{os.getlogin()}' in group 'kvm'")


def check_user_in_libvirt(ctx: Context) -> None:
    if not check_user_in_group(ctx, "libvirt"):
        error("You must add user '{os.getlogin()}' to group 'libvirt'")
        raise Exit("User '{os.getlogin()}' not in group 'libvirt'")

    info(f"[+] User '{os.getlogin()}' in group 'libvirt'")


def check_libvirt_sock_perms() -> None:
    read_libvirt_sock()
    write_libvirt_sock()
    info(f"[+] User '{os.getlogin()}' has read/write permissions on libvirt sock")


def check_env(ctx: Context):
    info("[+] Checking environment for local machines")
    supported_local_envs = ["Linux", "Darwin"]

    if platform.system() not in supported_local_envs:
        raise Exit("Local machines only supported on Linux and MacOS")

    if platform.system() == "Linux":
        kvm_ok()
        # on macOS libvirt runs as the local user, so no need to check for group membership
        check_user_in_kvm(ctx)
        check_user_in_libvirt(ctx)

    check_libvirt_sock_perms()


def launch_stack(
    ctx: Context, stack: Optional[str], ssh_key: str, x86_ami: str, arm_ami: str, provision_microvms: bool
):
    stack = check_and_get_stack(stack)
    if not stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if not vm_config_exists(stack):
        raise Exit(f"No {VMCONFIG} for stack {stack}. Refer to 'inv kmt.gen-config --help'")

    stack_dir = f"{get_kmt_os().stacks_dir}/{stack}"
    vm_config = f"{stack_dir}/{VMCONFIG}"

    ssh_key.rstrip(".pem")
    if ssh_key != "":
        ssh_key_file = find_ssh_key(ssh_key)
        ssh_add_cmd = f"ssh-add -l | grep {ssh_key} || ssh-add {ssh_key_file}"
    elif remote_vms_in_config(vm_config):
        if ask_for_ssh():
            raise Exit("No ssh key provided. Pass with '--ssh-key=<key-name>'")
        ssh_add_cmd = ""
    else:
        ssh_add_cmd = ""

    ctx.run(ssh_add_cmd)

    env = [
        "TEAM=ebpf-platform",
        "PULUMI_CONFIG_PASSPHRASE=1234",
        f"LibvirtSSHKeyX86={stack_dir}/libvirt_rsa-x86_64",
        f"LibvirtSSHKeyARM={stack_dir}/libvirt_rsa-arm64",
        f"CI_PROJECT_DIR={stack_dir}",
    ]

    prefix = ""
    local = ""
    if remote_vms_in_config(vm_config):
        prefix = "aws-vault exec sso-sandbox-account-admin --"

    if local_vms_in_config(vm_config):
        check_env(ctx)
        local = "--local"

    provision = ""
    if remote_vms_in_config(vm_config):
        provision = "--provision-instance"

    args = [
        "--provision-microvms" if provision_microvms else "",
        f"--instance-type-x86={X86_INSTANCE_TYPE}",
        f"--instance-type-arm={ARM_INSTANCE_TYPE}",
        f"--x86-ami-id={x86_ami}",
        f"--arm-ami-id={arm_ami}",
        f"--ssh-key-name={ssh_key}",
        "--infra-env=aws/sandbox",
        f"--vmconfig={vm_config}",
        f"--stack-name={stack}",
        local,
        provision,
    ]
    ctx.run(f"{' '.join(env)} {prefix} inv -e system-probe.start-microvms {' '.join(args)}")

    info(f"[+] Stack {stack} successfully setup")


def destroy_stack_pulumi(ctx: Context, stack: str, ssh_key: str):
    if ssh_key != "":
        ssh_key_file = find_ssh_key(ssh_key)
        ssh_add_cmd = f"ssh-add -l | grep {ssh_key} || ssh-add {ssh_key_file}"
    else:
        ssh_add_cmd = ""

    ctx.run(ssh_add_cmd)

    stack_dir = f"{get_kmt_os().stacks_dir}/{stack}"
    env = [
        "PULUMI_CONFIG_PASSPHRASE=1234",
        f"LibvirtSSHKeyX86={stack_dir}/libvirt_rsa-x86_64",
        f"LibvirtSSHKeyARM={stack_dir}/libvirt_rsa-arm64",
        f"CI_PROJECT_DIR={stack_dir}",
    ]

    vm_config = f"{stack_dir}/{VMCONFIG}"
    prefix = ""
    if remote_vms_in_config(vm_config):
        prefix = "aws-vault exec sso-sandbox-account-admin --"

    env_vars = ' '.join(env)
    ctx.run(
        f"{env_vars} {prefix} inv system-probe.start-microvms --infra-env=aws/sandbox --stack-name={stack} --destroy --local"
    )


def ec2_instance_ids(ctx: Context, ip_list: List[str]) -> List[str]:
    ip_addresses = ','.join(ip_list)
    list_instances_cmd = f"aws-vault exec sso-sandbox-account-admin -- aws ec2 describe-instances --filter \"Name=private-ip-address,Values={ip_addresses}\" \"Name=tag:team,Values=ebpf-platform\" --query 'Reservations[].Instances[].InstanceId' --output text"

    res = ctx.run(list_instances_cmd, warn=True)
    if res is None or not res.ok:
        error("[-] Failed to get instance ids. Instances not destroyed. Used console to delete ec2 instances")
        return []

    return res.stdout.splitlines()


def destroy_ec2_instances(ctx: Context, stack: str):
    stack_output = os.path.join(get_kmt_os().stacks_dir, stack, "stack.output")
    if not os.path.exists(stack_output):
        return

    infra = build_infrastructure(stack, remote_ssh_key="")
    ips: List[str] = list()
    for arch, instance in infra.items():
        if arch != "local":
            ips.append(instance.ip)

    if len(ips) == 0:
        info("[+] No ec2 instance to terminate in stack")
        return

    instance_ids = ec2_instance_ids(ctx, ips)
    if len(instance_ids) == 0:
        return

    if len(instance_ids) > 2:
        error(f"CAREFUL! More than two instance ids returned. Something is wrong: {instance_ids}")
        raise Exit("Too many instance_ids")

    ids = ' '.join(instance_ids)
    res = ctx.run(
        f"aws-vault exec sso-sandbox-account-admin -- aws ec2 terminate-instances --instance-ids {ids}", warn=True
    )
    if res is None or not res.ok:
        error(f"[-] Failed to terminate instances {ids}. Use console to terminate instances")
    else:
        info(f"[+] Instances {ids} terminated.")

    return


def remove_pool_directory(ctx: Context, stack: str):
    pools_dir = os.path.join(get_kmt_os().libvirt_dir, "pools")
    for _, dirs, _ in os.walk(pools_dir):
        for d in dirs:
            if resource_in_stack(stack, d):
                rm_path = os.path.join(pools_dir, d)
                ctx.run(f"sudo rm -r '{rm_path}'", hide=True)
                info(f"[+] Removed libvirt pool directory {rm_path}")


def destroy_stack_force(ctx: Context, stack: str):
    stack_dir = os.path.join(get_kmt_os().stacks_dir, stack)
    vm_config = os.path.join(stack_dir, VMCONFIG)

    if local_vms_in_config(vm_config):
        if libvirt is None:
            raise NoLibvirt()

        conn = libvirt.open(get_kmt_os().libvirt_socket)
        if not conn:
            raise Exit("destroy_stack_force: Failed to open connection to qemu:///system")
        delete_domains(conn, stack)
        delete_volumes(conn, stack)
        delete_pools(conn, stack)
        remove_pool_directory(ctx, stack)
        delete_networks(conn, stack)
        conn.close()

    destroy_ec2_instances(ctx, stack)

    # Find a better solution for this
    pulumi_stack_name = cast(
        'Result',
        ctx.run(
            f"PULUMI_CONFIG_PASSPHRASE=1234 pulumi stack ls -a -C ../test-infra-definitions 2> /dev/null | grep {stack} | cut -d ' ' -f 1",
            warn=True,
            hide=True,
        ),
    ).stdout.strip()

    if pulumi_stack_name == "":
        return

    ctx.run(
        f"PULUMI_CONFIG_PASSPHRASE=1234 pulumi cancel -y -C ../test-infra-definitions -s {pulumi_stack_name}",
        warn=True,
        hide=True,
    )
    ctx.run(
        f"PULUMI_CONFIG_PASSPHRASE=1234 pulumi stack rm --force -y -C ../test-infra-definitions -s {pulumi_stack_name}",
        warn=True,
        hide=True,
    )


def destroy_stack(ctx: Context, stack: Optional[str], pulumi: bool, ssh_key: str):
    stack = check_and_get_stack(stack)
    if not stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    info(f"[*] Destroying stack {stack}")
    if pulumi:
        destroy_stack_pulumi(ctx, stack, ssh_key)
    else:
        destroy_stack_force(ctx, stack)

    ctx.run(f"rm -r {get_kmt_os().stacks_dir}/{stack}")


def pause_stack(stack: Optional[str] = None):
    stack = check_and_get_stack(stack)
    if not stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")
    if libvirt is None:
        raise NoLibvirt()
    conn = libvirt.open(get_kmt_os().libvirt_socket)
    pause_domains(conn, stack)
    conn.close()


def resume_stack(stack=None):
    stack = check_and_get_stack(stack)
    if not stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")
    if libvirt is None:
        raise NoLibvirt()
    conn = libvirt.open(get_kmt_os().libvirt_socket)
    resume_domains(conn, stack)
    conn.close()


def read_libvirt_sock():
    if libvirt is None:
        raise NoLibvirt()
    conn = libvirt.open(get_kmt_os().libvirt_socket)
    if not conn:
        raise Exit("read_libvirt_sock: Failed to open connection to qemu:///system")
    conn.listAllDomains()
    conn.close()


testPoolXML = """
<pool type="dir">
  <name>mypool</name>
  <uuid>8c79f996-cb2a-d24d-9822-ac7547ab2d01</uuid>
  <capacity unit="bytes">100</capacity>
  <allocation unit="bytes">100</allocation>
  <available unit="bytes">100</available>
  <source>
  </source>
  <target>
    <path>/tmp</path>
    <permissions>
      <mode>0755</mode>
      <owner>-1</owner>
      <group>-1</group>
    </permissions>
  </target>
</pool>"""


def write_libvirt_sock():
    if libvirt is None:
        raise NoLibvirt()
    conn = libvirt.open(get_kmt_os().libvirt_socket)
    if not conn:
        raise Exit("write_libvirt_sock: Failed to open connection to qemu:///system")
    pool = conn.storagePoolDefineXML(testPoolXML, 0)
    if not pool:
        raise Exit("write_libvirt_sock: Failed to create StoragePool object.")
    pool.undefine()
    conn.close()
