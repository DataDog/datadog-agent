import os
import libvirt
from invoke import task
from invoke.exceptions import Exit
from glob import glob

KMT_DIR = os.path.join("home", "kernel-version-testing")
KMT_STACKS_DIR = os.path.join(KMT_DIR, "stacks")

lte414_extraparams = {"systemd.unified_cgroup_hierarchy": "0"}

config_skeleton = {"vmsets":[]}

vmset_templates = {
    "custom_kernels": {
        "name": "custom_kernels_{arch}",
        "recipe": "custom-{arch}",
        "arch": "{arch}",
        "kernels": [
            {"dir": "kernel-v{kernel_version}.x{karch}.pkg", "tag": "{tag}", "extra_params": {"console": "{console}"}}
        ],
        "vcpu": [],
        "memory": [],
        "image": {"image_path": "bullseye.qcow2.amd64-0.1-DEV", "image_uri": "{image_url}"},
    },
    "distro": {
        "name": "{distro}_{arch}",
        "recipe": "distro-{arch}",
        "arch": "{arch}",
        "kernels": [
            {"dir": "{dir}", "tag": "{tag}", "image_source": "{image_url}"},
        ],
        "vcpu": [],
        "memory": [],
    },
}

@task
def init(ctx):
    ctx.run(f"mkdir -p {KMT_STACKS_DIR}")
    # download dependencies


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


def delete_volumes(conn, stack):
    volumes = get_resources_in_stack(stack, conn.listAllVolumes)
    print(f"[*] {len(volumes)} storage volumes running in stack {stack}")

    for volume in volumes:
        name = volume.name()
        volume.destroy()
        volume.undefine()
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


@task
def destroy_stack(ctx, stack=None):
    if stack is None:
        raise Exit("Stack name is required")

    if not os.path.exists("f{KMT_STACKS_DIR}/{stack}"):
        raise Exit(f"stack {stack} not created")

    print(f"[*] Destroying stack {stack}")
    # ctx.run(f"pulumi login {KMT_DIR}/stacks/{stack}/.pulumi")
    conn = libvirt.open("qemu:///system")
    delete_domains(conn, stack)
    delete_volumes(conn, stack)
    delete_pools(conn, stack)
    delete_networks(conn, stack)
    conn.close()

    ctx.run("rm -r {KMT_STACKS_DIR}/{stack}")


@task
def create_stack(ctx, stack=None, update=False):
    if stack is None:
        raise Exit("Stack name is required")

    if not os.path.exists(f"{KMT_STACKS_DIR}"):
        raise Exit("Kernel matrix testing environment not correctly setup. Run 'inv kmt.init'.")

    ctx.run(f"mkdir {KMT_STACKS_DIR}/{stack}")

    if update:
        update_resources(ctx)

def empty_config(file_path):
    j = json.dumps(config_skeleton, indent=4)
    with open(file_path, 'w') as f:
        f.write(j)

@task
def gen_config(ctx, vms, stack=None, new=False):
    if stack is None:
        raise Exit("Stack name is required")

    if not os.path.exists(f"{KMT_STACKS_DIR}/{stack}"):
        raise Exit(f"Stack {stack} does not exist. Please create stack first 'inv kmt.stack-create --stack={stack}'")

    vm_types = vms.split(',')
    if len(vm_types) == 0:
        raise Exit("No VMs to boot provided")
 
    if new or not os.path.exists(f"{KMT_STACKS_DIR}/{stack}/vm-config.json"):
        ctx.run("rm -f {KMT_STACKS_DIR}/{stack}/vm-config.json")
        empty_config(f"{KMT_STACKS_DIR}/{stack}/vm-config.json")

    config_json = json.load(open(f"{KMT_STACKS_DIR}/{stack}/vm-config.json"))
    



@task
def update_resources(ctx):
    print("Updating resource dependencies will delete all running stacks.")
    if input("are you sure you want to continue? (y/n)") != "y":
        print("Update aborted")
        return

    for stack in glob(f"{KMT_STACKS_DIR}/*"):
        destroy_stack(ctx, stack=stack)
