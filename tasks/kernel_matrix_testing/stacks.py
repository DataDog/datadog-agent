import getpass
import json
import os

from .init_kmt import KMT_STACKS_DIR, VMCONFIG, check_and_get_stack
from .libvirt import delete_domains, delete_networks, delete_pools, delete_volumes, pause_domains, resume_domains
from .tool import Exit, ask, error, info, warn

try:
    import libvirt
except ImportError:
    libvirt = None

X86_INSTANCE_TYPE = "m5.metal"
ARM_INSTANCE_TYPE = "m6g.metal"


def stack_exists(stack):
    return os.path.exists(f"{KMT_STACKS_DIR}/{stack}")


def vm_config_exists(stack):
    return os.path.exists(f"{KMT_STACKS_DIR}/{stack}/{VMCONFIG}")


def create_stack(ctx, stack=None, branch=False):
    if not os.path.exists(f"{KMT_STACKS_DIR}"):
        raise Exit("Kernel matrix testing environment not correctly setup. Run 'inv kmt.init'.")

    stack = check_and_get_stack(stack, branch)

    stack_dir = f"{KMT_STACKS_DIR}/{stack}"
    if os.path.exists(stack_dir):
        raise Exit(f"Stack {stack} already exists")

    ctx.run(f"mkdir {stack_dir}")


def find_ssh_key(ssh_key):
    user = getpass.getuser()
    ssh_key_file = f"/home/{user}/.ssh/{ssh_key}.pem"
    if not os.path.exists(ssh_key_file):
        raise Exit(f"Could not find file for ssh key {ssh_key}. Looked for {ssh_key_file}")

    return ssh_key_file


def remote_vms_in_config(vmconfig):
    with open(vmconfig, 'r') as f:
        data = json.load(f)

    for s in data["vmsets"]:
        if s["arch"] != "local":
            return True

    return False


def local_vms_in_config(vmconfig):
    with open(vmconfig, 'r') as f:
        data = json.load(f)

    for s in data["vmsets"]:
        if s["arch"] == "local":
            return True

    return False


def ask_for_ssh():
    return (
        ask(
            "You may want to provide ssh key, since the given config launches a remote instance.\nContinue witough ssh key?[Y/n]"
        )
        != "y"
    )


def launch_stack(ctx, stack, branch, ssh_key, x86_ami, arm_ami):
    stack = check_and_get_stack(stack, branch)
    if not stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if not vm_config_exists(stack):
        raise Exit(f"No {VMCONFIG} for stack {stack}. Refer to 'inv kmt.gen-config --help'")

    stack_dir = f"{KMT_STACKS_DIR}/{stack}"
    vm_config = f"{stack_dir}/{VMCONFIG}"

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
        prefix = "aws-vault exec sandbox-account-admin --"

    if local_vms_in_config(vm_config):
        local = "--local"

    env_vars = ' '.join(env)
    ctx.run(
        f"{env_vars} {prefix} inv -e system-probe.start-microvms --instance-type-x86={X86_INSTANCE_TYPE} --instance-type-arm={ARM_INSTANCE_TYPE} --x86-ami-id={x86_ami} --arm-ami-id={arm_ami} --ssh-key-name={ssh_key} --infra-env=aws/sandbox --vmconfig={vm_config} --stack-name={stack} {local}"
    )

    info(f"[+] Stack {stack} successfully setup")


def destroy_stack_pulumi(ctx, stack, ssh_key):
    if ssh_key != "":
        ssh_key_file = find_ssh_key(ssh_key)
        ssh_add_cmd = f"ssh-add -l | grep {ssh_key} || ssh-add {ssh_key_file}"
    else:
        ssh_add_cmd = ""

    ctx.run(ssh_add_cmd)

    stack_dir = f"{KMT_STACKS_DIR}/{stack}"
    env = [
        "PULUMI_CONFIG_PASSPHRASE=1234",
        f"LibvirtSSHKeyX86={stack_dir}/libvirt_rsa-x86_64",
        f"LibvirtSSHKeyARM={stack_dir}/libvirt_rsa-arm64",
        f"CI_PROJECT_DIR={stack_dir}",
    ]

    vm_config = f"{stack_dir}/{VMCONFIG}"
    prefix = ""
    if remote_vms_in_config(vm_config):
        prefix = "aws-vault exec sandbox-account-admin --"

    env_vars = ' '.join(env)
    ctx.run(
        f"{env_vars} {prefix} inv system-probe.start-microvms --infra-env=aws/sandbox --stack-name={stack} --destroy --local"
    )


def is_ec2_ip_entry(entry):
    return entry.startswith("arm64-instance-ip") or entry.startswith("x86_64-instance-ip")


def ec2_instance_ids(ctx, ip_list):
    ip_addresses = ','.join(ip_list)
    list_instances_cmd = f"aws-vault exec sandbox-account-admin -- aws ec2 describe-instances --filter \"Name=private-ip-address,Values={ip_addresses}\" \"Name=tag:team,Values=ebpf-platform\" --query 'Reservations[].Instances[].InstanceId' --output text"

    res = ctx.run(list_instances_cmd, warn=True)
    if not res.ok:
        error("[-] Failed to get instance ids. Instances not destroyed. Used console to delete ec2 instances")
        return

    return res.stdout.splitlines()


def destroy_ec2_instances(ctx, stack):
    stack_output = os.path.join(KMT_STACKS_DIR, stack, "stack.outputs")
    if not os.path.exists(stack_output):
        warn(f"[-] File {stack_output} not found")
        return

    with open(stack_output, 'r') as f:
        output = f.read().split('\n')

    ips = list()
    for o in output:
        if not is_ec2_ip_entry(o):
            continue

        ips.append(o.split(' ')[1])

    instance_ids = ec2_instance_ids(ctx, ips)
    if len(instance_ids) == 0:
        return

    ids = ' '.join(instance_ids)
    res = ctx.run(
        f"aws-vault exec sandbox-account-admin -- aws ec2 terminate-instances --instance-ids {ids}", warn=True
    )
    if not res.ok:
        error(f"[-] Failed to terminate instances {ids}. Use console to terminate instances")
    else:
        info(f"[+] Instances {ids} terminated.")

    return


def destroy_stack_force(ctx, stack):
    conn = libvirt.open("qemu:///system")
    delete_domains(conn, stack)
    delete_volumes(conn, stack)
    delete_pools(conn, stack)
    delete_networks(conn, stack)
    conn.close()

    destroy_ec2_instances(ctx, stack)

    # Find a better solution for this
    pulumi_stack_name = ctx.run(
        f"PULUMI_CONFIG_PASSPHRASE=1234 pulumi stack ls -C ../test-infra-definitions 2> /dev/null | grep {stack} | cut -d ' ' -f 1",
        warn=True,
        hide=True,
    ).stdout.strip()

    ctx.run(
        f"PULUMI_CONFIG_PASSPHRASE=1234 pulumi cancel -y -C ../test-infra-definitions -s {pulumi_stack_name}",
        warn=True,
    )
    ctx.run(
        f"PULUMI_CONFIG_PASSPHRASE=1234 pulumi stack rm --force -y -C ../test-infra-definitions -s {pulumi_stack_name}",
        warn=True,
    )


def destroy_stack(ctx, stack, branch, force, ssh_key):
    stack = check_and_get_stack(stack, branch)
    if not stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    info(f"[*] Destroying stack {stack}")
    if force:
        destroy_stack_force(ctx, stack)
    else:
        destroy_stack_pulumi(ctx, stack, ssh_key)

    ctx.run(f"rm -r {KMT_STACKS_DIR}/{stack}")


def pause_stack(stack=None, branch=False):
    stack = check_and_get_stack(stack, branch)
    if not stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")
    conn = libvirt.open("qemu:///system")
    pause_domains(conn, stack)
    conn.close()


def resume_stack(stack=None, branch=False):
    stack = check_and_get_stack(stack, branch)
    if not stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")
    conn = libvirt.open("qemu:///system")
    resume_domains(conn, stack)
    conn.close()
