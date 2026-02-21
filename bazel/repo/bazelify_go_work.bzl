"""Generate "bazelified" go.work.

The migration from Go toolchain to Gazelle is incremental: modules not yet converted are listed as `# gazelle:exclude`
in BUILD.bazel so that Gazelle ignores them. However, rules_go is unaware of those exclusions and would pull all go.work
entries into go_deps on every `bazel mod tidy`, causing unwanted churn for unconverted modules.

This repository rule bridges the gap by producing a filtered copy of go.work that only retains `use` entries for
already-converted modules (i.e. those NOT covered by a `# gazelle:exclude` directive, with `.` always excluded). The
result is checked in as @bazelify_go_work//:go.work and used as the go_deps.from_file source.

This file and its generated output are temporary and will be removed once all modules have been migrated to Gazelle.
"""

load("@re.bzl", "re")
load("//bazel/repo:parse_go_work.bzl", "parse_go_work")

def _impl(rctx):
    excludes = set(["."]) | set([
        m.group(1)
        for line in rctx.read(rctx.attr.build_file).splitlines()
        for m in [re.search(r"# gazelle:exclude (\S+)", line)]
        if m
    ])

    go_work = parse_go_work(rctx, lambda p: p in excludes or any([p.startswith(e + "/") for e in excludes]))
    rctx.file("BUILD.bazel", 'exports_files(["go.work"])\n')
    rctx.file("go.work", "\n".join(go_work.lines + [""]))
    for symlink in set([p.partition("/")[0] for p in go_work.paths]):
        rctx.symlink(rctx.path(rctx.attr.go_work).dirname.get_child(symlink), symlink)

bazelify_go_work = repository_rule(
    attrs = {
        "build_file": attr.label(default = "//:BUILD.bazel", allow_single_file = True),
        "go_work": attr.label(default = "//:go.work", allow_single_file = True),
    },
    implementation = _impl,
    local = True,
)
