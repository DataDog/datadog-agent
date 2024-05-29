from __future__ import annotations

import functools
import re

from invoke.context import Context

from tasks.kernel_matrix_testing.platforms import get_platforms
from tasks.kernel_matrix_testing.tool import error, full_arch, get_binary_target_arch, info, warn
from tasks.kernel_matrix_testing.types import Component
from tasks.kernel_matrix_testing.vars import KMTPaths, arch_ls

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
    for arch in arch_ls:
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


def selftest_prepare(ctx: Context, allow_infra_changes: bool, component: Component) -> SelftestResult:
    """Ensures that we can run kmt.prepare for a given component.

    If allow_infra_changes is true, the stack will be created if it doesn't exist.
    """
    stack = "selftest-prepare"
    arch = full_arch("local")
    vms = f"{arch}-debian11-distro"

    ctx.run(f"inv kmt.destroy-stack --stack={stack}", warn=True, hide=True)
    res = ctx.run(f"inv -e kmt.gen-config --stack={stack} --vms={vms} --init-stack --yes", warn=True)
    if res is None or not res.ok:
        return None, "Cannot generate config with inv kmt.gen-config"

    if allow_infra_changes:
        res = ctx.run(f"inv kmt.launch-stack --stack={stack}", warn=True)
        if res is None or not res.ok:
            return None, "Cannot create stack with inv kmt.create-stack"

    compile_only = "--compile-only" if not allow_infra_changes else ""
    res = ctx.run(f"inv -e kmt.prepare --stack={stack} --component={component} {compile_only}", warn=True)
    if res is None or not res.ok:
        return False, "Cannot run inv -e kmt.prepare"

    paths = KMTPaths(stack, arch)
    testpath = paths.secagent_tests if component == "security-agent" else paths.sysprobe_tests
    if not testpath.is_dir():
        return False, f"Tests directory {testpath} not found"

    bytecode_dir = testpath / "pkg" / "ebpf" / "bytecode" / "build"
    object_files = list(bytecode_dir.glob("*.o"))
    if len(object_files) == 0:
        return False, f"No object files found in {bytecode_dir}"

    if component == "security-agent":
        test_binary = testpath / "pkg" / "security" / "testsuite"
    else:
        test_binary = testpath / "pkg" / "ebpf" / "testsuite"

    if not test_binary.is_file():
        return False, f"Test binary {test_binary} not found"

    binary_arch = get_binary_target_arch(ctx, test_binary)
    if binary_arch != arch:
        return False, f"Binary {test_binary} has unexpected arch {binary_arch} instead of {arch}"

    return True, f"inv -e kmt.prepare ran successfully for {component}"


def selftest(ctx: Context, allow_infra_changes: bool = False, filter: str | None = None):
    """Run all defined selftests

    :param allow_infra_changes: If true, the selftests will create the stack if it doesn't exist
    :param filter: If set, only run selftests that match the regex filter
    """
    all_selftests = [
        ("pulumi", selftest_pulumi),
        ("platforms.json", selftest_platforms_json),
        ("sysprobe-prepare", functools.partial(selftest_prepare, component="system-probe")),
        ("secagent-prepare", functools.partial(selftest_prepare, component="security-agent")),
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
