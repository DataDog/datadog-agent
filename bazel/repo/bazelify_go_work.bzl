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

def _filter_lines(build_file, go_work):
    exclusions = set(["."]) | set([m.group(1) for line in build_file for m in [re.search(r"# gazelle:exclude (\S+)", line)] if m])

    def _is_excluded(path):
        return path in exclusions or any([path.startswith(exclusion + "/") for exclusion in exclusions])

    in_use_block, lines, symlinks = 0, [], set()
    for line in go_work:
        stripped = line.strip()
        if stripped == "use (":
            in_use_block += 1
        elif stripped == ")":
            in_use_block -= 1
        elif in_use_block and stripped and not stripped.startswith("//"):
            if _is_excluded(stripped):
                continue
            symlinks.add(stripped.partition("/")[0])
        lines.append(line)
    return lines, symlinks

def _impl(rctx):
    """See module docstring."""
    lines, symlinks = _filter_lines(rctx.read(rctx.attr.build_file).splitlines(), rctx.read(rctx.attr.go_work).splitlines())
    rctx.file("BUILD.bazel", 'exports_files(["go.work"])\n')
    rctx.file("go.work", "\n".join(lines + [""]))
    for symlink in symlinks:
        rctx.symlink(rctx.path(rctx.attr.go_work).dirname.get_child(symlink), symlink)

bazelify_go_work = repository_rule(
    attrs = {
        "build_file": attr.label(default = "//:BUILD.bazel", allow_single_file = True),
        "go_work": attr.label(default = "//:go.work", allow_single_file = True),
    },
    implementation = _impl,
    local = True,
)
