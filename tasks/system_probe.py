from __future__ import annotations

import contextlib
import glob
import itertools
import json
import os
import platform
import re
import shutil
import string
import sys
import tempfile
from pathlib import Path
from subprocess import check_output

import requests
import yaml
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.build_tags import UNIT_TEST_TAGS, get_default_build_tags
from tasks.flavor import AgentFlavor
from tasks.libs.build.bazel import bazel
from tasks.libs.build.ninja import NinjaWriter
from tasks.libs.ciproviders.gitlab_api import ReferenceTag
from tasks.libs.common.color import color_message
from tasks.libs.common.git import get_commit_sha
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import (
    REPO_PATH,
    bin_name,
    get_build_flags,
    get_common_test_args,
    get_embedded_path,
    parse_kernel_version,
)
from tasks.libs.types.arch import ALL_ARCHS, Arch

BIN_DIR = os.path.join(".", "bin", "system-probe")
BIN_PATH = os.path.join(BIN_DIR, bin_name("system-probe"))

BPF_TAG = "linux_bpf"
BUNDLE_TAG = "ebpf_bindata"
NPM_TAG = "npm"

TEST_DIR = os.getenv('DD_AGENT_TESTING_DIR') or os.path.normpath(os.path.join(os.getcwd(), "test", "new-e2e", "tests"))
E2E_ARTIFACT_DIR = os.path.join(TEST_DIR, "sysprobe-functional/artifacts")
TEST_PACKAGES_LIST = [
    "./pkg/ebpf/...",
    "./pkg/network/...",
    "./pkg/collector/corechecks/ebpf/...",
    "./pkg/discovery/module/...",
    "./pkg/process/monitor/...",
    "./pkg/dyninst/...",
    "./pkg/gpu/...",
    "./pkg/logs/launchers/file/",
    "./pkg/privileged-logs/test/...",
    "./pkg/system-probe/config/...",
    "./comp/metadata/inventoryagent/...",
]
TEST_PACKAGES = " ".join(TEST_PACKAGES_LIST)
# change `timeouts` in `test/new-e2e/system-probe/test-runner/main.go` if you change them here
TEST_TIMEOUTS = {
    "pkg/network/protocols": "5m",
    "pkg/network/protocols/http$": "15m",
    "pkg/network/tracer$": "55m",
    "pkg/network/usm$": "55m",
    "pkg/network/usm/tests$": "55m",
}
EMBEDDED_SHARE_DIR = os.path.join("/opt", "datadog-agent", "embedded", "share", "system-probe", "ebpf")

is_windows = sys.platform == "win32"
is_macos = sys.platform == "darwin"

arch_mapping = {
    "amd64": "x64",
    "x86_64": "x64",
    "x64": "x64",
    "i386": "x86",
    "i686": "x86",
    "aarch64": "arm64",  # linux
    "arm64": "arm64",  # darwin
}
CURRENT_ARCH = arch_mapping.get(platform.machine(), "x64")
# system-probe doesn't depend on any particular version of libpcap so use the latest one (as of 2024-10-28)
# this version should be kept in sync with the one in the agent omnibus build
LIBPCAP_VERSION = "1.10.5"

TEST_HELPER_CBINS = ["cudasample"]

RUST_BINARIES = [
    "pkg/discovery/module/rust",
]


def get_ebpf_build_dir(arch: Arch) -> Path:
    return Path("pkg/ebpf/bytecode/build") / arch.kmt_arch  # Use KMT arch names for compatibility with CI


def get_ebpf_runtime_dir() -> Path:
    return Path("pkg/ebpf/bytecode/build/runtime")


def ninja_define_ebpf_compiler(
    nw: NinjaWriter,
    strip_object_files=False,
    kernel_release=None,
    with_unit_test=False,
    arch: Arch | None = None,
):
    if arch is not None and arch.is_cross_compiling():
        # -target ARCH is important even if we're just emitting LLVM. If we're cross-compiling, clang
        # might fail to interpret cross-arch assembly code (e.g, the headers with arm64-specific ASM code
        # of the linux kernel will fail compilation in x64 hosts due to unknown register names).
        nw.variable("target", f"-target {arch.gcc_arch} -emit-llvm")
    else:
        nw.variable("target", "-emit-llvm")
    nw.variable("ebpfflags", get_ebpf_build_flags(with_unit_test, arch=arch))
    nw.variable("kheaders", get_kernel_headers_flags(kernel_release, arch=arch))
    nw.rule(
        name="ebpfclang",
        command="/opt/datadog-agent/embedded/bin/clang-bpf -MD -MF $out.d $target $ebpfflags $kheaders $flags -c $in -o $out",
        depfile="$out.d",
    )

    strip = "/opt/datadog-agent/embedded/bin/llvm-strip -g $out"
    strip_lbb = "/opt/datadog-agent/embedded/bin/llvm-strip -w -N \"LBB*\" $out"
    strip_part = f"&& {strip} && {strip_lbb}" if strip_object_files else ""

    nw.rule(
        name="llc",
        command=f"/opt/datadog-agent/embedded/bin/llc-bpf -march=bpf -filetype=obj -o $out $in {strip_part}",
    )


def ninja_define_exe_compiler(nw: NinjaWriter, compiler='clang'):
    nw.rule(
        name="exe" + compiler,
        command=f"{compiler} -MD -MF $out.d $exeflags $flags $in -o $out $exelibs",
        depfile="$out.d",
    )


@task
def build_libpcap(ctx, env: dict, arch: Arch | None = None):
    """Download and build libpcap as a static library in the agent dev directory.
    The library is not rebuilt if it already exists.
    """
    embedded_path = get_embedded_path(ctx)
    assert embedded_path, "Failed to find embedded path"
    target_file = os.path.join(embedded_path, "embedded", "lib", "libpcap.a")
    if os.path.exists(target_file):
        version = ctx.run(f"strings {target_file} | grep -E '^libpcap version' | cut -d ' ' -f 3").stdout.strip()
        if version == LIBPCAP_VERSION:
            ctx.run(f"echo 'libpcap version {version} already exists at {target_file}'")
            return

    bazel(ctx, "run", "--", "@libpcap//:install", f"--destdir={embedded_path}")
    ctx.run(f"strip -g {target_file}")
    return


def get_libpcap_cgo_flags(ctx, install_path: str = None):
    """Return a dictionary with the CGO flags needed to link against libpcap.
    If install_path is provided, then we expect this path to contain libpcap as a shared library.
    """
    if install_path is not None:
        return {
            'CGO_CFLAGS': f"-I{os.path.join(install_path, 'embedded', 'include')}",
            'CGO_LDFLAGS': f"-L{os.path.join(install_path, 'embedded', 'lib')}",
        }
    else:
        embedded_path = get_embedded_path(ctx)
        assert embedded_path, "Failed to find embedded path"
        return {
            'CGO_CFLAGS': f"-I{os.path.join(embedded_path, 'embedded', 'include')}",
            'CGO_LDFLAGS': f"-L{os.path.join(embedded_path, 'embedded', 'lib')}",
        }


@task
def build(
    ctx,
    race=False,
    rebuild=False,
    go_mod="readonly",
    arch: str = CURRENT_ARCH,
    bundle_ebpf=False,
    strip_binary=False,
    static=False,
    fips_mode=False,
    glibc=True,
):
    """
    Build the system-probe
    """
    if not is_macos:
        build_object_files(ctx)

    build_sysprobe_binary(
        ctx,
        bundle_ebpf=bundle_ebpf,
        go_mod=go_mod,
        race=race,
        rebuild=rebuild,
        strip_binary=strip_binary,
        arch=arch,
        static=static,
        fips_mode=fips_mode,
        glibc=glibc,
    )


@task
def clean(
    ctx,
):
    clean_object_files(
        ctx,
    )
    ctx.run("go clean -cache")


@task
def build_sysprobe_binary(
    ctx,
    race=False,
    rebuild=False,
    go_mod="readonly",
    arch: str = CURRENT_ARCH,
    binary=BIN_PATH,
    install_path=None,
    bundle_ebpf=False,
    strip_binary=False,
    fips_mode=False,
    static=False,
    glibc=True,
) -> None:
    arch_obj = Arch.from_str(arch)

    ldflags, gcflags, env = get_build_flags(
        ctx,
        install_path=install_path,
        arch=arch_obj,
        static=static,
    )

    build_tags = get_default_build_tags(
        build="system-probe", flavor=AgentFlavor.fips if fips_mode else AgentFlavor.base
    )
    if bundle_ebpf:
        build_tags.append(BUNDLE_TAG)
    if strip_binary:
        ldflags += ' -s -w'

    if static:
        build_tags.extend(["osusergo", "netgo", "static"])
        build_tags = list(set(build_tags).difference({"netcgo"}))

    if not glibc:
        build_tags = list(set(build_tags).difference({"nvml"}))

    if not is_windows and "pcap" in build_tags:
        build_libpcap(ctx, arch=arch_obj, env=env)
        cgo_flags = get_libpcap_cgo_flags(ctx, install_path)
        # append libpcap cgo-related environment variables to any existing ones
        for k, v in cgo_flags.items():
            if k in env:
                env[k] += f" {v}"
            else:
                env[k] = v

    if os.path.exists(binary):
        os.remove(binary)

    go_build(
        ctx,
        f"{REPO_PATH}/cmd/system-probe",
        mod=go_mod,
        race=race,
        rebuild=rebuild,
        build_tags=build_tags,
        bin_path=binary,
        gcflags=gcflags,
        ldflags=ldflags,
        check_deadcode=os.getenv("DEPLOY_AGENT") == "true",
        coverage=os.getenv("E2E_COVERAGE_PIPELINE") == "true",
        env=env,
    )


def get_sysprobe_test_buildtags(is_windows, bundle_ebpf):
    platform = "windows" if is_windows else "linux"
    build_tags = get_default_build_tags(build="system-probe", platform=platform)

    if not is_windows and bundle_ebpf:
        build_tags.append(BUNDLE_TAG)

    # Some flags are not supported on KMT testing, so we remove them
    # until we have extra fixes (mainly coming from the unified build images)
    temporarily_unsupported_build_tags = [
        "pcap",  # libpcap headers not supported yet, specially for cross-compilation
        "trivy",  # trivy introduces dependencies on a higher version of glibc
    ]
    for tag in temporarily_unsupported_build_tags:
        if tag in build_tags:
            build_tags.remove(tag)

    build_tags.extend(UNIT_TEST_TAGS)

    return build_tags


@task(iterable=['extra_tags'])
def test(
    ctx,
    packages=TEST_PACKAGES,
    bundle_ebpf=False,
    output_path=None,
    skip_object_files=False,
    run=None,
    failfast=False,
    timeout=None,
    extra_arguments="",
    extra_tags: list[str] | None = None,
):
    """
    Run tests on eBPF parts
    If skip_object_files is set to True, this won't rebuild object files
    If output_path is set, we run `go test` with the flags `-c -o output_path`, which *compiles* the test suite
    into a single binary. This artifact is meant to be used in conjunction with e2e tests.
    """
    if os.getenv("GOPATH") is None:
        raise Exit(
            code=1,
            message="GOPATH is not set, if you are running tests with sudo, you may need to use the -E option to "
            "preserve your environment",
        )

    if not skip_object_files:
        build_object_files(ctx)

    build_tags = get_sysprobe_test_buildtags(is_windows, bundle_ebpf)

    if extra_tags:
        build_tags.extend(extra_tags)

    args = get_common_test_args(build_tags, failfast)
    args["output_params"] = f"-c -o {output_path}" if output_path else ""
    args["run"] = f"-run {run}" if run else ""
    args["go"] = "go"
    args["sudo"] = "sudo -E " if not is_windows and not output_path and not is_root() else ""
    args["extra_arguments"] = extra_arguments

    _, _, env = get_build_flags(ctx)
    env["DD_SYSTEM_PROBE_BPF_DIR"] = EMBEDDED_SHARE_DIR

    go_root = os.getenv("GOROOT")
    if go_root:
        args["go"] = os.path.join(go_root, "bin", "go")

    failed_pkgs = []
    package_dirs = go_package_dirs(packages.split(" "), build_tags)
    # we iterate over the packages here to get the nice streaming test output
    for pdir in package_dirs:
        args["dir"] = pdir
        testto = timeout if timeout else get_test_timeout(pdir)
        args["timeout"] = f"-timeout {testto}" if testto else ""
        cmd = '{sudo}{go} test -mod=readonly -v {failfast} {timeout} -tags "{build_tags}" {extra_arguments} {output_params} {dir} {run}'
        res = ctx.run(cmd.format(**args), env=env, warn=True)
        if res.exited is None or res.exited > 0:
            failed_pkgs.append(os.path.relpath(pdir, ctx.cwd))
            if failfast:
                break

    if len(failed_pkgs) > 0:
        print(color_message("failed packages:\n" + "\n".join(failed_pkgs), "red"))
        raise Exit(code=1, message="system-probe tests failed")


@task(
    help={
        "package": "The package to test. REQUIRED ",
        "skip_object_files": "Skip rebuilding the object files.",
        "run": "The name of the test to run. REQUIRED",
    }
)
def test_debug(
    ctx,
    package,
    run,
    bundle_ebpf=False,
    skip_object_files=False,
    failfast=False,
):
    """
    Run delve on a specific system-probe test.
    """

    if os.getenv("GOPATH") is None:
        raise Exit(
            code=1,
            message="GOPATH is not set, if you are running tests with sudo, you may need to use the -E option to "
            "preserve your environment",
        )

    if not skip_object_files:
        build_object_files(ctx)

    build_tags = [NPM_TAG]
    build_tags.extend(UNIT_TEST_TAGS)
    if not is_windows:
        build_tags.append(BPF_TAG)
        if bundle_ebpf:
            build_tags.append(BUNDLE_TAG)

    args = get_common_test_args(build_tags, failfast)
    args["run"] = run
    args["dlv"] = "dlv"
    args["sudo"] = "sudo -E " if not is_windows and not is_root() else ""
    args["dir"] = package

    _, _, env = get_build_flags(ctx)
    env["DD_SYSTEM_PROBE_BPF_DIR"] = EMBEDDED_SHARE_DIR

    cmd = '{sudo}{dlv} test {dir} --build-flags="-mod=readonly -v {failfast} -tags={build_tags}" -- -test.run {run}'
    ctx.run(cmd.format(**args), env=env, pty=True, warn=True)


def get_test_timeout(pkg):
    for tt, to in TEST_TIMEOUTS.items():
        if re.search(tt, pkg) is not None:
            return to
    return None


@contextlib.contextmanager
def chdir(dirname=None):
    curdir = os.getcwd()
    try:
        if dirname is not None:
            os.chdir(dirname)
        yield
    finally:
        os.chdir(curdir)


def go_package_dirs(packages, build_tags):
    """
    Retrieve a list of all packages we want to test
    This handles the ellipsis notation (eg. ./pkg/ebpf/...)
    """

    format_arg = '{{ .Dir }}'
    buildtags_arg = ",".join(build_tags)

    # Prepend module path if the package path is relative
    # and doesn't start with ./ (which go list handles correctly for local paths)
    if not is_windows:
        full_path_packages = []
        module_path = "github.com/DataDog/datadog-agent/"
        for pkg in packages:
            if not pkg.startswith(".") and not pkg.startswith(module_path):
                full_path_packages.append(module_path + pkg)
            else:
                full_path_packages.append(pkg)
        packages_arg = " ".join(full_path_packages)
    else:
        packages_arg = " ".join(packages)

    # Disable buildvcs to avoid attempting to invoke git, which can be slow in
    # VMs and we don't need its output here.
    cmd = f"go list -find -buildvcs=false -f \"{format_arg}\" -mod=readonly -tags \"{buildtags_arg}\" {packages_arg}"

    target_packages = [p.strip() for p in check_output(cmd, shell=True, encoding='utf-8').split("\n")]
    return [p for p in target_packages if len(p) > 0]


BUILD_COMMIT = os.path.join(E2E_ARTIFACT_DIR, "build.commit")


def clean_build(ctx):
    if not os.path.exists(E2E_ARTIFACT_DIR):
        return True

    if not os.path.exists(BUILD_COMMIT):
        return True

    # if this build happens on a new commit do it cleanly
    with open(BUILD_COMMIT) as f:
        build_commit = f.read().rstrip()
        curr_commit = get_commit_sha(ctx)
        if curr_commit != build_commit:
            return True

    return False


def full_pkg_path(name):
    return os.path.join(os.getcwd(), name[name.index("pkg") :])


@task
def e2e_prepare(ctx, ci=False, packages=""):
    """
    Compile test suite for e2e tests
    """
    build_tags = [NPM_TAG]
    if not is_windows:
        build_tags.append(BPF_TAG)

    target_packages = go_package_dirs(TEST_PACKAGES_LIST, build_tags)

    # Clean up previous build
    if os.path.exists(E2E_ARTIFACT_DIR) and (packages == "" or clean_build(ctx)):
        shutil.rmtree(E2E_ARTIFACT_DIR)
    elif packages != "":
        packages = [full_pkg_path(name) for name in packages.split(",")]
        # make sure valid packages were provided.
        for pkg in packages:
            if pkg not in target_packages:
                raise Exit(f"Unknown target packages {pkg} specified")

        target_packages = packages

    if os.path.exists(BUILD_COMMIT):
        os.remove(BUILD_COMMIT)

    os.makedirs(E2E_ARTIFACT_DIR, exist_ok=True)

    # clean target_packages only
    for pkg_dir in target_packages:
        test_dir = pkg_dir.lstrip(os.getcwd())
        if os.path.exists(os.path.join(E2E_ARTIFACT_DIR, test_dir)):
            shutil.rmtree(os.path.join(E2E_ARTIFACT_DIR, test_dir))

    # This will compile one 'testsuite' file per package by running `go test -c -o output_path`.
    # These artifacts will be "vendored" inside:
    # test/new-e2e/tests/sysprobe-functional/artifacts/pkg/network/testsuite
    # test/new-e2e/tests/sysprobe-functional/artifacts/pkg/network/netlink/testsuite
    # test/new-e2e/tests/sysprobe-functional/artifacts/pkg/ebpf/testsuite
    # test/new-e2e/tests/sysprobe-functional/artifacts/pkg/ebpf/bytecode/testsuite
    for i, pkg in enumerate(target_packages):
        target_path = os.path.join(E2E_ARTIFACT_DIR, os.path.relpath(pkg, os.getcwd()))
        target_bin = "testsuite"
        if is_windows:
            target_bin = "testsuite.exe"

        test(
            ctx,
            packages=pkg,
            skip_object_files=(i != 0),
            bundle_ebpf=False,
            output_path=os.path.join(target_path, target_bin),
        )

        # copy ancillary data, if applicable
        for extra in ["testdata", "build"]:
            extra_path = os.path.join(pkg, extra)
            if os.path.isdir(extra_path):
                shutil.copytree(extra_path, os.path.join(target_path, extra))

        for gobin in [
            "external_unix_proxy_server",
            "fmapper",
            "gotls_client",
            "gotls_server",
            "grpc_external_server",
            "prefetch_file",
            "fake_server",
            "sample_service",
            "standalone_attacher",
        ]:
            src_file_path = os.path.join(pkg, f"{gobin}.go")
            if not is_windows and os.path.isdir(pkg) and os.path.isfile(src_file_path):
                binary_path = os.path.join(target_path, gobin)
                with chdir(pkg):
                    go_build(
                        ctx, f"{gobin}.go", build_tags=["test"], ldflags="-extldflags '-static'", bin_path=binary_path
                    )

        for cbin in TEST_HELPER_CBINS:
            source = Path(pkg) / "testdata" / f"{cbin}.c"
            if not is_windows and source.is_file():
                binary = Path(target_path) / cbin
                ctx.run(f"clang -static -o {binary} {source}")

    gopath = os.getenv("GOPATH")
    copy_files = [
        "/opt/datadog-agent/embedded/bin/clang-bpf",
        "/opt/datadog-agent/embedded/bin/llc-bpf",
        f"{gopath}/bin/gotestsum",
    ]

    files_dir = os.path.join(E2E_ARTIFACT_DIR, "..")
    for cf in copy_files:
        if os.path.exists(cf):
            shutil.copy(cf, files_dir)

    go_build(ctx, "cmd/test2json", ldflags="-s -w", bin_path=f"{files_dir}/test2json", env={"CGO_ENABLED": "0"})
    ctx.run(f"echo {get_commit_sha(ctx)} > {BUILD_COMMIT}")


def get_linux_header_dirs(
    kernel_release: str | None = None,
    minimal_kernel_release: tuple[int, int, int] | None = None,
    arch: Arch | None = None,
) -> list[Path]:
    """Return a list of paths to the linux header directories for the given parameters.

    Raises ValueError if no kernel paths can be found

    :param kernel_release: The kernel release to use. If not provided, the current kernel release is used.
        If no headers are found for the given kernel release, the function will try to find the headers for
        some common kernel releases.
    :param minimal_kernel_release: The minimal kernel release to use. If provided, the function will discard
        any headers that are older than the minimal kernel release.
    :param arch: The architecture to use. If not provided, the current architecture is used. If no headers are
        found for the given architecture, the function will try to find the headers for any architecture.
    """
    if not kernel_release:
        os_info = os.uname()
        kernel_release = os_info.release
    kernel_release_vers = parse_kernel_version(kernel_release)

    if arch is None:
        arch = Arch.local()

    # Possible paths where the kernel headers can be found
    kernels_path = Path("/usr/src/kernels")
    usr_src_path = Path("/usr/src")
    lib_modules_path = Path("/lib/modules")

    # Get all possible candidates, we will filter them later based on the criteria given
    # by the arguments.
    candidates: set[Path] = set()
    if kernels_path.is_dir():
        # /usr/src/kernels doesn't always exist, so we check first. The other paths
        # are expected to exist, so we do want to raise an exception if they don't, as
        # it's an unexpected situation.
        candidates.update(kernels_path.iterdir())
    candidates.update(d for d in usr_src_path.iterdir() if d.name.startswith("linux-"))
    candidates.update(lib_modules_path.glob("*/build"))
    candidates.update(lib_modules_path.glob("*/source"))

    # Many of the candidates might be symlinks, resolve and de-duplicate
    candidates = {c.resolve() for c in candidates if c.is_dir()}

    # Inspect the paths and compute a priority for each of them based on how well
    # they match the restrictions given by our arguments.
    # Also, maintain a sort order to ensure that headers are included in the right position.
    # Priority and sort order will be the first two elements of each tuple of the list.
    paths_with_priority_and_sort_order: list[tuple[int, int, Path]] = []
    discarded_paths: list[tuple[str, Path]] = []  # Keep track of the discarded paths so we can debug failures
    for path in candidates:
        # Get the kernel name, discard when we cannot get a kernel version out of them
        candidate_kernel = path.name.removeprefix("linux-headers-").removeprefix("linux-kbuild-")
        try:
            candidate_kernel_vers = parse_kernel_version(candidate_kernel)
        except ValueError:
            discarded_paths.append(("no kernel version", path))
            continue

        priority = 0
        sort_order = 100

        # If the kernel version matches increase priority, this is the best match.
        if candidate_kernel_vers == kernel_release_vers:
            priority += 1

        # Completely discard kernels that don't match the minimal version
        if minimal_kernel_release is not None and candidate_kernel_vers < minimal_kernel_release:
            discarded_paths.append(
                (f"kernel version {candidate_kernel_vers} less than minimal {minimal_kernel_release}", path)
            )
            continue

        # Give more priority to kernels that match the desired architecture.
        matching_kernel_archs = {a for a in ALL_ARCHS if any(x in candidate_kernel for x in a.spellings)}
        if arch in matching_kernel_archs:
            sort_order = 0  # Matching architecture paths should be sorted the first
            priority += 1
        elif len(matching_kernel_archs) == 0:
            # If we find no match, assume it's a common path (e.g., -common folders in Debian)
            # which matches everything
            sort_order = 1  # Common folders should be after arch-specific ones
            priority += 1

        # Don't add duplicates
        if not any(p == path for _, _, p in paths_with_priority_and_sort_order):
            paths_with_priority_and_sort_order.append((priority, sort_order, path))

    if len(paths_with_priority_and_sort_order) == 0:
        raise ValueError(f"No kernel header path found. Discarded paths and reasons: {discarded_paths}")

    # Only get paths with maximum priority, those are the ones that match the best.
    # Note that there might be multiple of them (e.g., the arch-specific and the common path)
    max_priority = max(prio for prio, _, _ in paths_with_priority_and_sort_order)
    unsorted_linux_headers = [
        (path, ord) for prio, ord, path in paths_with_priority_and_sort_order if prio == max_priority
    ]

    # Include sort order is important, ensure we respect the sort order we defined while
    # discovering the paths. Also, in case of equal sort order, sort by path name to ensure
    # a deterministic order (useful to stop ninja from rebuilding on reordering of headers).
    linux_headers = [path for path, _ in sorted(unsorted_linux_headers, key=lambda x: (x[1], x[0]))]

    # Now construct all subdirectories. Again, order is important, so keep the list
    subdirs = [
        "include",
        "include/uapi",
        "include/generated/uapi",
        f"arch/{arch.kernel_arch}/include",
        f"arch/{arch.kernel_arch}/include/uapi",
        f"arch/{arch.kernel_arch}/include/generated",
        f"arch/{arch.kernel_arch}/include/generated/uapi",
    ]

    dirs: list[Path] = []
    for d in linux_headers:
        for s in subdirs:
            dirs.append(d / s)

    return dirs


@task
def print_linux_include_paths(_: Context, arch: str | None = None):
    """
    Print the result of the linux header directories discovery. Useful for debugging the build process.
    """
    paths = get_linux_header_dirs(arch=Arch.from_str(arch or "local"))
    print("\n".join(str(p) for p in paths))


def get_ebpf_build_flags(unit_test=False, arch: Arch | None = None):
    flags = []
    flags.extend(
        [
            '-D__KERNEL__',
            '-DCONFIG_64BIT',
            '-D__BPF_TRACING__',
            '-DKBUILD_MODNAME=\\"ddsysprobe\\"',
            '-DCOMPILE_PREBUILT',
        ]
    )
    if arch is not None:
        if arch.kernel_arch is None:
            raise Exit(f"eBPF architecture not supported for {arch}")
        flags.append(f"-D__TARGET_ARCH_{arch.kernel_arch}")
        flags.append(f"-D__{arch.gcc_arch.replace('-', '_')}__")

    if unit_test:
        flags.extend(['-D__BALOUM__'])
    flags.extend(
        [
            '-Wno-unused-value',
            '-Wno-pointer-sign',
            '-Wno-compare-distinct-pointer-types',
            '-Wunused',
            '-Wall',
            '-Werror',
        ]
    )
    flags.extend(["-include pkg/ebpf/c/asm_goto_workaround.h"])
    flags.extend(["-O2"])
    flags.extend(
        [
            # Some linux distributions enable stack protector by default which is not available on eBPF
            '-fno-stack-protector',
            '-fno-color-diagnostics',
            '-fno-unwind-tables',
            '-fno-asynchronous-unwind-tables',
            '-fno-jump-tables',
            '-fmerge-all-constants',
        ]
    )
    flags.extend(["-Ipkg/ebpf/c"])
    return flags


def get_kernel_headers_flags(kernel_release=None, minimal_kernel_release=None, arch: Arch | None = None):
    return [
        f"-isystem{d}"
        for d in get_linux_header_dirs(
            kernel_release=kernel_release, minimal_kernel_release=minimal_kernel_release, arch=arch
        )
    ]


def check_for_inline(ctx):
    print("checking for invalid inline usage...")
    src_dirs = ["pkg/ebpf/c/", "pkg/network/ebpf/c/", "pkg/security/ebpf/c/"]
    grep_filter = "--include='*.c' --include '*.h'"
    grep_exclude = "--exclude='bpf_helpers.h'"
    pattern = "'^[^/]*\\binline\\b'"
    grep_res = ctx.run(f"grep -n {grep_filter} {grep_exclude} -r {pattern} {' '.join(src_dirs)}", warn=True, hide=True)
    if grep_res.ok:
        print(color_message("Use __always_inline instead of inline:", "red"))
        print(grep_res.stdout)
        raise Exit(code=1)


def get_clang_version_and_build_version() -> tuple[str, str]:
    gitlab_ci_file = Path(__file__).parent.parent / ".gitlab-ci.yml"
    yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
    with open(gitlab_ci_file) as f:
        ci_config = yaml.safe_load(f)

    ci_vars = ci_config['variables']
    return ci_vars['CLANG_LLVM_VER'], ci_vars['CLANG_BUILD_VERSION']


def setup_runtime_clang(
    ctx: Context, arch: Arch | None = None, target_dir: Path | str = "/opt/datadog-agent/embedded/bin"
) -> None:
    target_dir = Path(target_dir)
    needs_sudo = not os.access(target_dir, os.W_OK)
    sudo = "sudo" if not is_root() and needs_sudo else ""

    if arch is None:
        arch = Arch.local()

    clang_version, clang_build_version = get_clang_version_and_build_version()

    runtime_binaries = {
        "clang-bpf": {"url_prefix": "clang", "version_line": 0, "needs_download": False},
        "llc-bpf": {"url_prefix": "llc", "version_line": 1, "needs_download": False},
        "llvm-strip": {"url_prefix": "llvm-strip", "version_line": 2, "needs_download": False},
    }

    for binary, meta in runtime_binaries.items():
        binary_path = target_dir / binary
        if not arch.is_cross_compiling() and sys.platform == "linux":
            if not binary_path.exists() or binary_path.stat().st_size == 0:
                print(f"'{binary}' missing")
                runtime_binaries[binary]["needs_download"] = True
                continue

            # We can check the version of clang and llc on the system, we have the same arch and can
            # execute the binaries. This way we can omit the download if the binaries exist and the version
            # matches the desired one
            res = ctx.run(f"{sudo} {binary_path} --version", warn=True, hide=True)
            if res is not None and res.ok:
                version_str = res.stdout.split("\n")[meta["version_line"]].strip().split(" ")[2].strip()
                if version_str != clang_version:
                    print(f"'{binary}' version '{version_str}' is not required version '{clang_version}'")
                    runtime_binaries[binary]["needs_download"] = True
        else:
            # If we're cross-compiling we cannot check the version of clang and llc on the system,
            # so we download them only if they don't exist
            runtime_binaries[binary]["needs_download"] = not binary_path.exists() or binary_path.stat().st_size == 0

    if not target_dir.exists():
        ctx.run(f"{sudo} mkdir -p {target_dir}")

    for binary, meta in runtime_binaries.items():
        if not meta["needs_download"]:
            continue

        # download correct version from dd-agent-omnibus S3 bucket
        binary_url = f"https://dd-agent-omnibus.s3.amazonaws.com/llvm/{meta['url_prefix']}-{clang_version}.{arch.name}.{clang_build_version}"
        binary_path = target_dir / binary
        print(f"'{binary}' downloading...")
        ctx.run(f"{sudo} wget -nv {binary_url} -O {binary_path}")
        ctx.run(f"{sudo} chmod 0755 {binary_path}")


@task
def validate_object_file_metadata(ctx: Context, build_dir: str | Path = "pkg/ebpf/bytecode/build", verbose=True):
    build_dir = Path(build_dir)
    missing_metadata_files = 0
    total_metadata_files = 0
    print(f"Validating metadata of eBPF object files in {build_dir}...")

    for file in build_dir.glob("**/*.o"):
        total_metadata_files += 1
        res = ctx.run(f"readelf -p dd_metadata {file}", warn=True, hide=True)
        if res is None or not res.ok:
            print(color_message(f"- {file}: missing metadata", "red"))
            missing_metadata_files += 1
            continue

        groups = re.findall(r"<(?P<key>[^:]+):(?P<value>[^>]+)>", res.stdout)
        if groups is None or len(groups) == 0:
            print(color_message(f"- {file}: invalid metadata", "red"))
            missing_metadata_files += 1
            continue

        if verbose:
            metadata = ", ".join(f"{k}={v}" for k, v in groups)
            print(color_message(f"- {file}: {metadata}", "green"))

    if missing_metadata_files > 0:
        raise Exit(
            f"{missing_metadata_files} object files are missing metadata. Remember to include the bpf_metadata.h header in all eBPF programs"
        )
    else:
        print(f"All {total_metadata_files} object files have valid metadata")


# All Bazel eBPF targets, grouped by output directory.
# Prebuilt targets go to build_dir/, CO-RE targets go to build_dir/co-re/.
_BAZEL_EBPF_PREBUILT_TARGETS = [
    "//pkg/network/ebpf/c/prebuilt:dns",
    "//pkg/network/ebpf/c/prebuilt:dns-debug",
    "//pkg/network/ebpf/c/prebuilt:offset-guess",
    "//pkg/network/ebpf/c/prebuilt:offset-guess-debug",
    "//pkg/network/ebpf/c/prebuilt:tracer",
    "//pkg/network/ebpf/c/prebuilt:tracer-debug",
    "//pkg/network/ebpf/c/prebuilt:usm",
    "//pkg/network/ebpf/c/prebuilt:usm-debug",
    "//pkg/network/ebpf/c/prebuilt:usm_events_test",
    "//pkg/network/ebpf/c/prebuilt:usm_events_test-debug",
    "//pkg/network/ebpf/c/prebuilt:shared-libraries",
    "//pkg/network/ebpf/c/prebuilt:shared-libraries-debug",
    "//pkg/network/ebpf/c/prebuilt:conntrack",
    "//pkg/network/ebpf/c/prebuilt:conntrack-debug",
    "//pkg/security/ebpf/c/prebuilt:runtime-security",
    "//pkg/security/ebpf/c/prebuilt:runtime-security-syscall-wrapper",
    "//pkg/security/ebpf/c/prebuilt:runtime-security-fentry",
    "//pkg/security/ebpf/c/prebuilt:runtime-security-offset-guesser",
]

_BAZEL_EBPF_CORE_TARGETS = [
    "//pkg/ebpf/c:lock_contention",
    "//pkg/ebpf/c:ksyms_iter",
    "//pkg/network/ebpf/c:tracer",
    "//pkg/network/ebpf/c:tracer-debug",
    "//pkg/network/ebpf/c/co-re:tracer-fentry",
    "//pkg/network/ebpf/c/co-re:tracer-fentry-debug",
    "//pkg/network/ebpf/c/runtime:usm",
    "//pkg/network/ebpf/c/runtime:usm-debug",
    "//pkg/network/ebpf/c/runtime:shared-libraries",
    "//pkg/network/ebpf/c/runtime:shared-libraries-debug",
    "//pkg/network/ebpf/c/runtime:conntrack",
    "//pkg/network/ebpf/c/runtime:conntrack-debug",
    "//pkg/collector/corechecks/ebpf/c/runtime:oom-kill",
    "//pkg/collector/corechecks/ebpf/c/runtime:oom-kill-debug",
    "//pkg/collector/corechecks/ebpf/c/runtime:tcp-queue-length",
    "//pkg/collector/corechecks/ebpf/c/runtime:tcp-queue-length-debug",
    "//pkg/collector/corechecks/ebpf/c/runtime:ebpf",
    "//pkg/collector/corechecks/ebpf/c/runtime:ebpf-debug",
    "//pkg/collector/corechecks/ebpf/c/runtime:noisy-neighbor",
    "//pkg/collector/corechecks/ebpf/c/runtime:noisy-neighbor-debug",
    "//pkg/gpu/ebpf/c/runtime:gpu",
    "//pkg/gpu/ebpf/c/runtime:gpu-debug",
    "//pkg/dyninst/ebpf:dyninst_event",
    "//pkg/dyninst/ebpf:dyninst_event-debug",
    "//pkg/ebpf/testdata/c:logdebug-test",
    "//pkg/ebpf/testdata/c:error_telemetry",
    "//pkg/ebpf/testdata/c:uprobe_attacher-test",
    "//cmd/system-probe/subcommands/ebpf/testdata:btf_test",
]

# Targets that go to their own source directory, not build_dir/co-re/
_BAZEL_EBPF_INPLACE_TARGETS = {
    "//pkg/ebpf/kernelbugs/c:uprobe-trigger": "pkg/ebpf/kernelbugs/c",
    "//pkg/ebpf/kernelbugs/c:detect-seccomp-bug": "pkg/ebpf/kernelbugs/c",
}

_BAZEL_RUNTIME_FLAT_TARGETS = [
    "//pkg/ebpf/bytecode:oom-kill_flat",
    "//pkg/ebpf/bytecode:tcp-queue-length_flat",
    "//pkg/ebpf/bytecode:usm_flat",
    "//pkg/ebpf/bytecode:shared-libraries_flat",
    "//pkg/ebpf/bytecode:conntrack_flat",
    "//pkg/ebpf/bytecode:tracer_flat",
    "//pkg/ebpf/bytecode:offsetguess-test_flat",
    "//pkg/ebpf/bytecode:runtime-security_flat",
    "//pkg/ebpf/bytecode:gpu_flat",
]

# _gen targets produce the Go integrity hash files (pkg/ebpf/bytecode/runtime/<name>.go).
_BAZEL_RUNTIME_GEN_TARGETS = [
    "//pkg/ebpf/bytecode:oom-kill_gen",
    "//pkg/ebpf/bytecode:tcp-queue-length_gen",
    "//pkg/ebpf/bytecode:usm_gen",
    "//pkg/ebpf/bytecode:shared-libraries_gen",
    "//pkg/ebpf/bytecode:conntrack_gen",
    "//pkg/ebpf/bytecode:tracer_gen",
    "//pkg/ebpf/bytecode:offsetguess-test_gen",
    "//pkg/ebpf/bytecode:runtime-security_gen",
    "//pkg/ebpf/bytecode:gpu_gen",
]

_NON_EBPF_TARGETS = frozenset(
    [
        "//pkg/ebpf/kernelbugs/c:detect-seccomp-bug",
    ]
)


def _ebpf_strip_targets(targets, strip):
    """Append .stripped suffix to eBPF targets when strip is requested.

    Non-eBPF targets (e.g. cc_binary) are returned unchanged since they
    don't have Bazel-side stripped variants.
    """
    if not strip:
        return list(targets)
    return [t + ".stripped" if t not in _NON_EBPF_TARGETS else t for t in targets]


def ebpf_bazel_flags(arch: Arch) -> list[str]:
    """Return extra Bazel flags needed for eBPF cross-compilation."""
    if arch.is_cross_compiling():
        return [f"--//bazel/rules/ebpf:target_arch={arch.gcc_arch}"]
    return []


def bazel_build_ebpf(ctx: Context, arch: Arch, build_dir: str, runtime_dir: str, strip: bool = True) -> None:
    """Build all eBPF artifacts via a single ``bazel build``.

    Builds eBPF .o objects (prebuilt, CO-RE, inplace), runtime flattened .c
    files, and Go integrity hash files, then copies outputs to the
    appropriate staging directories and source tree.
    """
    import shutil

    # detect-seccomp-bug is x86-only (has target_compatible_with in Bazel)
    if arch == Arch.from_str("arm64"):
        inplace_targets = {t: d for t, d in _BAZEL_EBPF_INPLACE_TARGETS.items() if "detect-seccomp-bug" not in t}
    else:
        inplace_targets = _BAZEL_EBPF_INPLACE_TARGETS

    prebuilt = _ebpf_strip_targets(_BAZEL_EBPF_PREBUILT_TARGETS, strip)
    core = _ebpf_strip_targets(_BAZEL_EBPF_CORE_TARGETS, strip)
    inplace = {_ebpf_strip_targets([t], strip)[0]: d for t, d in inplace_targets.items()}

    ebpf_targets = prebuilt + core + list(inplace.keys())
    all_build_targets = ebpf_targets + list(_BAZEL_RUNTIME_FLAT_TARGETS) + list(_BAZEL_RUNTIME_GEN_TARGETS)

    extra_flags = ebpf_bazel_flags(arch)

    print(f"Building {len(all_build_targets)} eBPF + runtime targets via Bazel...")
    bazel(ctx, "build", *extra_flags, *all_build_targets)
    bazel_bin = bazel(ctx, "info", "bazel-bin", capture_output=True).strip()

    co_re_dir = os.path.join(build_dir, "co-re")
    os.makedirs(build_dir, exist_ok=True)
    os.makedirs(co_re_dir, exist_ok=True)
    os.makedirs(runtime_dir, exist_ok=True)

    def _copy_output(target: str, dest_dir: str):
        label_path, name = target.lstrip("/").rsplit(":", 1)
        dest_name = name.removesuffix(".stripped")

        src_o = os.path.join(bazel_bin, label_path, f"{name}.o")
        src_bin = os.path.join(bazel_bin, label_path, name)

        # Only use mtime fast-path when strip mode hasn't changed.
        same_mode = name == dest_name

        if os.path.exists(src_o):
            dst = os.path.join(dest_dir, f"{dest_name}.o")
            if same_mode and os.path.exists(dst) and os.path.getmtime(dst) >= os.path.getmtime(src_o):
                return
            if os.path.exists(dst):
                os.chmod(dst, 0o644)
            shutil.copy2(src_o, dst)
            os.chmod(dst, 0o644)
        elif os.path.exists(src_bin):
            dst = os.path.join(dest_dir, dest_name)
            if same_mode and os.path.exists(dst) and os.path.getmtime(dst) >= os.path.getmtime(src_bin):
                return
            if os.path.exists(dst):
                os.chmod(dst, 0o755)
            shutil.copy2(src_bin, dst)
            os.chmod(dst, 0o755)
        else:
            print(f"Warning: expected output {src_o} or {src_bin} not found")

    for target in prebuilt:
        _copy_output(target, build_dir)

    for target in core:
        _copy_output(target, co_re_dir)

    for target, dest in inplace.items():
        os.makedirs(dest, exist_ok=True)
        _copy_output(target, dest)

    print(f"Copied eBPF objects to {build_dir}")

    # Copy runtime flattened .c files to staging directory.
    for target in _BAZEL_RUNTIME_FLAT_TARGETS:
        label_path, name = target.lstrip("/").rsplit(":", 1)
        # run_binary output is under <name>/<out_name>.c; the directory
        # name matches the macro name which equals out_name for all bundles.
        bundle_name = name.removesuffix("_flat")
        src = os.path.join(bazel_bin, label_path, bundle_name, f"{bundle_name}.c")
        dst = os.path.join(runtime_dir, f"{bundle_name}.c")
        if os.path.exists(src):
            if os.path.exists(dst):
                os.chmod(dst, 0o644)
            shutil.copy2(src, dst)
            os.chmod(dst, 0o644)
        else:
            print(f"Warning: expected runtime bundle output {src} not found")

    print(f"Copied runtime bundles to {runtime_dir}")

    # Copy generated Go integrity hash files to source tree.
    go_dest = os.path.join("pkg", "ebpf", "bytecode", "runtime")
    os.makedirs(go_dest, exist_ok=True)
    for target in _BAZEL_RUNTIME_GEN_TARGETS:
        label_path, name = target.lstrip("/").rsplit(":", 1)
        bundle_name = name.removesuffix("_gen")
        src = os.path.join(bazel_bin, label_path, bundle_name, f"{bundle_name}.go")
        dst = os.path.join(go_dest, f"{bundle_name}.go")
        if os.path.exists(src):
            if os.path.exists(dst):
                os.chmod(dst, 0o644)
            shutil.copy2(src, dst)
            os.chmod(dst, 0o644)
        else:
            print(f"Warning: expected runtime hash output {src} not found")

    print(f"Copied runtime hash files to {go_dest}")


# Paths under bazel-bin -> repo-relative destinations (also removed by clean_object_files).
_BAZEL_WINDOWS_RESOURCE_COPIES = (
    ("pkg/util/winutil/messagestrings/rsrc.syso", "pkg/util/winutil/messagestrings/rsrc.syso"),
    ("pkg/util/winutil/messagestrings/messagestrings.h", "pkg/util/winutil/messagestrings/messagestrings.h"),
    ("cmd/system-probe/windows_resources/rsrc.syso", "cmd/system-probe/rsrc.syso"),
)


def bazel_build_windows_resources(ctx: Context) -> None:
    """Build Windows resource files (.syso) via Bazel and copy to the source tree.

    Replaces the ninja-based windmc/windres pipeline for system-probe.
    Produces:
      - pkg/util/winutil/messagestrings/rsrc.syso + messagestrings.h  (shared message table)
      - cmd/system-probe/rsrc.syso                                    (system-probe versioninfo)
    """
    import shutil

    targets = [
        "//pkg/util/winutil/messagestrings:messagetable",
        "//cmd/system-probe/windows_resources:rsrc",
    ]
    bazel(ctx, "build", *targets)
    bazel_bin = bazel(ctx, "info", "bazel-bin", capture_output=True).strip()

    copies = [(os.path.join(bazel_bin, bazel_rel), dst) for bazel_rel, dst in _BAZEL_WINDOWS_RESOURCE_COPIES]
    for src, dst in copies:
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        shutil.copyfile(src, dst)

    print("Copied Windows resource files to source tree")


@task(aliases=["object-files"])
def build_object_files(
    ctx,
    arch: str = CURRENT_ARCH,
) -> None:
    arch_obj = Arch.from_str(arch)
    build_dir = get_ebpf_build_dir(arch_obj)
    runtime_dir = get_ebpf_runtime_dir()

    arch_flags = ebpf_bazel_flags(arch_obj)

    if not is_windows:
        check_for_inline(ctx)
        ctx.run(f"mkdir -p -m 0755 {runtime_dir}")
        ctx.run(f"mkdir -p -m 0755 {build_dir}/co-re")

        # Install Bazel-managed LLVM BPF tools (needed for stripping and runtime compilation).
        sudo = "" if is_root() else "sudo"
        ctx.run(f"{sudo} mkdir -p /opt/datadog-agent/embedded/bin")
        bazel(ctx, "run", *arch_flags, "--", "@llvm_bpf//:install", "--destdir=/opt/datadog-agent", sudo=not is_root())

        # Build eBPF .o files via Bazel
        bazel_build_ebpf(ctx, arch_obj, build_dir, runtime_dir)

    if is_windows:
        bazel_build_windows_resources(ctx)

    # Verify all committed cgo godefs files are up to date.
    # The test_suite skips platform-incompatible tests via target_compatible_with.
    bazel(ctx, "test", *arch_flags, "//pkg/ebpf:verify_generated_files")

    validate_object_file_metadata(ctx, build_dir, verbose=False)

    build_rust_binaries(ctx, arch=arch_obj)

    if not is_windows:
        sudo = "" if is_root() else "sudo"
        ctx.run(f"{sudo} mkdir -p {EMBEDDED_SHARE_DIR}")

        if ctx.run("command -v rsync >/dev/null 2>&1", warn=True, hide=True).ok:
            rsync_filter = "--filter='+ */' --filter='+ *.o' --filter='+ *.c' --filter='- *'"
            ctx.run(
                f"{sudo} rsync --chmod=F644 --chown=root:root -rvt {rsync_filter} {build_dir}/ {EMBEDDED_SHARE_DIR}"
            )
            ctx.run(
                f"{sudo} rsync --chmod=F644 --chown=root:root -rvt {rsync_filter} {runtime_dir}/ {EMBEDDED_SHARE_DIR}/runtime"
            )
        else:
            with ctx.cd(build_dir):

                def cp_cmd(out_dir):
                    dest = os.path.join(EMBEDDED_SHARE_DIR, out_dir)
                    return " ".join(
                        [
                            f"-execdir cp -vp {{}} {dest}/ \\;",
                            f"-execdir chown root:root {dest}/{{}} \\;",
                            f"-execdir chmod 0644 {dest}/{{}} \\;",
                        ]
                    )

                ctx.run(f"{sudo} find . -maxdepth 1 -type f -name '*.o' {cp_cmd('.')}")
                ctx.run(f"{sudo} mkdir -p {EMBEDDED_SHARE_DIR}/co-re")
                ctx.run(f"{sudo} find ./co-re -maxdepth 1 -type f -name '*.o' {cp_cmd('co-re')}")

            with ctx.cd(runtime_dir):
                ctx.run(f"{sudo} mkdir -p {EMBEDDED_SHARE_DIR}/runtime")
                ctx.run(f"{sudo} find ./ -maxdepth 1 -type f -name '*.c' {cp_cmd('runtime')}")


def build_rust_binaries(ctx: Context, arch: Arch, output_dir: Path | None = None, packages: list[str] | None = None):
    if is_windows or is_macos:
        return

    platform_map = {
        "x86_64": "//bazel/platforms:linux_x86_64",
        "arm64": "//bazel/platforms:linux_arm64",
    }

    platform_flags = []
    if arch.kmt_arch in platform_map:
        platform_flags.append(f"--platforms={platform_map[arch.kmt_arch]}")

    for source_path in RUST_BINARIES:
        if packages and not any(source_path.startswith(package) for package in packages):
            continue

        install_dest = output_dir / source_path if output_dir else Path(source_path)
        bazel(ctx, "run", *platform_flags, "--", f"@//{source_path}:install", f"--destdir={install_dest}")


_BAZEL_CWS_BALOUM_TARGETS = {
    "//pkg/security/ebpf/c/prebuilt:runtime-security-baloum": "runtime-security.o",
    "//pkg/security/ebpf/c/prebuilt:runtime-security-syscall-wrapper-baloum": "runtime-security-syscall-wrapper.o",
    "//pkg/security/ebpf/c/prebuilt:runtime-security-fentry-baloum": "runtime-security-fentry.o",
}


def build_cws_object_files(
    ctx,
    arch: str | Arch = CURRENT_ARCH,
    with_unit_test=False,
):
    import shutil

    arch_obj = Arch.from_str(arch)
    arch_flags = ebpf_bazel_flags(arch_obj)
    build_dir = get_ebpf_build_dir(arch_obj)
    runtime_dir = get_ebpf_runtime_dir()
    bazel_build_ebpf(ctx, arch_obj, str(build_dir), str(runtime_dir))
    bazel(ctx, "test", *arch_flags, "//pkg/ebpf:verify_generated_files")

    if with_unit_test:
        targets = list(_BAZEL_CWS_BALOUM_TARGETS.keys())
        bazel(ctx, "build", *arch_flags, *targets)
        bazel_bin = bazel(ctx, "info", "bazel-bin", capture_output=True).strip()

        for target, dest_name in _BAZEL_CWS_BALOUM_TARGETS.items():
            label_path, name = target.lstrip("/").rsplit(":", 1)
            src = os.path.join(bazel_bin, label_path, f"{name}.o")
            dst = os.path.join(str(build_dir), dest_name)
            shutil.copy2(src, dst)
            os.chmod(dst, 0o444)


def clean_object_files(ctx):
    build_root = Path("pkg/ebpf/bytecode/build")
    if build_root.exists():
        shutil.rmtree(build_root)

    for target, dest_dir in _BAZEL_EBPF_INPLACE_TARGETS.items():
        name = target.rsplit(":", 1)[1]
        for candidate in [Path(dest_dir) / f"{name}.o", Path(dest_dir) / name]:
            if candidate.exists():
                candidate.unlink()

    go_runtime = Path("pkg/ebpf/bytecode/runtime")
    for target in _BAZEL_RUNTIME_GEN_TARGETS:
        name = target.rsplit(":", 1)[1]
        bundle_name = name.removesuffix("_gen")
        candidate = go_runtime / f"{bundle_name}.go"
        if candidate.exists():
            candidate.unlink()

    for _, dst in _BAZEL_WINDOWS_RESOURCE_COPIES:
        p = Path(dst)
        if p.exists():
            p.unlink()


@task
def generate_lookup_tables(ctx):
    if is_windows:
        return

    lookup_table_generate_files = [
        "./pkg/network/go/goid/main.go",
        "./pkg/network/protocols/http/gotls/lookup/main.go",
    ]
    for file in lookup_table_generate_files:
        ctx.run(f"go generate {file}")


def is_root():
    return os.getuid() == 0


def check_for_ninja(ctx):
    if is_windows:
        ctx.run("where ninja")
    else:
        ctx.run("which ninja")
    ctx.run("ninja --version")


# list of programs we do not want to minimize against
no_minimize = ["lock_contention.o"]


@task(iterable=['bpf_programs'])
def generate_minimized_btfs(ctx, source_dir, output_dir, bpf_programs):
    """
    Given an input directory containing compressed full-sized BTFs, generates an identically-structured
    output directory containing compressed minimized versions of those BTFs, tailored to the given
    bpf program(s).
    """

    # If there are no input programs, we don't need to actually do anything; however, in order to
    # prevent CI jobs from failing, we'll create a dummy output directory
    if len(bpf_programs) == 0:
        ctx.run(f"mkdir -p {output_dir}/dummy_data")
        return

    if len(bpf_programs) == 1 and os.path.isdir(bpf_programs[0]):
        programs_dir = os.path.abspath(bpf_programs[0])
        print(f"using all object files from directory {programs_dir}")
        bpf_programs = glob.glob(f"{programs_dir}/*.o")

    newlist = []
    for prog_path in bpf_programs:
        prog = os.path.basename(prog_path)
        if prog not in no_minimize:
            newlist.append(prog_path)

    bpf_programs = newlist

    ctx.run(f"mkdir -p {output_dir}")

    check_for_ninja(ctx)

    ninja_file_path = os.path.join(ctx.cwd, 'generate-minimized-btfs.ninja')
    with open(ninja_file_path, 'w') as ninja_file:
        nw = NinjaWriter(ninja_file, width=180)

        nw.rule(name="decompress_btf", command="tar -xf $in -C $target_directory")
        nw.rule(name="minimize_btf", command="bpftool gen min_core_btf $in $out $input_bpf_programs")

        for root, dirs, files in os.walk(source_dir):
            path_from_root = os.path.relpath(root, source_dir)

            for d in dirs:
                output_subdir = os.path.join(output_dir, path_from_root, d)
                os.makedirs(output_subdir, exist_ok=True)

            for file in files:
                if not file.endswith(".btf.tar.xz"):
                    continue

                btf_filename = file.removesuffix(".tar.xz")
                minimized_btf_path = os.path.join(output_dir, path_from_root, btf_filename)

                nw.build(
                    rule="decompress_btf",
                    inputs=[os.path.join(root, file)],
                    outputs=[os.path.join(root, btf_filename)],
                    variables={
                        "target_directory": root,
                    },
                )

                nw.build(
                    rule="minimize_btf",
                    inputs=[os.path.join(root, btf_filename)],
                    outputs=[minimized_btf_path],
                    variables={
                        "input_bpf_programs": bpf_programs,
                    },
                )

    ctx.run(f"ninja -f {ninja_file_path}", env={"NINJA_STATUS": "(%r running) (%c/s) (%es) [%f/%t] "})


def compute_go_parallelism(debug: bool = False, ci: bool | None = None) -> int:
    """
    Compute Go build parallelism.

    Uses platform-specific heuristics: macOS=1 (due to Go bugs), CI=4,
    local=(CPU_count/2)+1. Can be overridden with NINJA_GO_PARALLELISM env var.

    Args:
        debug: Enable debug logging
        ci: Force CI mode (conservative defaults)

    Returns:
        Number of parallel Go builds to run (>= 1)
    """

    def log(message: str):
        print(message, flush=True, file=sys.stderr)

    def debug_log(message: str):
        if debug:
            log(message)

    # We want to bound the number of go builds to run concurrently to be well
    # below the core count to avoid OOMing the system.
    env_override = os.environ.get("NINJA_GO_PARALLELISM")
    go_parallelism = None
    if env_override:
        try:
            go_parallelism = int(env_override)
            log(f"[+] Using parallelism of {go_parallelism} for Go builds (NINJA_GO_PARALLELISM)")
        except ValueError:
            log(f"Invalid value for NINJA_GO_PARALLELISM: {env_override}. Using parallelism of 1.")
            go_parallelism = 1
    else:
        if sys.platform == "darwin":
            # On macOS there's some bug with running Go builds in parallel.
            reason = "on macOS, see https://github.com/golang/go/issues/59657"
            go_parallelism = 1
        elif ci:
            # Arbitrary, but high enough for a win and low enough to not OOM.
            reason = "CI"
            go_parallelism = 4
        else:
            # This heuristic is okay but it runs into trouble in containers
            # in docker or k8s where the limits on CPU are much lower than the
            # host has cores. This is the situation in CI
            reason = "derived from host CPU count"
            go_parallelism = (os.cpu_count() / 2) + 1
        debug_log(
            f"[+] Using parallelism of {go_parallelism} for Go builds ({reason});"
            + " override with NINJA_GO_PARALLELISM"
        )
    return go_parallelism


@task
def process_btfhub_archive(ctx, branch="main"):
    """
    process btfhub-archive repo to only select BTF tarball files of a single architecture
    :param ctx: invoke context
    :param branch: branch of DataDog/btfhub-archive to clone
    """
    output_dir = os.getcwd()
    with tempfile.TemporaryDirectory() as temp_dir:
        with ctx.cd(temp_dir):
            clone_cmd = (
                f"git clone --depth=1 --single-branch --branch={branch} https://github.com/DataDog/btfhub-archive.git"
            )
            retries = 2
            downloaded = False

            while not downloaded and retries > 0:
                res = ctx.run(clone_cmd, warn=True)
                downloaded = res is not None and res.ok

                if not downloaded:
                    retries -= 1
                    print(f"Failed to clone btfhub-archive. Remaining retries: {retries}")

            if not downloaded:
                raise Exit("Failed to clone btfhub-archive")

            with ctx.cd("btfhub-archive"):
                # iterate over all top-level directories, which are platforms (amzn, ubuntu, etc.)
                with os.scandir(ctx.cwd) as pit:
                    for pdir in pit:
                        if not pdir.is_dir() or pdir.name.startswith("."):
                            continue

                        # iterate over second-level directories, which are release versions (2, 20.04, etc.)
                        with os.scandir(pdir.path) as rit:
                            for rdir in rit:
                                if not rdir.is_dir() or rdir.is_symlink():
                                    continue

                                # iterate over arch directories
                                with os.scandir(rdir.path) as ait:
                                    for adir in ait:
                                        if not adir.is_dir() or adir.name not in {"x86_64", "arm64"}:
                                            continue

                                        print(f"{pdir.name}/{rdir.name}/{adir.name}")
                                        src_dir = adir.path
                                        # list BTF .tar.xz files in arch dir
                                        btf_files = os.listdir(src_dir)
                                        for file in btf_files:
                                            if not file.endswith(".tar.xz"):
                                                continue
                                            src_file = os.path.join(src_dir, file)

                                            # remove release and arch from destination
                                            btfs_dir = os.path.join(temp_dir, f"btfs-{adir.name}")
                                            dst_dir = os.path.join(btfs_dir, pdir.name)
                                            # ubuntu retains release version
                                            if pdir.name == "ubuntu":
                                                dst_dir = os.path.join(btfs_dir, pdir.name, rdir.name)

                                            os.makedirs(dst_dir, exist_ok=True)
                                            dst_file = os.path.join(dst_dir, file)
                                            if os.path.exists(dst_file):
                                                raise Exit(message=f"{dst_file} already exists")

                                            shutil.move(src_file, dst_file)

        # generate both tarballs
        for arch in ["x86_64", "arm64"]:
            btfs_dir = os.path.join(temp_dir, f"btfs-{arch}")
            output_path = os.path.join(output_dir, f"btfs-{arch}.tar")
            # at least one file needs to be moved for directory to exist
            if os.path.exists(btfs_dir):
                with ctx.cd(temp_dir):
                    # include btfs-$ARCH as prefix for all paths
                    # set modification time to zero to ensure deterministic tarball
                    ctx.run(f"tar --mtime=@0 -cf {output_path} btfs-{arch}")


@task
def save_test_dockers(ctx, output_dir, arch, use_crane=False):
    if is_windows:
        return

    # crane does not accept 'x86_64' as a valid architecture
    crane_arch = arch
    if arch == "x86_64":
        crane_arch = "amd64"

    vmconfig_paths = [
        "test/new-e2e/system-probe/config/vmconfig-system-probe.json",
        "test/new-e2e/system-probe/config/vmconfig-security-agent.json",
    ]
    urls = set()
    for vmconfig_path in vmconfig_paths:
        with open(vmconfig_path) as vmconfig:
            vmjson = json.load(vmconfig)
            if "vmsets" not in vmjson:
                continue

            for vmset in vmjson["vmsets"]:
                if "disks" not in vmset:
                    continue
                if vmset["arch"] != arch:
                    continue

                for disk in vmset["disks"]:
                    if "source" not in disk:
                        continue

                    root = disk["source"].removesuffix(f"/docker-{arch}.qcow2.xz")
                    urls.add(f"{root}/docker.ls")

    docker_ls = set()
    for u in urls:
        # only download images not present in preprepared vm disk
        resp = requests.get(u)

        # remove mirror prefixes as we might be downloading official images
        # from the AWS or DD mirror instead of dockerhub to avoid rate limits
        for line in resp.text.split('\n'):
            if not line.strip():
                continue

            docker_ls.add(
                line.removeprefix("public.ecr.aws/docker/library/")
                .removeprefix("registry.ddbuild.io/images/mirror/library/")
                .removeprefix("registry.ddbuild.io/images/mirror/")
            )

    images = _test_docker_image_list()
    for image in images - docker_ls:
        output_path = image.translate(str.maketrans('', '', string.punctuation))
        output_file = f"{os.path.join(output_dir, output_path)}.tar"
        if use_crane:
            ctx.run(f"crane pull --platform linux/{crane_arch} {image} {output_file}")
        else:
            ctx.run(f"docker pull --platform linux/{crane_arch} {image}")
            ctx.run(f"docker save {image} > {output_file}")


@task
def test_docker_image_list(_):
    images = _test_docker_image_list()
    print('\n'.join(images))


def _test_docker_image_list():
    import yaml

    docker_compose_paths = glob.glob("./pkg/network/protocols/**/*/docker-compose.yml", recursive=True)
    docker_compose_paths.extend(glob.glob("./pkg/network/usm/**/*/docker-compose.yml", recursive=True))
    docker_compose_paths.append("./pkg/network/protocols/tls/nodejs/testdata/docker-compose-ubuntu.yml")
    # Add relative docker-compose paths
    # For example:
    #   docker_compose_paths.append("./pkg/network/protocols/dockers/testdata/docker-compose.yml")

    images = set()
    for docker_compose_path in docker_compose_paths:
        with open(docker_compose_path) as f:
            docker_compose = yaml.safe_load(f.read())
        for component in docker_compose["services"]:
            images.add(docker_compose["services"][component]["image"])

    # Temporary: GoTLS monitoring inside containers tests are flaky in the CI, so at the meantime, the tests are
    # disabled, so we can skip downloading a redundant image.
    images.remove("public.ecr.aws/b1o7r7e0/usm-team/go-httpbin:https")

    # Add images used in docker run commands
    images.add("alpine:3.20.3")

    return images


@task
def save_build_outputs(ctx, destfile):
    if not destfile.endswith(".tar.xz"):
        raise Exit(message="destfile must be a .tar.xz file")

    absdest = os.path.abspath(destfile)
    count = 0
    outfiles = []
    with tempfile.TemporaryDirectory() as stagedir:
        arch = Arch.local()
        build_dir = get_ebpf_build_dir(arch)
        for subdir in ["", "co-re"]:
            src_dir = os.path.join(str(build_dir), subdir) if subdir else str(build_dir)
            for obj in glob.glob(os.path.join(src_dir, "*.o")):
                relpath = os.path.relpath(obj)
                filedir, _ = os.path.split(relpath)
                outdir = os.path.join(stagedir, filedir)
                os.makedirs(outdir, exist_ok=True)
                shutil.copy2(obj, outdir)
                outfiles.append(relpath)
                count += 1

        # Include inplace targets (e.g. uprobe-trigger.o, detect-seccomp-bug)
        # that live in their source directories rather than the central build dir.
        for target, dest_dir in _BAZEL_EBPF_INPLACE_TARGETS.items():
            name = target.rsplit(":", 1)[1]
            # eBPF targets produce .o files, native cc_binary targets produce bare binaries
            for candidate in [os.path.join(dest_dir, f"{name}.o"), os.path.join(dest_dir, name)]:
                if os.path.exists(candidate):
                    relpath = os.path.relpath(candidate)
                    filedir, _ = os.path.split(relpath)
                    outdir = os.path.join(stagedir, filedir)
                    os.makedirs(outdir, exist_ok=True)
                    shutil.copy2(candidate, outdir)
                    outfiles.append(relpath)
                    count += 1
                    break

        # Include runtime compilation flattened .c files (generated by Bazel,
        # consumed by omnibus packaging) in the tarball.
        runtime_dir = str(get_ebpf_runtime_dir())
        for cfile in glob.glob(os.path.join(runtime_dir, "*.c")):
            relpath = os.path.relpath(cfile)
            filedir, _ = os.path.split(relpath)
            outdir = os.path.join(stagedir, filedir)
            os.makedirs(outdir, exist_ok=True)
            shutil.copy2(cfile, outdir)
            outfiles.append(relpath)
            count += 1

        # Include runtime compilation Go integrity hash files (gitignored,
        # generated by Bazel) so the omnibus go build can find them.
        go_hash_dir = os.path.join("pkg", "ebpf", "bytecode", "runtime")
        for gofile in glob.glob(os.path.join(go_hash_dir, "*.go")):
            relpath = os.path.relpath(gofile)
            filedir, _ = os.path.split(relpath)
            outdir = os.path.join(stagedir, filedir)
            os.makedirs(outdir, exist_ok=True)
            shutil.copy2(gofile, outdir)
            outfiles.append(relpath)
            count += 1

        if count == 0:
            raise Exit(message="no build outputs captured")
        ctx.run(f"tar -C {stagedir} -cJf {absdest} .")

    outfiles.sort()
    for outfile in outfiles:
        ctx.run(f"sha256sum {outfile} >> {absdest}.sum")


def copy_ebpf_and_related_files(ctx: Context, target: Path | str, arch: Arch | None = None):
    if arch is None:
        arch = Arch.local()

    build_dir = get_ebpf_build_dir(arch)
    runtime_dir = get_ebpf_runtime_dir()
    ctx.run(f"cp {build_dir}/*.o {target}")
    ctx.run(f"mkdir {target}/co-re")
    ctx.run(f"cp {build_dir}/co-re/*.o {target}/co-re/")
    ctx.run(f"cp {runtime_dir}/*.c {target}")
    ctx.run(f"chmod 0444 {target}/*.o {target}/*.c {target}/co-re/*.o")
    ctx.run(f"cp /opt/datadog-agent/embedded/bin/clang-bpf {target}")
    ctx.run(f"cp /opt/datadog-agent/embedded/bin/llc-bpf {target}")


@task
def build_usm_debugger(
    ctx,
    arch: str = CURRENT_ARCH,
    strip_binary=False,
):
    build_object_files(ctx)

    build_dir = os.path.join("pkg", "ebpf", "bytecode", "build", arch)

    # copy compilation artifacts to the debugger root directory for the purposes of embedding
    usm_programs = [
        os.path.join(build_dir, "co-re", "usm-debug.o"),
        os.path.join(build_dir, "co-re", "shared-libraries-debug.o"),
    ]

    embedded_dir = os.path.join(".", "pkg", "network", "usm", "debugger", "cmd")

    for p in usm_programs:
        print(p)
        shutil.copy(p, embedded_dir)

    arch_obj = Arch.from_str(arch)
    ldflags, gcflags, env = get_build_flags(ctx, arch=arch_obj)
    if strip_binary:
        ldflags += ' -s -w'

    go_build(
        ctx,
        "./pkg/network/usm/debugger/cmd/usm-debugger",
        build_tags=["linux_bpf", "usm_debugger"],
        ldflags=ldflags,
        bin_path="bin/usm-debugger",
        env=env,
    )


@task
def build_gpu_event_viewer(ctx):
    build_dir = Path("pkg/gpu/testutil/event-viewer")

    tags = get_default_build_tags("system-probe")
    if "test" not in tags:
        tags.append("test")

    binary = build_dir / "event-viewer"
    main_file = build_dir / "main.go"

    go_build(ctx, main_file, build_tags=tags, bin_path=binary)
    print(f"Built {binary}")


@task
def collect_gpu_events(ctx, output_dir: str, pod_name: str, event_count: int = 1000, namespace: str | None = None):
    """
    Collect GPU events from a node for a given duration.

    Args:
        output_dir (str): The directory to save the collected events.
        duration (int): The duration of the collection in seconds.
        node (str): The node to collect events from.
        namespace (str | None): The namespace where the agent pod is running.
    """
    ns_arg = f"-n {namespace}" if namespace else ""
    ctx.run(
        f'kubectl {ns_arg} exec {pod_name} -c system-probe -- /bin/bash -c "curl --unix-socket \\$DD_SYSPROBE_SOCKET http://unix/gpu/debug/collect-events?count={event_count} > /tmp/gpu-events.ndjson"'
    )

    ctx.run(f"mkdir -p {output_dir}")
    ctx.run(f"kubectl {ns_arg} cp {pod_name}:/tmp/gpu-events.ndjson -c system-probe {output_dir}/gpu-events.ndjson")


@task
def build_dyninst_test_programs(ctx: Context, output_root: Path = ".", debug: bool = False):
    nf_path = os.path.join(output_root, "system-probe-dyninst-test-programs.ninja")
    with open(nf_path, "w") as nf:
        nw = NinjaWriter(nf)
        go_parallelism = compute_go_parallelism(debug, ci=False)
        nw.pool(name="gobuild", depth=go_parallelism)
        nw.rule(
            name="gobin",
            command="$chdir && $env $go build -o $out $extra_arguments $tags $ldflags $in $tool",
        )
        ninja_add_dyninst_test_programs(ctx, nw, output_root, "go")
    ctx.run(f"ninja -d explain -v -f {nf_path}")


def ninja_add_dyninst_test_programs(
    ctx: Context,
    nw: NinjaWriter,
    output_root: Path,
    go_path: str,
):
    """
    This function is used to add the dyninst test programs to the ninja file.

    It is used to build the test programs for the dyninst test suite across
    the relevant architectures and go versions.
    """

    dd_module = "github.com/DataDog/datadog-agent"
    testprogs_path = "pkg/dyninst/testprogs"
    progs_path = f"{testprogs_path}/progs"
    progs_prefix = f"{dd_module}/{progs_path}/"
    output_base = f"{output_root}/{testprogs_path}/binaries"
    build_tags = ["test", "linux_bpf"]

    # Find the dependencies of the test programs.
    tags_flag = f"-tags \"{','.join(build_tags)}\""
    list_format = "{{ .ImportPath }} {{ .Name }}: {{ join .Deps \" \" }}"
    # Run from within the progs directory so that the go list command can find
    # the go.mod file.
    with ctx.cd(progs_path):
        # Disable buildvcs to avoid attempting to invoke git, which can be
        # slow in VMs and we don't need it for our tests.
        list_cmd = f"go list -buildvcs=false -mod=readonly -test -f '{list_format}' {tags_flag} ./..."
        # Disable GOWORK because our testprogs go.mod isn't listed there.
        env = {"GOWORK": "off"}
        res = ctx.run(list_cmd, hide=True, env=env)
    if res.return_code != 0:
        raise Exit(message=f"Failed to list dependencies: {res.stderr}")
    pkg_deps = {}
    for line in res.stdout.splitlines():
        pkg_main, deps = line.split(": ", 1)
        pkg, name = pkg_main.split(" ", 1)
        pkg = pkg.removeprefix(progs_prefix)
        if name == "main":
            deps = (d for d in deps.split(" ") if d.startswith(progs_prefix))
            pkg_deps[pkg] = {d.removeprefix(progs_prefix) for d in deps}

    go_versions = ["go1.23.11", "go1.24.3", "go1.25.0", "go1.26rc1"]
    archs = ["amd64", "arm64"]

    # Avoiding cgo aids in reproducing the build environment. It's less good in
    # some ways because it's not likely that other folks build without CGO.
    # Eventually we're going to want a better story for how to test against a
    # variety of go binaries.
    outputs = set()
    for pkg, go_version, arch in itertools.product(
        pkg_deps.keys(),
        go_versions,
        archs,
    ):
        direct = glob.glob(f"{progs_path}/{pkg}/*.go")
        go_files = set(direct)
        go_files.add(f"{progs_path}/go.mod")
        for dep in pkg_deps[pkg]:
            dep_files = glob.glob(f"{progs_path}/{dep}/*.go")
            dep_files = [p for p in dep_files if not p.endswith("test.go")]
            go_files.update(os.path.abspath(p) for p in dep_files)
        config_str = f"arch={arch},toolchain={go_version}"
        output_path = f"{output_base}/{config_str}/{pkg}"
        output_path = os.path.abspath(output_path)
        outputs.add(output_path)
        pkg_path = os.path.abspath(f"./{progs_path}/{pkg}")
        nw.build(
            inputs=[pkg_path],
            outputs=[output_path],
            implicit=list(go_files),
            rule="gobin",
            pool="gobuild",
            variables={
                "go": go_path,
                "extra_arguments": " ".join(
                    [
                        # Trimpath is used so that the binaries have predictable
                        # source paths.
                        "-trimpath",
                        # Disable buildvcs to avoid attempting to invoke git, which can
                        # be slow in VMs and we don't need it for our tests.
                        "-buildvcs=false",
                    ]
                ),
                # Run from within the package directory so that the go build
                # command finds the go.mod file.
                "chdir": f"cd {pkg_path}",
                "tags": tags_flag,
                "ldflags": "-ldflags=\"-extldflags '-static'\"",
                "env": " ".join(
                    [
                        "CGO_ENABLED=0",
                        f"GOARCH={arch}",
                        "GOOS=linux",
                        f"GOTOOLCHAIN={go_version}",
                        "GOWORK=off",
                    ]
                ),
            },
        )

    # Remove any previously built binaries that are no longer needed.
    for path in glob.glob(f"{output_base}/*/*"):
        path = os.path.abspath(path)
        if os.path.isfile(path) and path not in outputs:
            os.remove(path)
