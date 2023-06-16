from .init_kmt import KMT_DIR, KMT_STACKS_DIR, VMCONFIG, check_and_get_stack, get_active_branch_name
import os
from invoke.exceptions import Exit
import getpass
import libvirt

def stack_exists(stack):
    return os.path.exists(f"{KMT_STACKS_DIR}/{stack}")

def vm_config_exists(stack):
    return os.path.exists(f"{KMT_STACKS_DIR}/{stack}/{VMCONFIG}")


def create_stack(ctx, stack=None, branch=False):
    if not os.path.exists(f"{KMT_STACKS_DIR}"):
        raise Exit("Kernel matrix testing environment not correctly setup. Run 'inv kmt.init'.")

    stack = check_and_get_stack(stack, branch) 
    if branch:
        stack = get_active_branch_name()

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

def launch_stack(
    ctx, stack=None, branch=False, ssh_key="", x86_ami="ami-0584a00dd384af6ab", arm_ami="ami-0b7cd13521845570c"
):
    stack = check_and_get_stack(stack, branch)
    if not stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if not vm_config_exists(stack):
        raise Exit(f"No {VMCONFIG} for stack {stack}. Refer to 'inv kmt.gen-config --help'")

    if not os.path.exists("../test-infra-definitions"):
        raise Exit("'test-infra-definitions' repository required to launc VMs")

    vm_config = f"{KMT_STACKS_DIR}/{stack}/{VMCONFIG}"
    micro_vm_scenario = "../test-infra-definitions/scenarios/aws/microVMs"

    if ssh_key != "":
        ssh_key_file = find_ssh_key(ssh_key)
        ssh_add_cmd = f"ssh-add -l | grep {ssh_key} || ssh-add {ssh_key_file}"
    else:
        ssh_add_cmd = ""

    pulumi_cmd = [
        "PULUMI_CONFIG_PASSPHRASE=1234",
        "pulumi",
        "up",
        "-c scenario=aws/microvms",
        f"-c ddinfra:aws/defaultKeyPairName={ssh_key}",
        "-c ddinfra:env=aws/sandbox",
        "-c ddinfra:aws/defaultARMInstanceType=m6g.metal",
        "-c ddinfra:aws/defaultInstanceType=i3.metal",
        "-c ddinfra:aws/defaultInstanceStorageSize=500",
        f"-c microvm:microVMConfigFile={vm_config}",
        f"-c microvm:workingDir={KMT_DIR}",
        "-c microvm:provision=false",
        f"-c microvm:x86AmiID={x86_ami}",
        f"-c microvm:arm64AmiID={arm_ami}",
        f"-C {micro_vm_scenario}",
        "-y",
        f"-s {stack}",
    ]

    if not os.path.exists(micro_vm_scenario):
        raise Exit(f"Could not find scenario directory at {micro_vm_scenario}")

    print("Run this ->\n")
    print(ssh_add_cmd)
    print(' '.join(pulumi_cmd))

def resource_in_stack(stack, resource):
    return resource.startswith(stack)

def get_resources_in_stack(stack, list_fn):
    resources = list_fn()
    stack_resources = list()
    for resource in resources:
        if resource_in_stack(stack, resource.name()):
            stack_resources.append(resource)

    return stack_resources

def delete_domains(conn, stack):
    domains = get_resources_in_stack(stack, conn.listAllDomains)
    print(f"[*] {len(domains)} VMs running in stack {stack}")

    for domain in domains:
        name = domain.name()
        domain.destroy()
        domain.undefine()
        print(f"[+] VM {name} deleted")


def getAllStackVolumesFn(conn, stack):
    def getAllStackVolumes():
        pools = get_resources_in_stack(stack, conn.listAllStoragePools)

        volumes = list()
        for pool in pools:
            volumes += pool.listAllVolumes()

        return volumes

    return getAllStackVolumes


def delete_volumes(conn, stack):
    volumes = get_resources_in_stack(stack, getAllStackVolumesFn(conn, stack))
    print(f"[*] {len(volumes)} storage volumes running in stack {stack}")

    for volume in volumes:
        name = volume.name()
        volume.delete()
        #        volume.undefine()
        print(f"[+] Storage volume {name} deleted")


def delete_pools(conn, stack):
    pools = get_resources_in_stack(stack, conn.listAllStoragePools)
    print(f"[*] {len(pools)} storage pools running in stack {stack}")

    for pool in pools:
        name = pool.name()
        pool.destroy()
        pool.undefine()
        print(f"[+] Storage pool {name} deleted")


def delete_networks(conn, stack):
    networks = get_resources_in_stack(stack, conn.listAllNetworks)
    print(f"[*] {len(networks)} networks running in stack {stack}")

    for network in networks:
        name = network.name()
        network.destroy()
        network.undefine()
        print(f"[+] Network {name} deleted")


def destroy_stack_pulumi(ctx, stack):
    destroy_cmd = [
        "PULUMI_CONFIG_PASSPHRASE=1234",
        "pulumi",
        "destroy",
        f"-C ../test-infra-definitions/scenarios/aws/microVMs -s {stack}",
    ]

    print("Run this ->\n")
    print(' '.join(destroy_cmd))


def is_ec2_ip_entry(entry):
    return entry.startswith("arm64-instance-ip") or entry.startswith("x86_64-instance-ip")


def ec2_instance_ids(ctx, ip_list):
    ip_addresses = ','.join(ip_list)
    list_instances_cmd = "aws-vault exec sandbox-account-admin -- aws ec2 describe-instances --filter \"Name=private-ip-address,Values={private_ips}\" \"Name=tag:Team,Values=ebpf-platform\" --query 'Reservations[].Instances[].InstanceId' --output text".format(
        private_ips=ip_addresses
    )

    res = ctx.run(list_instances_cmd, warn=True)
    if not res.ok:
        print("[-] Failed to get instance ids. Instances not destroyed. Used console to delete ec2 instances")
        return

    return res.stdout.splitlines()


def destroy_ec2_instance(ctx, stack):
    stack_output = os.path.join(KMT_STACKS_DIR, stack, "stack.output")
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
        print(f"[-] Failed to terminate instances {ids}. Use console to terminate instances")
    else:
        print(f"[+] Instances {ids} terminated.")

    return


def destroy_stack_force(ctx, stack):
    conn = libvirt.open("qemu:///system")
    delete_domains(conn, stack)
    delete_volumes(conn, stack)
    delete_pools(conn, stack)
    delete_networks(conn, stack)
    conn.close()


def destroy_stack(ctx, stack=None, branch=False, force=False):
    stack = check_and_get_stack(stack, branch)
    if not stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    print(f"[*] Destroying stack {stack}")
    if force:
        destroy_stack_force(ctx, stack)
        ctx.run(
            f"PULUMI_CONFIG_PASSPHRASE=1234 pulumi stack rm --force -y -C ../test-infra-definitions/scenarios/aws/microVMs -s {stack}"
        )
    else:
        destroy_stack_pulumi(ctx, stack)

    ctx.run(f"rm -r {KMT_STACKS_DIR}/{stack}")
