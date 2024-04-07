from __future__ import annotations

import copy
import itertools
import json
import os
import platform
from typing import TYPE_CHECKING, Any, List, Optional, Set, Union, cast
from urllib.parse import urlparse

from invoke.context import Context

from tasks.kernel_matrix_testing.kmt_os import Linux, get_kmt_os
from tasks.kernel_matrix_testing.platforms import filter_by_ci_component, get_platforms
from tasks.kernel_matrix_testing.stacks import check_and_get_stack, create_stack, stack_exists
from tasks.kernel_matrix_testing.tool import Exit, ask, info, warn
from tasks.kernel_matrix_testing.vars import VMCONFIG, arch_mapping

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import (  # noqa: F401
        Arch,
        ArchOrLocal,
        Component,
        CustomKernel,
        DistroKernel,
        Kernel,
        PathOrStr,
        Platforms,
        Recipe,
        VMConfig,
        VMDef,
        VMSetDict,
    )

local_arch = "local"

try:
    from thefuzz import fuzz, process
except ImportError:
    process = None
    fuzz = None

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
    # Ubuntu mappings
    "ubuntu_16": "ubuntu_16.04",
    "ubuntu_18": "ubuntu_18.04",
    "ubuntu_20": "ubuntu_20.04",
    "ubuntu_22": "ubuntu_22.04",
    "ubuntu_23": "ubuntu_23.10",
    "xenial": "ubuntu_16.04",
    "bionic": "ubuntu_18.04",
    "focal": "ubuntu_20.04",
    "jammy": "ubuntu_22.04",
    "mantic": "ubuntu_23.10",
    # Amazon Linux mappings
    "amazon_4.14": "amzn_4.14",
    "amazon_5.4": "amzn_5.4",
    "amazon_5.10": "amzn_5.10",
    "amzn_4.14": "amzn_4.14",
    "amzn_414": "amzn_4.14",
    "amzn_5.4": "amzn_5.4",
    "amzn_5.10": "amzn_5.10",
    "amzn_2023": "amzn_2023",
    "amazon_2023": "amzn_2023",
    "al3": "amzn_2023",
    "amzn_3": "amzn_2023",
    # Fedora mappings
    "fedora_37": "fedora_37",
    "fedora_38": "fedora_38",
    # Debian mappings
    "debian_10": "debian_10",
    "debian_11": "debian_11",
    "debian_12": "debian_12",
    # CentOS mappings
    "centos_79": "centos_79",
    "centos_7": "centos_79",
    "centos_8": "centos_8",
    # Rocky Linux mappings
    "rocky_8.5": "rocky_8.5",
    "rocky_9.3": "rocky_9.3",
}

TICK = "\u2713"
CROSS = "\u2718"
table = [
    ["Image", "x86_64", "arm64"],
    ["ubuntu-18 (bionic)", TICK, CROSS],
    ["ubuntu-20 (focal)", TICK, TICK],
    ["ubuntu-22 (jammy)", TICK, TICK],
    ["amazon linux 2 - v4.14", TICK, TICK],
    ["amazon linux 2 - v5.4", TICK, TICK],
    ["amazon linux 2 - v5.10", TICK, TICK],
    ["fedora 37 - v6.0.7", TICK, TICK],
    ["fedora 38 - v6.2.9", TICK, TICK],
    ["debian 10 - v4.19.0", TICK, TICK],
    ["debian 11 - v5.10.0", TICK, TICK],
]


def get_vmconfig_template_file(template="system-probe"):
    return f"test/new-e2e/system-probe/config/vmconfig-{template}.json"


def get_vmconfig(file: PathOrStr) -> VMConfig:
    with open(file) as f:
        return cast('VMConfig', json.load(f))


def get_vmconfig_template(template="system-probe") -> VMConfig:
    return get_vmconfig(get_vmconfig_template_file(template))


def lte_414(version: str) -> bool:
    major, minor = version.split('.')
    return (int(major) <= 4) and (int(minor) <= 14)


def get_image_list(distro: bool, custom: bool) -> List[List[str]]:
    custom_kernels: List[List[str]] = list()
    for k in kernels:
        if lte_414(k):
            custom_kernels.append([f"custom kernel v{k}", TICK, CROSS])
        else:
            custom_kernels.append([f"custom kernel v{k}", TICK, TICK])

    if (not (distro or custom)) or (distro and custom):
        return table + custom_kernels
    elif distro:
        return table
    elif custom:
        return custom_kernels
    else:
        return []


def check_memory_and_vcpus(memory: List[Any], vcpus: List[Any]):
    for mem in memory:
        if not mem.isnumeric() or int(mem) == 0:
            raise Exit(f"Invalid values for memory provided {memory}")

    for v in vcpus:
        if not v.isnumeric or int(v) == 0:
            raise Exit(f"Invalid values for vcpu provided {v}")


def empty_config(file_path: str):
    j = json.dumps({"vmsets": []}, indent=4)
    with open(file_path, 'w') as f:
        f.write(j)


def list_possible() -> List[str]:
    distros = list(distributions.keys())
    archs = list(arch_mapping.keys())
    archs.append(local_arch)

    result: List[str] = list()
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
def normalize_vm_def(possible: List[str], vm: str) -> VMDef:
    if process is None or fuzz is None:
        raise Exit("thefuzz module is not installed, please install it to continue")

    # attempt to fuzzy match user provided vm-def with the possible list.
    res = process.extractOne(vm, possible, scorer=fuzz.token_sort_ratio)
    if res is None:
        raise Exit(f"Unable to find a match for {vm}")
    vm_def = cast(str, res[0])
    recipe, version, arch = vm_def.split('-')

    if arch != local_arch:
        arch = arch_mapping[arch]

    if recipe == "distro":
        version = distributions[version]
    elif recipe != "custom":
        raise Exit(f"Invalid recipe {recipe}")

    return recipe, version, arch


def get_custom_kernel_config(version: str, arch: ArchOrLocal) -> CustomKernel:
    if arch == local_arch:
        arch = arch_mapping[platform.machine()]

    if arch == "x86_64":
        console = "ttyS0"
    else:
        console = "ttyAMA0"

    if lte_414(version):
        extra_params = {"console": console, "systemd.unified_cgroup_hierarchy": "0"}
    else:
        extra_params = {
            "console": console,
        }

    return {
        "dir": f"kernel-v{version}.{arch}.pkg",
        "tag": version,
        "extra_params": extra_params,
    }


def xz_suffix_removed(path: str):
    if path.endswith(".xz"):
        return path[: -len(".xz")]

    return path


# This function derives the configuration for each
# unique kernel or distribution from the normalized vm-def.
# For more details on the generated configuration element, refer
# to the micro-vms scenario in test-infra-definitions
def get_kernel_config(
    platforms: Platforms, recipe: Recipe, version: str, arch: ArchOrLocal
) -> Union[DistroKernel, CustomKernel]:
    if recipe == "custom":
        return get_custom_kernel_config(version, arch)

    if arch == "local":
        arch = arch_mapping[platform.machine()]

    url_base = platforms["url_base"]
    kernel_path = platforms[arch][version]
    kernel_name = xz_suffix_removed(os.path.basename(kernel_path))

    return {"tag": version, "image_source": os.path.join(url_base, kernel_path), "dir": kernel_name}


def vmset_exists(vm_config: VMConfig, tags: Set[str]) -> bool:
    vmsets = vm_config["vmsets"]

    for vmset in vmsets:
        if set(vmset.get("tags", [])) == tags:
            return True

    return False


def kernel_in_vmset(vmset: VMSetDict, kernel: Kernel) -> bool:
    vmset_kernels = vmset.get("kernels", [])
    for k in vmset_kernels:
        if k["tag"] == kernel["tag"]:
            return True

    return False


def vmset_name(arch: ArchOrLocal, recipe: Recipe) -> str:
    return f"{recipe}_{arch}"


def add_custom_vmset(vmset: 'VMSet', vm_config: VMConfig):
    arch = vmset.arch
    if arch == local_arch:
        arch = arch_mapping[platform.machine()]

    lte = False
    for vm in vmset.vms:
        if lte_414(vm.version):
            lte = True
            break

    image_path = f"custom-bullseye.{arch}.qcow2"
    if lte:
        image_path = f"custom-buster.{arch}.qcow2"

    if vmset_exists(vm_config, vmset.tags):
        return

    new_set = cast(
        'VMSetDict',
        dict(
            tags=list(vmset.tags),
            recipe=f"{vmset.recipe}-{vmset.arch}",
            arch=vmset.arch,
            kernels=list(),
            image={
                "image_path": image_path,
                "image_source": f"https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/rootfs/{image_path}",
            },
        ),
    )

    vm_config["vmsets"].append(new_set)


def add_vmset(vmset: 'VMSet', vm_config: VMConfig):
    if vmset_exists(vm_config, vmset.tags):
        return

    if vmset.recipe == "custom":
        return add_custom_vmset(vmset, vm_config)

    new_set = cast(
        'VMSetDict', dict(tags=list(vmset.tags), recipe=f"{vmset.recipe}-{vmset.arch}", arch=vmset.arch, kernels=list())
    )

    vm_config["vmsets"].append(new_set)


def add_kernel(vm_config: VMConfig, kernel: Kernel, tags: Set[str]):
    for vmset in vm_config["vmsets"]:
        if set(vmset.get("tags", [])) != tags:
            continue

        if not kernel_in_vmset(vmset, kernel):
            if "kernels" not in vmset:
                vmset["kernels"] = list()
            vmset["kernels"].append(kernel)
            return

    raise Exit(f"Unable to find vmset with tags {tags}")


def add_vcpu(vmset: VMSetDict, vcpu: List[int]):
    vmset["vcpu"] = vcpu


def add_memory(vmset: VMSetDict, memory: List[int]):
    vmset["memory"] = memory


def template_name(arch: ArchOrLocal, recipe: str) -> str:
    if arch == local_arch:
        arch = arch_mapping[platform.machine()]

    recipe_without_arch = recipe.split("-")[0]
    return f"{recipe_without_arch}_{arch}"


def add_machine_type(vmconfig_template: VMConfig, vmset: VMSetDict):
    tname = template_name(vmset.get("arch", 'local'), vmset.get("recipe", ""))
    for template in vmconfig_template["vmsets"]:
        if tname not in template.get("tags", []):
            continue

        if "machine" not in template:
            return

        vmset["machine"] = template["machine"]


def add_disks(vmconfig_template: VMConfig, vmset: VMSetDict):
    tname = template_name(vmset.get("arch", 'local'), vmset.get("recipe", ""))

    for template in vmconfig_template["vmsets"]:
        if tname in template.get("tags", []):
            vmset["disks"] = copy.deepcopy(template.get("disks", []))

            if "arch" not in vmset:
                raise Exit("arch is not defined in vmset")
            if vmset["arch"] == local_arch:
                kmt_os = get_kmt_os()
            else:
                # Remote VMs are always Linux instances
                kmt_os = Linux

            for disk in vmset.get("disks", []):
                disk["target"] = disk["target"].replace("%KMTDIR%", os.fspath(kmt_os.kmt_dir))


def add_console(vmset: VMSetDict):
    vmset["console_type"] = "file"


def url_to_fspath(url: str) -> str:
    source = urlparse(url)
    filename = os.path.basename(source.path)
    filename = xz_suffix_removed(os.path.basename(source.path))

    return f"file://{os.path.join(get_kmt_os().rootfs_dir,filename)}"


def image_source_to_path(vmset: VMSetDict):
    if vmset.get("recipe") == f"custom-{vmset.get('arch')}":
        if "image" not in vmset:
            raise Exit("image not found in vmset")

        vmset["image"]["image_source"] = url_to_fspath(vmset["image"]["image_source"])
        return

    for kernel in cast(List['DistroKernel'], vmset.get("kernels", [])):
        kernel["image_source"] = url_to_fspath(kernel["image_source"])

    for disk in vmset.get("disks", []):
        disk["source"] = url_to_fspath(disk["source"])


class VM:
    def __init__(self, version: str):
        self.version = version

    def __repr__(self):
        return f"<VM> {self.version}"


class VMSet:
    def __init__(self, arch: ArchOrLocal, recipe: Recipe, tags: Set[str]):
        self.arch: ArchOrLocal = arch
        self.recipe: Recipe = recipe
        self.tags: Set[str] = tags
        self.vms: List[VM] = list()

    def __eq__(self, other: Any):
        if not isinstance(other, VMSet):
            return False

        for tag in self.tags:
            if tag not in other.tags:
                return False
        return True

    def __hash__(self):
        return hash('-'.join(self.tags))

    def __repr__(self):
        vm_str = list()
        for vm in self.vms:
            vm_str.append(vm.version)
        return f"<VMSet> tags={'-'.join(self.tags)} arch={self.arch} vms={','.join(vm_str)}"

    def add_vm_if_belongs(self, recipe: Recipe, version: str, arch: ArchOrLocal):
        if recipe == "custom":
            expected_tag = custom_version_prefix(version)
            found = False
            for tag in self.tags:
                if tag == expected_tag:
                    found = True

            if not found:
                return

        if self.recipe == recipe and self.arch == arch:
            self.vms.append(VM(version))


def custom_version_prefix(version: str) -> str:
    return "lte_414" if lte_414(version) else "gt_414"


def build_vmsets(normalized_vm_defs: List[VMDef], sets: List[str]) -> Set[VMSet]:
    vmsets: Set[VMSet] = set()
    for recipe, version, arch in normalized_vm_defs:
        if recipe == "custom":
            sets.append(custom_version_prefix(version))

        # duplicate vm if multiple sets provided by user
        for s in sets:
            vmsets.add(VMSet(arch, recipe, {vmset_name(arch, recipe), s}))

        if len(sets) == 0:
            vmsets.add(VMSet(arch, recipe, {vmset_name(arch, recipe)}))

    # map vms to vmsets
    for recipe, version, arch in normalized_vm_defs:
        for vmset in vmsets:
            vmset.add_vm_if_belongs(recipe, version, arch)

    return vmsets


def generate_vmconfig(
    vm_config: VMConfig,
    normalized_vm_defs: List[VMDef],
    vcpu: List[int],
    memory: List[int],
    sets: List[str],
    ci: bool,
    template: str,
) -> VMConfig:
    platforms = get_platforms()
    vmconfig_template = get_vmconfig_template(template)
    vmsets = build_vmsets(normalized_vm_defs, sets)

    # add new vmsets to new vm_config
    for vmset in vmsets:
        add_vmset(vmset, vm_config)

    # add vm configurations to vmsets.
    for vmset in vmsets:
        for vm in vmset.vms:
            add_kernel(
                vm_config,
                get_kernel_config(platforms, vmset.recipe, vm.version, vmset.arch),
                vmset.tags,
            )

    for vmset in vm_config["vmsets"]:
        add_vcpu(vmset, vcpu)
        add_memory(vmset, memory)
        add_machine_type(vmconfig_template, vmset)

        if vmset.get("recipe", "") != "custom":
            add_disks(vmconfig_template, vmset)

        # For local VMs we want to read images from the filesystem
        if vmset.get("arch") == local_arch:
            image_source_to_path(vmset)

        if ci:
            add_console(vmset)

    return vm_config


def ls_to_int(ls: List[Any]) -> List[int]:
    int_ls: List[int] = list()
    for elem in ls:
        int_ls.append(int(elem))

    return int_ls


def build_normalized_vm_def_set(vms: str) -> List[VMDef]:
    vm_types = vms.split(',')
    if len(vm_types) == 0:
        raise Exit("No VMs to boot provided")

    possible = list_possible()
    return [normalize_vm_def(possible, vm) for vm in vm_types]


def gen_config_for_stack(
    ctx: Context,
    stack: Optional[str],
    vms: str,
    sets: List[str],
    init_stack: bool,
    vcpu: List[int],
    memory: List[int],
    new: bool,
    ci: bool,
    template: str,
):
    stack = check_and_get_stack(stack)
    if not stack_exists(stack) and not init_stack:
        raise Exit(
            f"Stack {stack} does not exist. Please create stack first 'inv kmt.stack-create --stack={stack}, or specify --init-stack option'"
        )

    if init_stack:
        create_stack(ctx, stack)

    info(f"[+] Select stack {stack}")

    ## get all possible (recipe, version, arch) combinations we can support.
    vmconfig_file = f"{get_kmt_os().stacks_dir}/{stack}/{VMCONFIG}"
    if new or not os.path.exists(vmconfig_file):
        empty_config(vmconfig_file)

    with open(vmconfig_file) as f:
        orig_vm_config = f.read()
    vm_config = json.loads(orig_vm_config)

    vm_config = generate_vmconfig(vm_config, build_normalized_vm_def_set(vms), vcpu, memory, sets, ci, template)
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


def list_all_distro_normalized_vms(archs: List[Arch], component: Optional[Component] = None):
    platforms = get_platforms()
    if component is not None:
        platforms = filter_by_ci_component(platforms, component)

    vms: List[VMDef] = list()
    for arch in archs:
        for distro in platforms[arch]:
            vms.append(("distro", distro, arch))

    return vms


def gen_config(
    ctx: Context,
    stack: Optional[str],
    vms: str,
    sets: str,
    init_stack: bool,
    vcpu: str,
    memory: str,
    new: bool,
    ci: bool,
    arch: str,
    output_file: PathOrStr,
    template: Component,
):
    vcpu_ls = vcpu.split(',')
    memory_ls = memory.split(',')

    check_memory_and_vcpus(memory_ls, vcpu_ls)
    set_ls = list()
    if sets != "":
        set_ls = sets.split(",")

    if not ci:
        return gen_config_for_stack(
            ctx,
            stack,
            vms,
            set_ls,
            init_stack,
            ls_to_int(vcpu_ls),
            ls_to_int(memory_ls),
            new,
            ci,
            template,
        )

    arch_ls: List[Arch] = ["x86_64", "arm64"]
    if arch != "":
        arch_ls = [arch_mapping[arch]]

    vms_to_generate = list_all_distro_normalized_vms(arch_ls, template)
    vm_config = generate_vmconfig(
        {"vmsets": []}, vms_to_generate, ls_to_int(vcpu_ls), ls_to_int(memory_ls), set_ls, ci, template
    )

    with open(output_file, "w") as f:
        f.write(json.dumps(vm_config, indent=4))
