from __future__ import annotations

import copy
import itertools
import json
import os
import random
from collections import defaultdict
from typing import TYPE_CHECKING, Any, cast
from urllib.parse import urlparse

from invoke.context import Context

from tasks.kernel_matrix_testing.kmt_os import Linux, get_kmt_os
from tasks.kernel_matrix_testing.platforms import filter_by_ci_component, get_platforms
from tasks.kernel_matrix_testing.stacks import check_and_get_stack, create_stack, destroy_stack, stack_exists
from tasks.kernel_matrix_testing.tool import Exit, ask, convert_kmt_arch_or_local, info, warn
from tasks.kernel_matrix_testing.vars import KMT_SUPPORTED_ARCHS, VMCONFIG
from tasks.libs.types.arch import ARCH_AMD64, ARCH_ARM64, Arch

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import (  # noqa: F401
        Component,
        CustomKernel,
        DistroKernel,
        Kernel,
        KMTArchName,
        KMTArchNameOrLocal,
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

TICK = "\033[32m\u2713\033[0m"
CROSS = "\033[31m\u2718\033[0m"
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


def get_image_list(distro: bool, custom: bool) -> list[list[str]]:
    headers = [
        "VM name",
        "OS ID",
        "OS Name",
        "OS Version",
        "Kernel",
        "x86_64",
        "arm64",
        "Alternative names",
        "Example VM tags to use with --vms (fuzzy matching)",
    ]
    custom_kernels: list[list[str]] = []
    for k in sorted(kernels, key=lambda x: tuple(map(int, x.split('.')))):
        if lte_414(k):
            custom_kernels.append(
                [f"custom-{k}", "debian", "Debian", "Custom", k, TICK, CROSS, "", f"custom-{k}-x86_64"]
            )
        else:
            custom_kernels.append(
                [f"custom-{k}", "debian", "Debian", "Custom", k, TICK, TICK, "", f"custom-{k}-x86_64"]
            )

    distro_kernels: list[list[str]] = []
    platforms = get_platforms()
    mappings = get_distribution_mappings()
    # Group kernels by name and kernel version, show whether one or two architectures are supported
    for arch in KMT_SUPPORTED_ARCHS:
        for name, platinfo in platforms[arch].items():
            if isinstance(platinfo, str):
                continue  # Old format

            # See if we've already added this kernel but for a different architecture. If not, create the entry.
            entry = None
            for row in distro_kernels:
                if row[0] == name and row[4] == platinfo.get('kernel'):
                    entry = row
                    break
            if entry is None:
                names = {k for k, v in mappings.items() if v == name}
                # Take two random names for the table so users get an idea of possible mappings
                names = random.choices(list(names), k=min(2, len(names)))

                entry = [
                    name,
                    platinfo.get("os_id"),
                    platinfo.get("os_name"),
                    platinfo.get("os_version"),
                    platinfo.get("kernel"),
                    CROSS,
                    CROSS,
                    ", ".join(platinfo.get("alt_version_names", [])),
                    ", ".join(f"distro-{n}-{arch}" for n in names),
                ]
                distro_kernels.append(entry)

            if arch == "x86_64":
                entry[5] = TICK
            else:
                entry[6] = TICK

    # Sort by name
    distro_kernels.sort(key=lambda x: x[0])

    table = [headers]
    if distro:
        table += distro_kernels
    if custom:
        table += custom_kernels

    return table


def check_memory_and_vcpus(memory: list[Any], vcpus: list[Any]):
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


def get_distribution_mappings() -> dict[str, str]:
    platforms = get_platforms()
    distro_mappings: dict[str, str] = {}
    alternative_spellings = {"amzn": ["amazon", "al"]}
    mapping_candidates: dict[str, set[str]] = defaultdict(
        set
    )  # Store here maps that could generate duplicates. Values are the possible targets

    for arch in KMT_SUPPORTED_ARCHS:
        for name, platinfo in platforms[arch].items():
            if isinstance(platinfo, str):
                continue  # Avoid a crash if we have the old format in the platforms file
            if name in distro_mappings:
                continue  # Ignore already existing images (from other arch)

            distro_mappings[name] = name  # Direct name
            distro_mappings[name.replace('.', '')] = name  # Allow name without dots
            for alt in platinfo.get("alt_version_names", []):
                distro_mappings[alt] = name  # Alternative version names map directly to the main name

            os_id = platinfo.get("os_id", "")
            version = platinfo.get('version', "")

            if version != "":
                if (
                    os_id != "" and os_id != name.split('_')[0]
                ):  # If the os_id is different from the main name, add it too
                    distro_mappings[f"{os_id}_{version}"] = name

                for alt in alternative_spellings.get(os_id, []):
                    distro_mappings[f"{alt}_{version}"] = name

                name_no_minor_version = f"{os_id}_{version.split('.')[0]}"
                mapping_candidates[name_no_minor_version].add(name)

    # Add candidates that didn't have any duplicates
    for name, candidates in mapping_candidates.items():
        if len(candidates) == 1:
            distro_mappings[name] = candidates.pop()

    return distro_mappings


def list_possible() -> list[str]:
    distros = list(get_distribution_mappings().keys())
    archs: list[str] = list(ARCH_AMD64.spellings) + list(ARCH_ARM64.spellings) + [local_arch]

    result: list[str] = []
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
def normalize_vm_def(possible: list[str], vm: str) -> VMDef:
    if process is None or fuzz is None:
        raise Exit("thefuzz module is not installed, please install it to continue")

    # attempt to fuzzy match user provided vm-def with the possible list.
    res = process.extractOne(vm, possible, scorer=fuzz.token_sort_ratio)
    if res is None:
        raise Exit(f"Unable to find a match for {vm}")
    vm_def = cast(str, res[0])
    recipe, version, arch = vm_def.split('-')

    if arch != local_arch:
        arch = Arch.from_str(arch).kmt_arch

    if recipe == "distro":
        version = get_distribution_mappings()[version]
    elif recipe != "custom":
        raise Exit(f"Invalid recipe {recipe}")

    return recipe, version, arch


def get_custom_kernel_config(version: str, arch: KMTArchNameOrLocal) -> CustomKernel:
    arch = convert_kmt_arch_or_local(arch)
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
    platforms: Platforms, recipe: Recipe, version: str, arch: KMTArchNameOrLocal
) -> DistroKernel | CustomKernel:
    if recipe == "custom":
        return get_custom_kernel_config(version, arch)

    if arch == "local":
        arch = Arch.local().kmt_arch

    url_base = platforms["url_base"]
    platinfo = platforms[arch][version]
    if "image" not in platinfo or "image_version" not in platinfo:
        raise Exit(f"image not found in platform information for {version}")
    kernel_path = f"{platinfo['image_version']}/{platinfo['image']}"
    kernel_name = xz_suffix_removed(os.path.basename(kernel_path))

    return {"tag": version, "image_source": os.path.join(url_base, kernel_path), "dir": kernel_name}


def vmset_exists(vm_config: VMConfig, tags: set[str]) -> bool:
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


def vmset_name(arch: KMTArchNameOrLocal, recipe: Recipe) -> str:
    return f"{recipe}_{arch}"


def add_custom_vmset(vmset: VMSet, vm_config: VMConfig):
    arch = vmset.arch
    arch = convert_kmt_arch_or_local(arch)
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
        {
            "tags": list(vmset.tags),
            "recipe": f"{vmset.recipe}-{vmset.arch}",
            "arch": vmset.arch,
            "kernels": [],
            "image": {
                "image_path": image_path,
                "image_source": f"https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/rootfs/{image_path}",
            },
        },
    )

    vm_config["vmsets"].append(new_set)


def add_vmset(vmset: VMSet, vm_config: VMConfig):
    if vmset_exists(vm_config, vmset.tags):
        return

    if vmset.recipe == "custom":
        return add_custom_vmset(vmset, vm_config)

    new_set = cast(
        'VMSetDict',
        {"tags": list(vmset.tags), "recipe": f"{vmset.recipe}-{vmset.arch}", "arch": vmset.arch, "kernels": []},
    )

    vm_config["vmsets"].append(new_set)


def add_kernel(vm_config: VMConfig, kernel: Kernel, tags: set[str]):
    for vmset in vm_config["vmsets"]:
        if set(vmset.get("tags", [])) != tags:
            continue

        if not kernel_in_vmset(vmset, kernel):
            if "kernels" not in vmset:
                vmset["kernels"] = []
            vmset["kernels"].append(kernel)
            return

    raise Exit(f"Unable to find vmset with tags {tags}")


def add_vcpu(vmset: VMSetDict, vcpu: list[int]):
    vmset["vcpu"] = vcpu


def add_memory(vmset: VMSetDict, memory: list[int]):
    vmset["memory"] = memory


def template_name(arch: KMTArchNameOrLocal, recipe: str) -> str:
    arch = convert_kmt_arch_or_local(arch)
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

    for kernel in cast(list['DistroKernel'], vmset.get("kernels", [])):
        kernel["image_source"] = url_to_fspath(kernel["image_source"])

    for disk in vmset.get("disks", []):
        disk["source"] = url_to_fspath(disk["source"])


class VM:
    def __init__(self, version: str):
        self.version = version

    def __repr__(self):
        return f"<VM> {self.version}"


class VMSet:
    def __init__(self, arch: KMTArchNameOrLocal, recipe: Recipe, testset: str, tags: set[str]):
        self.arch: KMTArchNameOrLocal = arch
        self.recipe: Recipe = recipe
        self.tags: set[str] = tags
        self.vms: list[VM] = []
        self.testset: str = testset

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
        vm_str = []
        for vm in self.vms:
            vm_str.append(vm.version)
        return f"<VMSet> tags={'-'.join(self.tags)} arch={self.arch} vms={','.join(vm_str)}"

    def add_vm_if_belongs(self, recipe: Recipe, version: str, arch: KMTArchNameOrLocal, testset: str):
        if recipe == "custom":
            expected_tag = custom_version_prefix(version)
            found = False
            for tag in self.tags:
                if tag == expected_tag:
                    found = True

            if not found:
                return

        if self.recipe == recipe and self.arch == arch and self.testset == testset:
            self.vms.append(VM(version))


def custom_version_prefix(version: str) -> str:
    return "lte_414" if lte_414(version) else "gt_414"


def build_vmsets(normalized_vm_defs_by_set: dict[str, list[VMDef]]) -> set[VMSet]:
    vmsets: set[VMSet] = set()
    for testset in normalized_vm_defs_by_set:
        for recipe, _, arch in normalized_vm_defs_by_set[testset]:
            vmsets.add(VMSet(arch, recipe, testset, {vmset_name(arch, recipe), testset}))

    # map vms to vmsets
    for testset in normalized_vm_defs_by_set:
        for recipe, version, arch in normalized_vm_defs_by_set[testset]:
            for vmset in vmsets:
                vmset.add_vm_if_belongs(recipe, version, arch, testset)

    return vmsets


def generate_vmconfig(
    vm_config: VMConfig,
    normalized_vm_defs_by_set: list[VMDef],
    vcpu: list[int],
    memory: list[int],
    ci: bool,
    template: str,
) -> VMConfig:
    platforms = get_platforms()
    vmconfig_template = get_vmconfig_template(template)
    vmsets = build_vmsets(normalized_vm_defs_by_set)

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

    for vmset_config in vm_config["vmsets"]:
        add_vcpu(vmset_config, vcpu)
        add_memory(vmset_config, memory)
        add_machine_type(vmconfig_template, vmset_config)

        if vmset_config.get("recipe", "") != "custom":
            add_disks(vmconfig_template, vmset_config)

        # For local VMs we want to read images from the filesystem
        if vmset_config.get("arch") == local_arch:
            image_source_to_path(vmset_config)

        if ci:
            add_console(vmset_config)

    return vm_config


def ls_to_int(ls: list[Any]) -> list[int]:
    int_ls: list[int] = []
    for elem in ls:
        int_ls.append(int(elem))

    return int_ls


def build_normalized_vm_def_by_set(vms: str, sets: list[str]) -> dict[str, list[VMDef]]:
    vm_types = vms.split(',')
    if len(vm_types) == 0:
        raise Exit("No VMs to boot provided")

    possible = list_possible()
    normalized = [normalize_vm_def(possible, vm) for vm in vm_types]

    if len(sets) == 0:
        sets = ["default"]

    normalized_by_set = {}
    for s in sets:
        normalized_by_set[s] = normalized

    return normalized_by_set


def gen_config_for_stack(
    ctx: Context,
    stack: str | None,
    vms: str,
    sets: list[str],
    init_stack: bool,
    vcpu: list[int],
    memory: list[int],
    new: bool,
    ci: bool,
    template: str,
    yes=False,
):
    stack = check_and_get_stack(stack)
    if not stack_exists(stack) and not init_stack:
        raise Exit(
            f"Stack {stack} does not exist. Please create stack first 'inv kmt.create-stack --stack={stack}', or specify --init-stack option to the current command"
        )

    if init_stack:
        create_stack(ctx, stack)

    info(f"[+] Select stack {stack}")

    ## get all possible (recipe, version, arch) combinations we can support.
    vmconfig_file = f"{get_kmt_os().stacks_dir}/{stack}/{VMCONFIG}"
    if os.path.exists(vmconfig_file) and not new:
        raise Exit(
            "Editing configuration is currently not supported. Destroy the stack first to change the configuration."
        )

    if new or not os.path.exists(vmconfig_file):
        empty_config(vmconfig_file)

    with open(vmconfig_file) as f:
        orig_vm_config = f.read()
    vm_config = json.loads(orig_vm_config)

    vm_config = generate_vmconfig(vm_config, build_normalized_vm_def_by_set(vms, sets), vcpu, memory, ci, template)
    vm_config_str = json.dumps(vm_config, indent=4)

    tmpfile = "/tmp/vm.json"
    with open(tmpfile, "w") as f:
        f.write(vm_config_str)

    info(f"[+] We will apply the following configuration to {stack} (file: {vmconfig_file}): ")
    for vmset in vm_config["vmsets"]:
        if "arch" not in vmset:
            continue

        arch = vmset["arch"]
        if arch == local_arch:
            print(f"Local {Arch.local().name} VMs")
        else:
            print(f"Remote {arch} VMs (running in EC2 instance)")

        for cpu, mem in itertools.product(vmset.get("vcpu", []), vmset.get("memory", [])):
            for kernel in vmset.get("kernels", []):
                print(f"  - {kernel['tag']} ({cpu} vCPUs, {mem} MB memory)")

        print()

    if not yes and ask("are you sure you want to apply the diff? (y/n)") != "y":
        warn("[-] diff not applied")
        destroy_stack(ctx, stack, False, None)
        return

    with open(vmconfig_file, "w") as f:
        f.write(vm_config_str)

    info(f"[+] vmconfig @ {vmconfig_file}")


def list_all_distro_normalized_vms_by_test_set(archs: list[KMTArchName], component: Component | None = None):
    platforms = get_platforms()
    if component is not None:
        platforms_by_test_set = filter_by_ci_component(platforms, component)

    vms = {}
    for testset in platforms_by_test_set:
        for arch in archs:
            for distro in platforms_by_test_set[testset][arch]:
                if testset not in vms:
                    vms[testset] = []

                vms[testset].append(("distro", distro, arch))

    return vms


def gen_config(
    ctx: Context,
    stack: str | None,
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
    yes: bool = False,
):
    vcpu_ls = vcpu.split(',')
    memory_ls = memory.split(',')

    check_memory_and_vcpus(memory_ls, vcpu_ls)
    set_ls = []
    if sets != "":
        set_ls = sets.split(",")

    if not ci:
        return gen_config_for_stack(
            ctx, stack, vms, set_ls, init_stack, ls_to_int(vcpu_ls), ls_to_int(memory_ls), new, ci, template, yes=yes
        )

    arch_ls: list[KMTArchName] = KMT_SUPPORTED_ARCHS
    if arch != "":
        arch_ls = [Arch.from_str(arch).kmt_arch]

    vms_to_generate = list_all_distro_normalized_vms_by_test_set(arch_ls, template)
    vm_config = generate_vmconfig(
        {"vmsets": []}, vms_to_generate, ls_to_int(vcpu_ls), ls_to_int(memory_ls), ci, template
    )

    print("Generated VMSets with tags:")
    for vmset in vm_config["vmsets"]:
        tags = vmset["tags"]
        tags.sort()
        print(f"- {', '.join(tags)}")

    with open(output_file, "w") as f:
        f.write(json.dumps(vm_config, indent=4))
