from __future__ import annotations

import functools
import re
from typing import TYPE_CHECKING

from invoke.context import Context

from tasks.kernel_matrix_testing.platforms import get_platforms
from tasks.kernel_matrix_testing.tool import error, get_binary_target_arch, info, warn
from tasks.kernel_matrix_testing.vars import KMT_SUPPORTED_ARCHS, KMTPaths
from tasks.libs.types.arch import ARCH_AMD64, ARCH_ARM64, get_arch
from tasks.system_probe import get_ebpf_build_dir

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import Component

    SelftestResult = tuple[bool | None, str]


def selftest_pulumi(ctx: Context, _: bool) -> SelftestResult:
    """Tests that pulumi is installed and can be run."""
    res = ctx.run("pulumi --non-interactive plugin ls", hide=True, warn=True)
    if res is None or not res.ok:
        return False, "Cannot run pulumi, check installation"
    return True, "pulumi is installed"


def selftest_platforms_json(ctx: Context, _: bool) -> SelftestResult:
    """Checks that platforms.json file is readable and correct."""
    try:
        plat = get_platforms()
    except Exception as e:
        return False, f"Cannot read platforms.json file: {e}"

    image_vers: set[str] = set()
    for arch in KMT_SUPPORTED_ARCHS:
        if arch not in plat:
            return False, f"Missing {arch} in platforms.json file"

        for image, data in plat[arch].items():
            if "image_version" not in data:
                return False, f"Image {image} does not have image_version field"
            image_vers.add(data["image_version"])

    if len(image_vers) != 1:
        return False, f"Multiple image versions found: {image_vers}"

    res = ctx.run("inv -e kmt.ls", hide=True, warn=True)
    if res is None or not res.ok:
        return False, "Cannot run inv -e kmt.ls, platforms.json file might be incorrect"
    return True, "platforms.json file exists, is readable and is correct"


def selftest_prepare(ctx: Context, _: bool, component: Component, cross_compile: bool) -> SelftestResult:
    """Ensures that we can run kmt.prepare for a given component."""
    stack = f"selftest-prepare-{component}-xbuild{cross_compile}"
    arch = get_arch("local")
    target = arch
    if cross_compile:
        if target == ARCH_AMD64:
            target = ARCH_ARM64
        else:
            target = ARCH_AMD64
        arch_arg = f"--arch={target.kmt_arch}"
    else:
        arch_arg = ""

    vms = f"{target.name}-debian11-distro"

    ctx.run(f"inv kmt.destroy-stack --stack={stack}", warn=True, hide=True)
    res = ctx.run(f"inv -e kmt.gen-config --stack={stack} --vms={vms} --init-stack --yes", warn=True)
    if res is None or not res.ok:
        return None, "Cannot generate config with inv kmt.gen-config"

    res = ctx.run(f"inv -e kmt.prepare --stack={stack} --component={component} --compile-only {arch_arg}", warn=True)
    if res is None or not res.ok:
        return False, "Cannot run inv -e kmt.prepare"

    paths = KMTPaths(f"{stack}-ddvm", target)
    testpath = paths.secagent_tests if component == "security-agent" else paths.sysprobe_tests
    if not testpath.is_dir():
        return False, f"Tests directory {testpath} not found"

    bytecode_dir = testpath / get_ebpf_build_dir(target)
    object_files = list(bytecode_dir.glob("*.o"))
    if len(object_files) == 0:
        return False, f"No object files found in {bytecode_dir}"

    runtime_dir = bytecode_dir / "runtime"
    runtime_files = list(runtime_dir.glob("*.c"))
    if len(runtime_files) == 0:
        return False, f"No runtime files found in {runtime_dir}"

    for f in runtime_files:
        if f.parent.name != "runtime":
            return False, f"Runtime file {f} is not in runtime directory"

    if component == "security-agent":
        test_binary = testpath / "pkg" / "security" / "testsuite"
    else:
        test_binary = testpath / "pkg" / "ebpf" / "testsuite"

    if not test_binary.is_file():
        return False, f"Test binary {test_binary} not found"

    binary_arch = get_binary_target_arch(ctx, test_binary)
    if binary_arch != target:
        return False, f"Binary {test_binary} has unexpected arch {binary_arch} instead of {target}"

    return True, f"inv -e kmt.prepare ran successfully for {component}"


def selftest_multiarch_test(ctx: Context, allow_infra_changes: bool) -> SelftestResult:
    stack = "selftest-test-multiarch"
    vms = "x64-debian11-distro,arm64-debian11-distro"

    if not allow_infra_changes:
        return None, "Skipping multiarch test, infra changes not allowed"

    ctx.run(f"inv kmt.destroy-stack --stack={stack}", warn=True, hide=True)
    res = ctx.run(f"inv -e kmt.gen-config --stack={stack} --vms={vms} --init-stack --yes", warn=True)
    if res is None or not res.ok:
        return None, "Cannot generate config with inv kmt.gen-config"

    res = ctx.run(f"inv kmt.launch-stack --stack={stack}", warn=True)
    if res is None or not res.ok:
        return None, "Cannot create stack with inv kmt.create-stack"

    # We just test the printk patcher as it's simple, owned by eBPF platform,
    # loads eBPF files and does not depend on other components
    res = ctx.run(f"inv -e kmt.test --stack={stack} --packages=pkg/ebpf --run='.*TestPatchPrintkNewline.*'", warn=True)
    if res is None or not res.ok:
        return False, "Cannot run inv -e kmt.test"

    return True, "inv -e kmt.test ran successfully for multiarch"


def selftest(ctx: Context, allow_infra_changes: bool = False, filter: str | None = None):
    """Run all defined selftests

    :param allow_infra_changes: If true, the selftests will create the stack if it doesn't exist
    :param filter: If set, only run selftests that match the regex filter
    """
    all_selftests = [
        ("pulumi", selftest_pulumi),
        ("platforms.json", selftest_platforms_json),
        ("sysprobe-prepare", functools.partial(selftest_prepare, component="system-probe", cross_compile=False)),
        ("secagent-prepare", functools.partial(selftest_prepare, component="security-agent", cross_compile=False)),
        (
            "sysprobe-prepare x-compile",
            functools.partial(selftest_prepare, component="system-probe", cross_compile=True),
        ),
        (
            "secagent-prepare x-compile",
            functools.partial(selftest_prepare, component="security-agent", cross_compile=True),
        ),
        ("multiarch test", selftest_multiarch_test),
    ]
    results: list[tuple[str, SelftestResult]] = []

    for name, selftest in all_selftests:
        if filter is not None and not re.search(filter, name):
            warn(f"[!] Skipping {name}")
            continue

        info(f"[+] Running selftest {name}")
        try:
            results.append((name, selftest(ctx, allow_infra_changes)))
        except Exception as e:
            results.append((name, (None, f"Exception: {e}")))

    print("\nSelftest results:")

    for name, (ok, msg) in results:
        if ok is None:
            warn(f"[!] {name} couldn't complete: {msg}")
        elif ok:
            info(f"[*] {name} OK: {msg}")
        else:
            error(f"[!] {name} failed: {msg}")
