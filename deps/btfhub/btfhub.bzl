"""Lists a pinned BTFHub archive commit as fixed buckets of kernel BTFs to minimize."""

_ARCHES = ["x86_64", "arm64"]

_BUILD = """\
package(default_visibility = ["//visibility:public"])

exports_files(["remote.txt"])
""" + "".join([
    'filegroup(name = "{0}", srcs = glob(["{0}/*.txt"]))\n'.format(arch)
    for arch in _ARCHES
])

def _git(rctx, *args):
    res = rctx.execute(["git", "--git-dir=archive.git"] + list(args))
    if res.return_code:
        fail("git {} failed: {}".format(args[0], res.stderr))
    return res.stdout

def _btfhub_archive_impl(rctx):
    rctx.execute(["git", "init", "-q", "--bare", "archive.git"])

    # Trees only: kernel BTFs are fetched by blob id in the minimization actions.
    _git(rctx, "-c", "fetch.negotiationAlgorithm=noop", "fetch", "-q", "--depth=1", "--filter=blob:none", "--no-tags", rctx.attr.remote, rctx.attr.commit)
    tree = _git(rctx, "ls-tree", "-r", "--full-tree", rctx.attr.commit)
    rctx.delete("archive.git")

    buckets = {arch: [[] for _ in range(rctx.attr.buckets)] for arch in _ARCHES}
    seen = {}
    for line in tree.splitlines():
        meta, path = line.split("\t", 1)
        mode, _, blob = meta.split(" ")
        parts = path.split("/")

        # Symlinked release directories (mode 120000) alias real ones and are skipped.
        if mode != "100644" or len(parts) != 4 or parts[2] not in buckets or not path.endswith(".btf.tar.xz"):
            continue
        distro, release, arch, name = parts

        # Ubuntu ships the same kernel names on several releases, see pkg/ebpf/btf.go.
        out = "/".join([distro, release, name] if distro == "ubuntu" else [distro, name])
        out = out.removesuffix(".tar.xz")
        if (arch, out) in seen:
            fail("{} and {} both minimize to {}".format(seen[(arch, out)], path, out))
        seen[(arch, out)] = path
        buckets[arch][hash(out) % rctx.attr.buckets].append("{} {}\n".format(blob, out))

    for arch, lists in buckets.items():
        for i, entries in enumerate(lists):
            rctx.file("{}/{}.txt".format(arch, i), "".join(sorted(entries)))
    rctx.file("remote.txt", rctx.attr.remote + "\n")
    rctx.file("BUILD.bazel", _BUILD)

btfhub_archive = repository_rule(
    implementation = _btfhub_archive_impl,
    doc = """Lists the kernel BTFs of a BTFHub archive commit without downloading them.

    Each arch gets `buckets` manifests of `<git blob id> <output path>` lines,
    assigned by a hash of the output path. A bump of the commit only changes
    the buckets whose kernels changed, so only those are minimized again.
    """,
    attrs = {
        "buckets": attr.int(default = 64),
        "commit": attr.string(mandatory = True),
        "remote": attr.string(mandatory = True),
    },
)
