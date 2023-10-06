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


def create_stack(ctx, stack=None):
    if not os.path.exists(f"{KMT_STACKS_DIR}"):
        raise Exit("Kernel matrix testing environment not correctly setup. Run 'inv kmt.init'.")

    stack = check_and_get_stack(stack)

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


def kvm_ok(ctx):
    ctx.run("kvm-ok")
    info("[+] Kvm available on system")


def check_user_in_group(ctx, group):
    ctx.run(f"cat /proc/$$/status | grep '^Groups:' | grep $(cat /etc/group | grep '{group}:' | cut -d ':' -f 3)")
    info(f"[+] User '{os.getlogin()}' in group '{group}'")


def check_user_in_kvm(ctx):
    check_user_in_group(ctx, "kvm")


def check_user_in_libvirt(ctx):
    check_user_in_group(ctx, "libvirt")


def check_libvirt_sock_perms():
    read_libvirt_sock()
    write_libvirt_sock()
    info(f"[+] User '{os.getlogin()}' has read/write permissions on libvirt sock")


def check_env(ctx):
    kvm_ok(ctx)
    check_user_in_kvm(ctx)
    check_user_in_libvirt(ctx)
    check_libvirt_sock_perms()


def launch_stack(ctx, stack, ssh_key, x86_ami, arm_ami):
    stack = check_and_get_stack(stack)
    if not stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if not vm_config_exists(stack):
        raise Exit(f"No {VMCONFIG} for stack {stack}. Refer to 'inv kmt.gen-config --help'")

    stack_dir = f"{KMT_STACKS_DIR}/{stack}"
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
        prefix = "aws-vault exec sso-sandbox-account-admin --"

    env_vars = ' '.join(env)
    ctx.run(
        f"{env_vars} {prefix} inv system-probe.start-microvms --infra-env=aws/sandbox --stack-name={stack} --destroy --local"
    )


def is_ec2_ip_entry(entry):
    return entry.startswith("arm64-instance-ip") or entry.startswith("x86_64-instance-ip")


def ec2_instance_ids(ctx, ip_list):
    ip_addresses = ','.join(ip_list)
    list_instances_cmd = f"aws-vault exec sso-sandbox-account-admin -- aws ec2 describe-instances --filter \"Name=private-ip-address,Values={ip_addresses}\" \"Name=tag:team,Values=ebpf-platform\" --query 'Reservations[].Instances[].InstanceId' --output text"

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

    if len(ips) == 0:
        info("[+] No ec2 instance to terminate in stack")
        return

    instance_ids = ec2_instance_ids(ctx, ips)
    if len(instance_ids) == 0:
        return

    ids = ' '.join(instance_ids)
    res = ctx.run(
        f"aws-vault exec sso-sandbox-account-admin -- aws ec2 terminate-instances --instance-ids {ids}", warn=True
    )
    if not res.ok:
        error(f"[-] Failed to terminate instances {ids}. Use console to terminate instances")
    else:
        info(f"[+] Instances {ids} terminated.")

    return


def destroy_stack_force(ctx, stack):
    conn = libvirt.open("qemu:///system")
    if not conn:
        raise Exit("destroy_stack_force: Failed to open connection to qemu:///system")
    delete_domains(conn, stack)
    delete_volumes(conn, stack)
    delete_pools(conn, stack)
    delete_networks(conn, stack)
    conn.close()

    destroy_ec2_instances(ctx, stack)

    # Find a better solution for this
    pulumi_stack_name = ctx.run(
        f"PULUMI_CONFIG_PASSPHRASE=1234 pulumi stack ls -a -C ../test-infra-definitions 2> /dev/null | grep {stack} | cut -d ' ' -f 1",
        warn=True,
        hide=True,
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


def destroy_stack(ctx, stack, force, ssh_key):
    stack = check_and_get_stack(stack)
    if not stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    info(f"[*] Destroying stack {stack}")
    if force:
        destroy_stack_force(ctx, stack)
    else:
        destroy_stack_pulumi(ctx, stack, ssh_key)

    ctx.run(f"rm -r {KMT_STACKS_DIR}/{stack}")


def pause_stack(stack=None):
    stack = check_and_get_stack(stack)
    if not stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")
    conn = libvirt.open("qemu:///system")
    pause_domains(conn, stack)
    conn.close()


def resume_stack(stack=None):
    stack = check_and_get_stack(stack)
    if not stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")
    conn = libvirt.open("qemu:///system")
    resume_domains(conn, stack)
    conn.close()


def read_libvirt_sock():
    conn = libvirt.open("qemu:///system")
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
    conn = libvirt.open("qemu:///system")
    if not conn:
        raise Exit("write_libvirt_sock: Failed to open connection to qemu:///system")
    pool = conn.storagePoolDefineXML(testPoolXML, 0)
    if not pool:
        raise Exit("write_libvirt_sock: Failed to create StoragePool object.")
    pool.undefine()
    conn.close()
