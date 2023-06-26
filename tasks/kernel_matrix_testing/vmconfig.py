from .init_kmt import KMT_STACKS_DIR, VMCONFIG, check_and_get_stack
from .download import archs_mapping, karch_mapping
from .stacks import stack_exists, create_stack
import platform
import math
import json
import itertools
from thefuzz import process
from thefuzz import fuzz
import os
from .tool import Exit, ask, warn, info

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
    "amazon_4.14": "amzn_4.14",
    "amazon_5.4": "amzn_5.4",
    "amazon_5.10": "amzn_5.10",
    "amazon_5.15": "amzn_5.15",
    "amzn_4.14": "amzn_4.14",
    "amzn_5.4": "amzn_5.4",
    "amzn_5.10": "amzn_5.10",
    "amzn_5.15": "amzn_5.15",
}
distro_arch_mapping = {"x86_64": "amd64", "arm64": "arm64"}
images_path = {
    "bionic": "file:///home/kernel-version-testing/rootfs/bionic-server-cloudimg-{arch}.qcow2",
    "focal": "file:///home/kernel-version-testing/rootfs/focal-server-cloudimg-{arch}.qcow2",
    "jammy": "file:///home/kernel-version-testing/rootfs/jammy-server-cloudimg-{arch}.qcow2",
    "bullseye": "file:///home/kernel-version-testing/rootfs/bullseye.qcow2.{arch}-DEV",
    "buster": "file:///home/kernel-version-testing/rootfs/buster.qcow2.{arch}-DEV",
    "amzn_4.14": "file:///home/kernel-version-testing/rootfs/amzn2-kvm-2.0-{arch}-4.14.qcow2",
    "amzn_5.4": "file:///home/kernel-version-testing/rootfs/amzn2-kvm-2.0-{arch}-5.4.qcow2",
    "amzn_5.10": "file:///home/kernel-version-testing/rootfs/amzn2-kvm-2.0-{arch}-5.10.qcow2",
    "amzn_5.15": "file:///home/kernel-version-testing/rootfs/amzn2-kvm-2.0-{arch}-5.15.qcow2",
}
images_name = {
    "bionic": "bionic-server-cloudimg-{arch}.qcow2",
    "focal": "focal-server-cloudimg-{arch}.qcow2",
    "jammy": "jammy-server-cloudimg-{arch}.qcow2",
    "bullseye": "bullseye.qcow2.{arch}-DEV",
    "buster": "buster.qcow2.{arch}-DEV",
    "amzn_4.14": "amzn2-kvm-2.0-{arch}-4.14.qcow2",
    "amzn_5.4": "amzn2-kvm-2.0-{arch}-5.4.qcow2",
    "amzn_5.10": "amzn2-kvm-2.0-{arch}-5.10.qcow2",
    "amzn_5.15": "amzn2-kvm-2.0-{arch}-5.15.qcow2",
}

consoles = {"x86_64": "ttyS0", "arm64": "ttyAMA0"}


def power_log_str(x):
    num = int(x)
    return str(2 ** (math.ceil(math.log(num, 2))))


def mem_to_pow_of_2(memory):
    for i in range(len(memory)):
        new = power_log_str(memory[i])
        if new != memory[i]:
            info(f"rounding up memory: {memory[i]} -> {new}")
            memory[i] = new


def check_memory_and_vcpus(memory, vcpus):
    for mem in memory:
        if not mem.isnumeric() or int(mem) == 0:
            raise Exit(f"Invalid values for memory provided {memory}")

    for v in vcpus:
        if not v.isnumeric or int(v) == 0:
            raise Exit(f"Invalid values for vcpu provided {v}")


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


# Set id uniquely categorizes each requested
# VM into particular sets.
# Each set id will contain 1 or more of the VMs requested
# by the user.
def vmset_id(recipe, version, arch):
    info(f"[+] recipe {recipe}, version {version}, arch {arch}")
    if recipe == "custom":
        if lte_414(version):
            return (recipe, arch, "lte_414")
        else:
            return (recipe, arch, "gt_414")
    else:
        return recipe, arch, "distro"


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
        "dir": images_name[version].format(arch=distro_arch_mapping[arch]),
        "tag": version,
        "image_source": images_path[version].format(arch=distro_arch_mapping[arch]),
    }


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
                "image_uri": images_path["buster"].format(arch=distro_arch_mapping[platform_arch]),
            }
        else:
            vmset["image"] = {
                "image_path": f"bullseye.qcow2.{distro_arch_mapping[platform_arch]}-DEV",
                "image_uri": images_path["bullseye"].format(arch=distro_arch_mapping[platform_arch]),
            }
    elif recipe == "distro":
        vmset = {"name": vmset_name_from_id(set_id), "recipe": f"distro-{arch}", "arch": arch, "kernels": kernels}
    else:
        raise Exit(f"Invalid recipe {recipe}")

    if arch == "arm64":
        vmset["machine"] = "virt"

    return vmset


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


def ls_to_int(ls):
    int_ls = list()
    for elem in ls:
        int_ls.append(int(elem))

    return int_ls


def gen_config(ctx, stack=None, branch=False, vms="", init_stack=False, vcpu="4", memory="8192", new=False):
    stack = check_and_get_stack(stack, branch)
    if not stack_exists(stack) and not init_stack:
        raise Exit(
            f"Stack {stack} does not exist. Please create stack first 'inv kmt.stack-create --stack={stack}, or specify --create-stack option'"
        )

    if init_stack:
        create_stack(ctx, stack)

    info(f"[+] Select stack {stack}")

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
        empty_config(vmconfig_file)

    with open(vmconfig_file) as f:
        orig_vm_config = f.read()
    vm_config = json.loads(orig_vm_config)

    generate_vm_config(vm_config, vm_types, ls_to_int(vcpu_ls), ls_to_int(memory_ls))
    vm_config_str = json.dumps(vm_config, indent=4)

    tmpfile = "/tmp/vm.json"
    with open(tmpfile, "w") as f:
        f.write(vm_config_str)

    if new:
        empty_config("/tmp/empty.json")
        ctx.run(f"git diff /tmp/empty.json {tmpfile}", warn=True)
    else:
        ctx.run(f"git diff {vmconfig_file} {tmpfile}", warn=True)

    if ask("are you sure you want to apply the diff? (y/n)") != "y":
        warn("[-] diff not applied")
        return

    with open(vmconfig_file, "w") as f:
        f.write(vm_config_str)

    info(f"[+] vmconfig @ {vmconfig_file}")
