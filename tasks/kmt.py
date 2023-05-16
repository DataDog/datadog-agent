import os
import itertools
import filecmp
import math
import json
import libvirt
import platform
from invoke import task
from invoke.exceptions import Exit
from glob import glob
from thefuzz import process
from thefuzz import fuzz

KMT_DIR = os.path.join("/", "home", "kernel-version-testing")
KMT_ROOTFS_DIR = os.path.join(KMT_DIR, "rootfs")
KMT_STACKS_DIR = os.path.join(KMT_DIR, "stacks")
KMT_PACKAGES_DIR = os.path.join(KMT_DIR, "kernel-packages")
KMT_BACKUP_DIR = os.path.join(KMT_DIR, "backups")
KMT_LIBVIRT_DIR = os.path.join(KMT_DIR, "libvirt")

VMCONFIG = "vm-config.json"

karch_mapping = {"x86_64": "x86", "arm64": "arm64"}
consoles = {"x86_64": "ttyS0", "arm64": "ttyAMA0"}
archs_mapping = {
    "amd64": "x86_64",
    "x86": "x86_64",
    "x86_64": "x86_64",
    "arm64": "arm64",
    "aarch64": "arm64",
    "arm": "arm64",
    "local": "local",
}
kernels = [
    "5.0",
    "5.1",
    "5.2",
    "5.3",
    "5.4",
    "5.5",
    "5.6",
    "5.7",
    "5.8",
    "5.9",
    "5.10",
    "5.11",
    "5.12",
    "5.13",
    "5.14",
    "5.15",
    "5.16",
    "5.17",
    "5.18",
    "5.19",
    "4.4",
    "4.5",
    "4.6",
    "4.7",
    "4.8",
    "4.9",
    "4.10",
    "4.11",
    "4.12",
    "4.13",
    "4.14",
    "4.15",
    "4.16",
    "4.17",
    "4.18",
    "4.19",
    "4.20",
]
distributions = {
    "ubuntu_18": "bionic",
    "ubuntu_20": "focal",
    "ubuntu_22": "jammy",
    "jammy": "jammy",
    "focal": "focal",
    "bionic": "bionic",
}
distribution_version_mapping = {"jammy": "ubuntu", "focal": "ubuntu", "bionic": "ubuntu"}
distro_arch_mapping = {"x86_64": "amd64", "arm64": "arm64"}
images_path = {
    "bionic": "file:///home/kernel-version-testing/rootfs/bionic-server-cloudimg-amd64.qcow2",
    "focal": "file:///home/kernel-version-testing/rootfs/focal-server-cloudimg-amd64.qcow2",
    "jammy": "file:///home/kernel-version-testing/rootfs/jammy-server-cloudimg-amd64.qcow2",
    "bullseye": "file:///home/kernel-version-testing/rootfs/bullseye.qcow2.arm64-DEV",
    "buster": "file:///home/kernel-version-testing/rootfs/buster.qcow2.amd64-DEV",
}


def is_root():
    return os.getuid() == 0


def stack_exists(stack):
    return os.path.exists(f"{KMT_STACKS_DIR}/{stack}")


def vm_config_exists(stack):
    return os.path.exists(f"{KMT_STACKS_DIR}/{stack}/{VMCONFIG}")


@task
def init(ctx):
    sudo = "sudo" if not is_root() else ""
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd 1000 | cut -d ':' -f 1) {KMT_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd 1000 | cut -d ':' -f 1) {KMT_PACKAGES_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd 1000 | cut -d ':' -f 1) {KMT_BACKUP_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd 1000 | cut -d ':' -f 1) {KMT_STACKS_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd 1000 | cut -d ':' -f 1) {KMT_LIBVIRT_DIR}")
    ctx.run(f"{sudo} install -d -m 0755 -g libvirt -o $(getent passwd 1000 | cut -d ':' -f 1) {KMT_ROOTFS_DIR}")
    # download dependencies

    download_kernel_packages(ctx)
    download_rootfs(ctx)


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
def create_stack(ctx, stack):
    if not os.path.exists(f"{KMT_STACKS_DIR}"):
        raise Exit("Kernel matrix testing environment not correctly setup. Run 'inv kmt.init'.")

    stack_dir = f"{KMT_STACKS_DIR}/{stack}"
    if os.path.exists(stack_dir):
        raise Exit(f"Stack {stack} already exists")

    ctx.run(f"mkdir {stack_dir}")


def empty_config(file_path):
    j = json.dumps({"vmsets": []}, indent=4)
    with open(file_path, 'w') as f:
        f.write(j)


def list_possible():
    distros = list(distributions.keys())
    archs = list(archs_mapping.keys())

    result = list()
    possible = list(itertools.product(["custom"], kernels, archs)) + list(itertools.product(["distro"], distros, archs))
    for p in possible:
        result.append(f"{p[0]}-{p[1]}-{p[2]}")

    return result


# This function derives the configuration for each
# unique kernel or distribution from the normalized vm-def.
# For more details on the generated configuration element, refer
# to the micro-vms scenario in test-infra-definitions
def get_kernel_config(recipe, version, arch):
    if recipe == "custom":
        return get_custom_kernel_config(version, arch)
    elif recipe == "distro":
        return get_distro_image_config(version, arch)

    raise Exit(f"Invalid recipe {recipe}")


def lte_414(version):
    major, minor = version.split('.')
    return (int(major) <= 4) and (int(minor) <= 14)


def get_custom_kernel_config(version, arch):
    if arch == "local":
        arch = archs_mapping[platform.machine()]

    kernel = {
        "dir": f"kernel-v{version}.{karch_mapping[arch]}.pkg",
        "tag": version,
        "extra_params": {"console": consoles[arch]},
    }

    if lte_414(version):
        kernel["extra_params"]["systemd.unified_cgroup_hierarchy"] = "0"

    return kernel


def get_distro_image_config(version, arch):
    if arch == "local":
        arch = archs_mapping[platform.machine()]

    return {
        "dir": f"{distributions[version]}-server-cloudimg-{distro_arch_mapping[arch]}.qcow2",
        "tag": version,
        "image_source": images_path[version],
    }


# normalize_vm_def converts the detected user provider vm-def
# to a standard form with consisten values for
# recipe: [custom, distro]
# version: [4.4, 4.5, ..., 5.15, jammy, focal, bionic]
# arch: [x86_64, amd64]
# Each normalized_vm_def output corresponds to each VM
# requested by the user
def normalize_vm_def(vm_def):
    recipe, version, arch = vm_def.split('-')

    arch = archs_mapping[arch]
    if recipe == "distro":
        version = distributions[version]

    return recipe, version, arch


def vmset_name_from_id(set_id):
    recipe, arch, id_tag = set_id

    return f"{recipe}_{id_tag}_{arch}"


# This function generates new VMSets. Refer to the documentation
# of the micro-vm scenario in test-infra-definitions to see what
# a VMSet is.
def build_new_vmset(set_id, kernels):
    recipe, arch, version = set_id
    vmset = dict()

    if arch == "local":
        platform_arch = archs_mapping[platform.machine()]
    else:
        platform_arch = arch

    if recipe == "custom":
        vmset = {"name": vmset_name_from_id(set_id), "recipe": f"custom-{arch}", "arch": arch, "kernels": kernels}
        if version == "lte_414":
            vmset["image"] = {
                "image_path": f"buster.qcow2.{distro_arch_mapping[platform_arch]}-DEV",
                "image_uri": images_path["buster"],
            }
        else:
            vmset["image"] = {
                "image_path": f"bullseye.qcow2.{distro_arch_mapping[platform_arch]}-DEV",
                "image_uri": images_path["bullseye"],
            }
    elif recipe == "distro":
        vmset = {"name": vmset_name_from_id(set_id), "recipe": f"distro-{arch}", "arch": arch, "kernels": kernels}
    else:
        raise Exit(f"Invalid recipe {recipe}")

    if arch == "arm64":
        vmset["machine"] = "virt"

    return vmset


# Set id uniquely categorizes each requested
# VM into particular sets.
# Each set id will contain 1 or more of the VMs requested
# by the user.
def vmset_id(recipe, version, arch):
    print(f"[+] recipe {recipe}, version {version}, arch {arch}")
    if recipe == "custom":
        if lte_414(version):
            return (recipe, arch, "lte_414")
        else:
            return (recipe, arch, "gt_414")
    else:
        return recipe, arch, distribution_version_mapping[version]


def vmset_exists(vm_config, set_name):
    vmsets = vm_config["vmsets"]

    for vmset in vmsets:
        if vmset["name"] == set_name:
            return True

    return False


def kernel_in_vmset(vmset, kernel):
    vmset_kernels = vmset["kernels"]
    for k in vmset_kernels:
        if k["tag"] == kernel["tag"]:
            return True

    return False


def add_kernels_to_vmset(vmset, set_name, kernels):
    for k in kernels:
        if kernel_in_vmset(vmset, k):
            continue
        if vmset["name"] == set_name:
            vmset["kernels"].append(k)


# Each vmset is uniquely identified by its name, which
# can be derived from the set_id. If a vmset exists,
# and we have data to add, this function modifies the appropriate
# vmset.
def modify_existing_vmsets(vm_config, set_id, kernels):
    set_name = vmset_name_from_id(set_id)

    if not vmset_exists(vm_config, set_name):
        return False

    vmsets = vm_config["vmsets"]
    for vmset in vmsets:
        add_kernels_to_vmset(vmset, set_name, kernels)

    return True


def generate_vm_config(vm_config, vms, vcpu, memory):
    # get all possible (recipe, version, arch) combinations we can support.
    possible = list_possible()

    kernels = dict()
    for vm in vms:
        # attempt to fuzzy match user provided vm-def with the possible list.
        vm_def, _ = process.extractOne(vm, possible, scorer=fuzz.token_sort_ratio)
        normalized_vm_def = normalize_vm_def(vm_def)
        set_id = vmset_id(*normalized_vm_def)
        # generate kernel configuration for each vm-def
        if set_id not in kernels:
            kernels[set_id] = [get_kernel_config(*normalized_vm_def)]
        else:
            kernels[set_id].append(get_kernel_config(*normalized_vm_def))

    keys_to_remove = list()
    # detect if the requested VM falls in an already existing vmset
    for set_id in kernels.keys():
        if modify_existing_vmsets(vm_config, set_id, kernels[set_id]):
            keys_to_remove.append(set_id)

    # delete kernels already added
    for key in keys_to_remove:
        del kernels[key]

    # this loop generates vmsets which do not already exist
    for set_id in kernels.keys():
        vm_config["vmsets"].append(build_new_vmset(set_id, kernels[set_id]))

    # Modify the vcpu and memory configuration of all sets.
    for vmset in vm_config["vmsets"]:
        vmset["vcpu"] = vcpu
        vmset["memory"] = memory


def check_memory_and_vcpus(memory, vcpus):
    for mem in memory:
        if not mem.isnumeric() or int(mem) == 0:
            raise Exit(f"Invalid values for memory provided {memory}")

    for v in vcpus:
        if not v.isnumeric or int(v) == 0:
            raise Exit(f"Invalid values for vcpu provided {vcpu}")


def power_log_str(x):
    num = int(x)
    return str(2 ** (math.ceil(math.log(num, 2))))


def mem_to_pow_of_2(memory):
    for i in range(len(memory)):
        new = power_log_str(memory[i])
        if new != memory[i]:
            print(f"rounding up memory: {memory[i]} -> {new}")
            memory[i] = new

def ls_to_int(ls):
    int_ls = list()
    for elem in ls:
        int_ls.append(int(elem))

    return int_ls

@task(
    help={
        "vms": "Comma seperated List of VMs to setup. Each definition must contain the following elemets (recipe, architecture, version).",
        "stack": "Name of the stack within which to generate the configuration file",
        "vcpu": "Comma seperated list of CPUs, to launch each VM with",
        "memory": "Comma seperated list of memory to launch each VM with. Automatically rounded up to power of 2",
        "new": "Generate new configuration file instead of appending to existing one within the provided stack",
    }
)
def gen_config(ctx, stack, vms="", init_stack=False, vcpu="4", memory="8192", new=False):
    if not stack_exists(stack) and not init_stack:
        raise Exit(
            f"Stack {stack} does not exist. Please create stack first 'inv kmt.stack-create --stack={stack}, or specify --create-stack option'"
        )

    if init_stack:
        create_stack(ctx, stack)

    print(f"[+] Select stack {stack}")

    vm_types = vms.split(',')
    if len(vm_types) == 0:
        raise Exit("No VMs to boot provided")

    vcpu_ls = vcpu.split(',')
    memory_ls = memory.split(',')

    check_memory_and_vcpus(memory_ls, vcpu_ls)
    mem_to_pow_of_2(memory_ls)

    vmconfig_file = f"{KMT_STACKS_DIR}/{stack}/{VMCONFIG}"
    # vmconfig_file = "/tmp/vm-config.json"
    if new or not os.path.exists(vmconfig_file):
        ctx.run("rm -f {vmconfig_file}")
        empty_config(vmconfig_file)

    with open(vmconfig_file) as f:
        orig_vm_config = f.read()

    vm_config = json.loads(orig_vm_config)
    generate_vm_config(vm_config, vm_types, ls_to_int(vcpu_ls), ls_to_int(memory_ls))
    vm_config_str = json.dumps(vm_config, indent=4)

    tmpfile = "/tmp/vm.json"
    with open(tmpfile, "w") as f:
        f.write(vm_config_str)

    ctx.run(f"git diff {vmconfig_file} {tmpfile}", warn=True)

    if input("are you sure you want to apply the diff? (y/n)") != "y":
        print("[-] diff not applied")
        return

    with open(vmconfig_file, "w") as f:
        f.write(vm_config_str)

    print(f"[+] vmconfig @ {vmconfig_file}")


def revert_kernel_packages(ctx):
    arch = archs_mapping[platform.machine()]
    kernel_packages_sum = f"kernel-packages-{arch}.sum"
    kernel_packages_tar = f"kernel-packages-{arch}.tar"
    ctx.run(f"rm -f {KMT_PACKAGES_DIR}/*")
    ctx.run(f"mv {KMT_BACKUP_DIR}/{kernel_packages_sum} {KMT_PACKAGES_DIR}")
    ctx.run(f"mv {KMT_BACKUP_DIR}/{kernel_packages_tar} {KMT_PACKAGES_DIR}")
    ctx.run(f"tar xvf {KMT_PACKAGES_DIR}/{kernel_packages_tar} | xargs -i tar xzf {{}}")


def download_kernel_packages(ctx, revert=False):
    arch = archs_mapping[platform.machine()]
    kernel_packages_sum = f"kernel-packages-{arch}.sum"
    kernel_packages_tar = f"kernel-packages-{arch}.tar"

    # download kernel packages
    res = ctx.run(
        f"wget -q https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/{kernel_packages_tar} -O {KMT_PACKAGES_DIR}/{kernel_packages_tar}",
        warn=True,
    )
    if not res.ok:
        if revert:
            revert_kernel_packages(ctx)
        raise Exit("Failed to download kernel pacakges")

    res = ctx.run(
        f"wget -q https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/{kernel_packages_sum} -O {KMT_PACKAGES_DIR}/{kernel_packages_sum}",
        warn=True,
    )
    if not res.ok:
        if revert:
            revert_kernel_packages(ctx)
        raise Exit("Failed to download kernel pacakges checksum")

    # extract pacakges
    res = ctx.run(f"cd {KMT_PACKAGES_DIR} && tar xvf {kernel_packages_tar} | xargs -i tar xzf {{}}")
    if not res.ok:
        if revert:
            revert_kernel_packages(ctx)
        raise Exit("Failed to extract kernel packages")

    # set permissions
    res = ctx.run(f"find {KMT_PACKAGES_DIR} -name kernel-v* -type d -exec chmod 0766 {{}}/* \;")
    if not res.ok:
        if revert:
            revert_kernel_packages(ctx)
        raise Exit("Failed to set permissions to kernel packages")


def update_kernel_packages(ctx):
    arch = archs_mapping[platform.machine()]
    kernel_packages_sum = f"kernel-packages-{arch}.sum"
    kernel_packages_tar = f"kernel-packages-{arch}.tar"

    ctx.run(
        f"wget -q https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/{kernel_packages_sum} -O /tmp/{kernel_packages_sum}"
    )

    current_sum_file = f"{KMT_PACKAGES_DIR}/{kernel_packages_sum}"
    if filecmp.cmp(current_sum_file, f"/tmp/{kernel_packages_sum}"):
        print("[-] No update required for custom kernel packages")

    # backup kernel-packges
    karch = karch_mapping[archs_mapping[platform.machine()]]
    ctx.run(
        f"find {KMT_PACKAGES_DIR} -name \"kernel-*.{karch}.pkg.tar.gz\" -type f | rev | cut -d '/' -f 1  | rev > /tmp/package.ls"
    )
    ctx.run(f"cd {KMT_PACKAGES_DIR} && tar -cf {kernel_packages_tar} -T /tmp/package.ls")
    ctx.run(f"cp {KMT_PACKAGES_DIR}/{kernel_packages_tar} {KMT_BACKUP_DIR}")
    ctx.run(f"cp {current_sum_file} {KMT_BACKUP_DIR}")
    print("[+] Backed up current packages")

    # clean kernel packages directory
    ctx.run(f"rm -f {KMT_PACKAGES_DIR}/*")

    download_kernel_packages(ctx, revert=True)

    print("[+] Kernel packages successfully updated")


def revert_rootfs(ctx, rootfs):
    ctx.run(f"rm -f {KMT_ROOTFS_DIR}/*")
    ctx.run(f"mv {KMT_ROOTFS_DIR}/{rootfs}.sum {KMT_ROOTFS_DIR}")
    ctx.run(f"mv {KMT_ROOTFS_DIR}/{rootfs}.tar.gz {KMT_ROOTFS_DIR}")


def download_rootfs(ctx, revert=False):
    arch = archs_mapping[platform.machine()]
    if arch == "x86_64":
        rootfs = "rootfs-amd64"
    elif arch == "arm64":
        rootfs = "rootfs-arm64"
    else:
        Exit(f"Unsupported arch detected {arch}")

    # download rootfs
    res = ctx.run(
        f"wget -q https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/{rootfs}.sum -O {KMT_ROOTFS_DIR}/{rootfs}.sum"
    )
    if not res.ok:
        if revert:
            revert_rootfs(ctx)
        raise Exit("Failed to download rootfs check sum file")

    res = ctx.run(
        f"wget -q https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/{rootfs}.tar.gz -O {KMT_ROOTFS_DIR}/{rootfs}.tar.gz"
    )
    if not res.ok:
        if revert:
            revert_rootfs(ctx)
        raise Exit("Failed to download rootfs")

    # extract rootfs
    res = ctx.run(f"cd {KMT_ROOTFS_DIR} && tar xzvf {rootfs}.tar.gz")
    if not res.ok:
        if revert:
            revert_rootfs(ctx)
        raise Exit("Failed to extract rootfs")

    # set permissions
    res = ctx.run(f"find {KMT_ROOTFS_DIR} -name *qcow* -type f -exec chmod 0766 {{}} \;")
    if not res.ok:
        if revert:
            revert_rootfs(ctx)
        raise Exit("Failed to set permissions 0766 to rootfs")



def update_rootfs(ctx):
    arch = archs_mapping[platform.machine()]
    if arch == "x86_64":
        rootfs = "rootfs-amd64"
    elif arch == "arm64":
        rootfs = "rootfs-arm64"
    else:
        Exit(f"Unsupported arch detected {arch}")

    ctx.run(
        f"wget -q https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/{rootfs}.sum -O /tmp/{rootfs}.sum"
    )

    current_sum_file = f"{KMT_ROOTFS_DIR}/{rootfs}.sum"
    if filecmp.cmp(current_sum_file, "/tmp/{rootfs}.sum"):
        print("[-] No update required for root filesystems and bootable images")

    # backup rootfs
    ctx.run("cp {KMT_ROOTFS_DIR}/{rootfs}.tar.gz {KMT_BACKUP_DIR}")
    ctx.run("cp {KMT_ROOTFS_DIR}/{rootfs}.sum {KMT_BACKUP_DIR}")
    print("[+] Backed up rootfs")

    # clean rootfs directory
    ctx.run(f"rm -f {KMT_ROOTFS_DIR}/*")

    download_rootfs(ctx, revert=True)

    print("[+] Root filesystem and bootables images updated")


@task
def update_resources(ctx):
    print("Updating resource dependencies will delete all running stacks.")
    if input("are you sure you want to continue? (y/n)") != "y":
        print("[-] Update aborted")
        return

    for stack in glob(f"{KMT_STACKS_DIR}/*"):
        destroy_stack(ctx, stack=stack)

    update_kernel_packages(ctx)
    update_rootfs(ctx)


@task
def revert_resources(ctx):
    print("Reverting resource dependencies will delete all running stacks.")
    if input("are you sure you want to revert to backups? (y/n)") != "y":
        print("[-] Revert aborted")
        return

    for stack in glob(f"{KMT_STACKS_DIR}/*"):
        destroy_stack(ctx, stack=stack)

    revert_kernel_packages(ctx)
    revert_rootfs(ctx)

    print("[+] Reverted successfully")


'''
aws-vault exec sandbox-account-admin -- pulumi up \
    -c scenario=aws/microvms \
    -c ddinfra:aws/defaultKeyPairName=usama-saqib-datadog-aws-2 \
    -c ddinfra:aws/defaultPrivateKeyPath=$HOME/.ssh/usama-saqib-datadog-aws-2.pem \
    -c ddinfra:aws/defaultPrivateKeyPassword=\
    -c ddinfra:env=aws/sandbox \
    -c ddinfra:aws/defaultARMInstanceType=m6g.metal \
    -c ddinfra:aws/defaultInstanceType=i3.metal \
    -c ddinfra:aws/defaultInstanceStorageSize=500 \
    -c microvm:microVMConfigFile="/home/usama.saqib/go/github.com/DataDog/test-infra-definitions/aws/scenarios/microVMs/sample-vm-config.json" \
    -c microvm:workingDir="/home/kernel-version-testing" \
    -c microvm:provision=false \
    -c microvm:x86AmiID="ami-0584a00dd384af6ab" \
    -c microvm:arm64AmiID="ami-0a5c054df5931fbfc" \
    -C ./aws/scenarios/microVMs \
    -s usama-saqib

'''


@task
def launch_stack(ctx, stack, test_infra_definitions="../test-infra-definitions", ssh_key="", x86_ami="", arm64_ami=""):
    if not stack_exists(stack):
        raise Exit(f"Stack {stack} does not exist. Please create with 'inv kmt.stack-create --stack=<name>'")

    if not vm_config_exists(stack):
        raise Exit(f"No {VMCONFIG} for stack {stack}. Refer to 'inv kmt.gen-config --help'")

    if not os.path.exists(test_infra_definitions):
        raise Exit("'test-infra-definitions' repository required to launc VMs")

    pulumi_cmd = [
        "aws-vault exec sandbox-account-admin",
        "--",
        "pulumi",
        "up",
        "-c scenario=aws/microvms",
        "-c ddinfra:aws/defaultKeyPairName=",
        "-c ddinfra:aws/defaultPrivateKeyPath=",
        "-c ddinfra:aws/defaultPrivateKeyPassword=",
        "-c ddinfra:env=aws/sandbox",
        "-c ddinfra:aws/defaultARMInstanceType=m6g.metal",
        "-c ddinfra:aws/defaultInstanceType=i3.metal",
        "-c ddinfra:aws/defaultInstanceStorageSize=500",
        "-c microvm:microVMConfigFile=\"/home/usama.saqib/go/github.com/DataDog/test-infra-definitions/aws/scenarios/microVMs/sample-vm-config.json\"",
        "-c microvm:workingDir=",
        "-c microvm:provision=false",
        "-c microvm:x86AmiID=ami-0584a00dd384af6ab",
        "-c microvm:arm64AmiID=ami-0a5c054df5931fbfc",
        "-C ./aws/scenarios/microVMs",
        "-s usama-saqib",
    ]
