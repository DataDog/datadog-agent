"""Repository rule to discover host kernel headers for eBPF compilation."""

NAME = "linux_headers"

_ARCH_MAP = {
    "x86_64": "x86",
    "amd64": "x86",
    "aarch64": "arm64",
    "arm64": "arm64",
}

_ARCH_SPELLINGS = {
    "x86": ["amd64", "x86_64", "x86-64"],
    "arm64": ["arm64", "aarch64"],
}

_SUBDIRS = [
    "include",
    "include/uapi",
    "include/generated/uapi",
    # arch-specific subdirs are appended dynamically
]

def _parse_kernel_version(name):
    """Extract (major, minor, patch) from a kernel header directory name.

    Returns None if no version can be parsed.
    """
    clean = name
    for prefix in ["linux-headers-", "linux-kbuild-"]:
        if clean.startswith(prefix):
            clean = clean[len(prefix):]
            break

    parts = clean.split(".")
    if len(parts) < 2:
        return None

    major = parts[0].lstrip("0") or "0"
    minor_str = parts[1]

    # minor might contain a dash (e.g. "10" from "5.10.0-0.deb10.30-amd64")
    minor = minor_str.split("-")[0]

    patch = "0"
    if len(parts) >= 3:
        patch_str = parts[2].split("-")[0]
        if patch_str:
            patch = patch_str

    if not major.isdigit() or not minor.isdigit() or not patch.isdigit():
        return None

    return (int(major), int(minor), int(patch))

def _matches_arch(name, kernel_arch):
    """Check if a directory name matches the given architecture."""
    spellings = _ARCH_SPELLINGS.get(kernel_arch, [])
    for s in spellings:
        if s in name:
            return True
    return False

def _has_any_arch(name):
    """Check if a directory name matches any known architecture."""
    for spellings in _ARCH_SPELLINGS.values():
        for s in spellings:
            if s in name:
                return True
    return False

def _list_dirs(rctx, path):
    """List directories under the given path. Returns list of (name, full_path)."""
    result = rctx.execute(["ls", "-1", path], timeout = 5)
    if result.return_code != 0:
        return []
    entries = []
    for name in result.stdout.strip().splitlines():
        if not name:
            continue
        full = path + "/" + name
        entries.append((name, full))
    return entries

def _resolve_path(rctx, path):
    """Resolve symlinks."""
    result = rctx.execute(["readlink", "-f", path], timeout = 5)
    if result.return_code != 0:
        return path
    resolved = result.stdout.strip()
    return resolved if resolved else path

def _is_dir(rctx, path):
    result = rctx.execute(["test", "-d", path], timeout = 5)
    return result.return_code == 0

def _version_at_least(version, minimum):
    """Return True if version >= minimum. Both are (major, minor, patch) tuples."""
    return version >= minimum

def _get_kernel_release(rctx):
    """Return the running kernel's release string via uname -r."""
    result = rctx.execute(["uname", "-r"], timeout = 5)
    if result.return_code != 0:
        return None
    return result.stdout.strip()

def _discover_header_dirs(rctx, kernel_arch, min_kernel_version = None, kernel_release_version = None):
    """Discover kernel header directories, mirroring get_linux_header_dirs() from tasks/system_probe.py.

    Args:
        rctx: repository context
        kernel_arch: target kernel architecture (e.g. "x86", "arm64")
        min_kernel_version: optional (major, minor, patch) tuple; headers older than this are discarded
        kernel_release_version: optional (major, minor, patch) tuple of the running kernel;
            candidates matching this version are prioritized
    """

    candidates = {}  # resolved_path -> name

    # /usr/src/kernels/*
    for name, full in _list_dirs(rctx, "/usr/src/kernels"):
        if _is_dir(rctx, full):
            resolved = _resolve_path(rctx, full)
            candidates[resolved] = name

    # /usr/src/linux-*
    for name, full in _list_dirs(rctx, "/usr/src"):
        if name.startswith("linux-") and _is_dir(rctx, full):
            resolved = _resolve_path(rctx, full)
            candidates[resolved] = name

    # /lib/modules/*/build and /lib/modules/*/source
    for _, mod_full in _list_dirs(rctx, "/lib/modules"):
        for sub in ["build", "source"]:
            p = mod_full + "/" + sub
            if _is_dir(rctx, p):
                resolved = _resolve_path(rctx, p)

                candidates[resolved] = resolved.rsplit("/", 1)[-1]

    # Score and filter
    # Debian/Ubuntu split header packages. So one can install the headers for a different architecture
    # for example, to do cross-compilation. We need to exclude the headers for the wrong architecture.
    # Therefore, prioritization is done based on the architecture match and then the version match.
    # TODO{agent-build}: kernel headers should be a hermetic package instead of a host lookup.
    scored = []  # list of (priority, sort_order, resolved_path)
    rejected_by_version = []  # names rejected by min_kernel_version filter
    for resolved, name in candidates.items():
        version = _parse_kernel_version(name)
        if version == None:
            continue

        if min_kernel_version and not _version_at_least(version, min_kernel_version):
            rejected_by_version.append("{} ({}.{}.{})".format(name, version[0], version[1], version[2]))
            continue

        priority = 0
        sort_order = 100

        if kernel_release_version and version == kernel_release_version:
            priority += 1

        if _matches_arch(name, kernel_arch):
            sort_order = 0
            priority += 1
        elif not _has_any_arch(name):
            sort_order = 1
            priority += 1

        scored.append((priority, sort_order, resolved, name))

    if not scored:
        msg = "No kernel header directories found. Searched /usr/src/kernels, /usr/src/linux-*, /lib/modules/*/build|source."
        if rejected_by_version:
            msg += " Rejected (below min_kernel_version {}.{}.{}): {}".format(
                min_kernel_version[0],
                min_kernel_version[1],
                min_kernel_version[2],
                ", ".join(sorted(rejected_by_version)),
            )
        fail(msg)

    max_priority = max([s[0] for s in scored])
    best = [(s[2], s[1], s[3]) for s in scored if s[0] == max_priority]

    # Sort by (sort_order, path) for determinism
    best_sorted = sorted(best, key = lambda x: (x[1], x[0]))
    return [path for path, _, _ in best_sorted]

def _linux_headers_impl(rctx):
    os_name = rctx.os.name
    if "linux" not in os_name:
        rctx.file("BUILD.bazel", """\
# Stub linux_headers for non-Linux platforms.
# eBPF targets use target_compatible_with = ["@platforms//os:linux"].
""")
        rctx.file("defs.bzl", 'KERNEL_HEADER_DIRS = []\nKERNEL_ARCH = ""\n')
        return

    arch = rctx.os.arch
    kernel_arch = _ARCH_MAP.get(arch)
    if not kernel_arch:
        fail("Unsupported architecture for kernel headers: " + arch)

    min_ver = None
    if rctx.attr.min_kernel_version:
        min_ver = _parse_kernel_version(rctx.attr.min_kernel_version)
        if min_ver == None:
            fail("Invalid min_kernel_version: " + rctx.attr.min_kernel_version)

    kernel_release = _get_kernel_release(rctx)
    kernel_release_ver = _parse_kernel_version(kernel_release) if kernel_release else None

    header_roots = _discover_header_dirs(
        rctx,
        kernel_arch,
        min_kernel_version = min_ver,
        kernel_release_version = kernel_release_ver,
    )

    arch_subdirs = [
        "arch/{}/include".format(kernel_arch),
        "arch/{}/include/uapi".format(kernel_arch),
        "arch/{}/include/generated".format(kernel_arch),
        "arch/{}/include/generated/uapi".format(kernel_arch),
    ]
    all_subdirs = _SUBDIRS + arch_subdirs

    # Create symlinks for each header root
    includes = []
    for i, root in enumerate(header_roots):
        link_name = "kernel_{}".format(i)
        rctx.symlink(root, link_name)
        for subdir in all_subdirs:
            full = link_name + "/" + subdir
            if _is_dir(rctx, root + "/" + subdir):
                includes.append(full)

    include_paths_str = "\n".join(['    "{}",'.format(inc) for inc in includes])

    rctx.file("BUILD.bazel", """\
# Generated by @@//bazel/toolchains/linux_headers:linux_headers.bzl
load("@bazel_skylib//rules:common_settings.bzl", "string_list_flag")

filegroup(
    name = "all",
    srcs = glob(["kernel_*/**"]),
    visibility = ["//visibility:public"],
)

# Expose header include directories for the ebpf_prog rule.
# These are the -isystem paths needed for prebuilt eBPF compilation.
KERNEL_HEADER_INCLUDES = [
{include_paths}
]
""".format(include_paths = include_paths_str))

    rctx.file("defs.bzl", """\
# Generated: kernel header include directories for eBPF compilation.
KERNEL_HEADER_DIRS = [
{include_paths}
]
KERNEL_ARCH = "{kernel_arch}"
""".format(include_paths = include_paths_str, kernel_arch = kernel_arch))

linux_headers_repo = repository_rule(
    implementation = _linux_headers_impl,
    doc = "Discover host kernel headers for eBPF prebuilt compilation.",
    local = True,
    attrs = {
        "min_kernel_version": attr.string(
            default = "5.8.0",
            doc = "Minimum kernel version for header discovery (e.g. '5.8.0'). " +
                  "Headers older than this are discarded. Set to empty string to disable.",
        ),
    },
)

linux_headers_extension = module_extension(
    implementation = lambda ctx: linux_headers_repo(name = NAME),
)
