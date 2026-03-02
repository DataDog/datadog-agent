"""Generate "bazelified" go.work.

The migration from Go toolchain to Gazelle is incremental: modules not yet converted are listed as `# gazelle:exclude`
in BUILD.bazel so that Gazelle ignores them. However, rules_go is unaware of those exclusions and would pull all go.work
entries into go_deps on every `bazel mod tidy`, causing unwanted churn for unconverted modules.

This repository rule bridges the gap by producing a filtered copy of go.work that only retains `use` entries for
already-converted modules (i.e. those NOT covered by a `# gazelle:exclude` directive, with `.` always excluded). The
result is checked in as @bazelify_go_work//:go.work and used as the go_deps.from_file source.

Modules without a BUILD file are automatically excluded with a warning, as gazelle cannot resolve their go.mod.

This file and its generated output are temporary and will be removed once all modules have been migrated to Gazelle.
"""

load("@re.bzl", "re")

def _filter_lines(rctx):
    """Filter the lines of the go.work file to only include the modules that are used in the build file."""
    workspace = rctx.path(rctx.attr.go_work).dirname
    exclusions = set([m.group(1) for line in rctx.read(rctx.attr.build_file).splitlines() for m in [re.search(r"# gazelle:exclude (\S+)", line)] if m])

    def _is_excluded(path):
        if path in exclusions or any([path.startswith(exclusion + "/") for exclusion in exclusions]):
            return True
        if path != ".":
            mod_dir = workspace.get_child(path)
            if not mod_dir.get_child("BUILD.bazel").exists and not mod_dir.get_child("BUILD").exists:
                # buildifier: disable=print
                print("WARNING: Module '{}' has no BUILD.bazel file, it won't be passed to gazelle. Add a BUILD.bazel file or a gazelle:exclude directive.".format(path))
                return True
        return False

    in_use_block, lines, symlinks = 0, [], set()
    for line in rctx.read(rctx.attr.go_work).splitlines():
        stripped = line.strip()
        if stripped == "use (":
            in_use_block += 1
        elif stripped == ")":
            in_use_block -= 1
        elif in_use_block and stripped and not stripped.startswith("//"):
            if _is_excluded(stripped):
                continue
            symlinks |= set(["go.mod", "go.sum"]) if stripped == "." else set([stripped.partition("/")[0]])
        lines.append(line)
    return lines, symlinks

def _impl(rctx):
    """See module docstring."""
    lines, symlinks = _filter_lines(rctx)
    rctx.file("BUILD.bazel", 'exports_files(["go.work"])\n')
    rctx.file("go.work", "\n".join(lines + [""]))

    # prevent `bazel` from following symlinks when evaluating recursive target patterns in the repo directory
    rctx.file("DONT_FOLLOW_SYMLINKS_WHEN_TRAVERSING_THIS_DIRECTORY_VIA_A_RECURSIVE_TARGET_PATTERN")
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
