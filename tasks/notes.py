import re
import shlex
import sys
from datetime import date
from glob import glob

from invoke import Failure, task
from invoke.exceptions import Exit

from tasks.libs.ciproviders.github_api import create_release_pr
from tasks.libs.common.color import color_message
from tasks.libs.common.git import get_current_branch, try_git_command
from tasks.libs.common.worktree import agent_context
from tasks.libs.releasing.notes import (
    CHANGELOG_SECTIONS,
    _add_dca_prelude,
    _add_prelude,
    _new_fragment_path,
    update_changelog_generic,
)
from tasks.libs.releasing.version import deduce_version


@task
def new(ctx, slug, dca=False, installscript=False):
    """Create a new release note fragment.

    Creates a YAML template in releasenotes/notes/ (or releasenotes-dca/notes/
    if --dca is set, or releasenotes-installscript/notes/ if --installscript is set).
    Fill in the relevant sections and delete the rest.

    Example:
        dda inv notes.new my-feature
        dda inv notes.new my-fix --dca
    """
    if dca:
        changelog_dir = 'releasenotes-dca'
    elif installscript:
        changelog_dir = 'releasenotes-installscript'
    else:
        changelog_dir = 'releasenotes'

    note_path = _new_fragment_path(changelog_dir, slug)
    note_path.parent.mkdir(parents=True, exist_ok=True)

    section_lines = []
    for key, title in CHANGELOG_SECTIONS:
        if key == 'upgrade':
            section_lines.append(
                f"# {key}:\n#   - |\n#     {title}: include steps users can follow if affected.\n"
            )
        else:
            section_lines.append(f"# {key}:\n#   - |\n#     {title}.\n")

    template = "---\n# Fill in the relevant section(s) below. Delete sections that don't apply.\n# Content should be written in Markdown.\n\n"
    template += "".join(section_lines)

    with open(note_path, 'w', encoding='utf-8') as f:
        f.write(template)

    print(f"Created release note: {note_path}")
    print("Edit the file, fill in the relevant sections, and commit it with your PR.")


@task
def add_prelude(ctx, release_branch):
    version = deduce_version(ctx, release_branch, next_version=False)

    with agent_context(ctx, release_branch):
        _add_prelude(ctx, version)


@task
def add_dca_prelude(ctx, release_branch):
    """
    Release of the Cluster Agent should be pinned to a version of the Agent.
    """
    version = deduce_version(ctx, release_branch, next_version=False)

    with agent_context(ctx, release_branch):
        _add_dca_prelude(ctx, version)


@task
def add_installscript_prelude(ctx, release_branch):
    version = deduce_version(ctx, release_branch, next_version=False)

    with agent_context(ctx, release_branch):
        note_path = _new_fragment_path('releasenotes-installscript', f'prelude-release-{version}')
        note_path.parent.mkdir(parents=True, exist_ok=True)

        with open(note_path, 'w', encoding='utf-8') as f:
            f.write(
                f"""prelude: |
  Released on: {date.today()}
"""
            )

        ctx.run(f"git add {shlex.quote(str(note_path))}")
        print("\nCommit this with:")
        print(f"git commit -m \"Add prelude for {version} release\"")


@task
def update_changelog(ctx, release_branch, target="all", upstream="origin"):
    """
    Generate the new CHANGELOG.md when releasing a minor version (linux/macOS only).
    By default generates Agent and Cluster Agent changelogs.
    Use target == "agent" or target == "cluster-agent" to only generate one or the other.
    """

    with agent_context(ctx, release_branch):
        new_version = deduce_version(ctx, release_branch, next_version=False)
        new_version_int = list(map(int, new_version.split(".")))
        if len(new_version_int) != 3:
            print(f"Error: invalid version: {new_version_int}")
            raise Exit(code=1)

        # Step 1 - generate the changelogs

        generate_agent = target in ["all", "agent"]
        generate_cluster_agent = target in ["all", "cluster-agent"]

        # let's avoid losing uncommitted change with 'git reset --hard'
        try:
            ctx.run("git diff --exit-code HEAD", hide="both")
        except Failure:
            print("Error: You have uncommitted changes, please commit or stash before using update_changelog")
            return

        if generate_agent:
            update_changelog_generic(ctx, new_version, "releasenotes", "CHANGELOG.md")
        if generate_cluster_agent:
            update_changelog_generic(ctx, new_version, "releasenotes-dca", "CHANGELOG-DCA.md")

        # Step 2 - commit changes

        update_branch = f"changelog-update-{new_version}"
        base_branch = get_current_branch(ctx)

        print(color_message(f"Branching out to {update_branch}", "bold"))
        ctx.run(f"git checkout -b {update_branch}")

        print(color_message("Committing CHANGELOG.md and CHANGELOG-DCA.md", "bold"))
        print(
            color_message(
                "If commit signing is enabled, you will have to make sure the commit gets properly signed.", "bold"
            )
        )

        commit_message = f"'Changelog updates for {new_version} release'"

        ok = try_git_command(ctx, f"git commit -m {commit_message}")
        if not ok:
            raise Exit(
                color_message(
                    f"Could not create commit. Please commit manually with:\ngit commit -m {commit_message}\n, push the {update_branch} branch and then open a PR.",
                    "red",
                ),
                code=1,
            )

        # Step 3 - Push and create PR

        print(color_message("Pushing new branch to the upstream repository", "bold"))
        res = ctx.run(f"git push --set-upstream {upstream} {update_branch}", warn=True)
        if res.exited is None or res.exited > 0:
            raise Exit(
                color_message(
                    f"Could not push branch {update_branch} to the upstream '{upstream}'. Please push it manually and then open a PR.",
                    "red",
                ),
                code=1,
            )

        create_release_pr(
            f"Changelog update for {new_version} release",
            base_branch,
            update_branch,
            new_version,
            changelog_pr=True,
            milestone=str(new_version),
        )


@task
def update_installscript_changelog(ctx, release_branch):
    """
    Generate the new CHANGELOG-INSTALLSCRIPT.md when releasing a minor version.
    """

    new_version = deduce_version(ctx, release_branch, next_version=False)

    with agent_context(ctx, release_branch):
        new_version_int = list(map(int, new_version.split(".")))

        if len(new_version_int) != 3:
            print(f"Error: invalid version: {new_version_int}")
            raise Exit(code=1)

        # let's avoid losing uncommitted change with 'git reset --hard'
        try:
            ctx.run("git diff --exit-code HEAD", hide="both")
        except Failure:
            print(
                "Error: You have uncommitted changes, please commit or stash before using update-installscript-changelog"
            )
            return

        # make sure we are up to date
        ctx.run("git fetch")

        update_changelog_generic(ctx, new_version, "releasenotes-installscript", "CHANGELOG-INSTALLSCRIPT.md")

        print("\nCommit this with:")
        print(f"git commit -m \"[INSTALLSCRIPT] Update CHANGELOG-INSTALLSCRIPT for {new_version}\"")


# Matches ``code`` (RST inline code) — captured group is the inner text.
_RST_CODE_RE = re.compile(r'``([^`]+)``')

# Matches `text <url>`_ (RST inline hyperlink).
_RST_LINK_RE = re.compile(r'`([^`<]+?)\s+<([^>]+)>`_')

# Matches any remaining `something`_ (standalone RST reference we can't auto-convert).
_RST_REF_RE = re.compile(r'`[^`]+`_')


def _convert_rst_to_md(text: str) -> str:
    text = _RST_LINK_RE.sub(lambda m: f'[{m.group(1).strip()}]({m.group(2)})', text)
    text = _RST_CODE_RE.sub(lambda m: f'`{m.group(1)}`', text)
    return text


@task
def migrate_rst(ctx, dry_run=False):
    """Convert RST formatting in fragment YAML files to Markdown (one-time migration).

    Handles the two constructs found in reno-era fragments:
      ``code``      -> `code`
      `text <url>`_ -> [text](url)

    Any remaining `ref`_ patterns that can't be auto-converted are reported
    for manual review. Run with --dry-run to preview without writing changes.
    """
    import yaml

    dirs = [
        'releasenotes/notes',
        'releasenotes-dca/notes',
        'releasenotes-installscript/notes',
    ]

    fragment_files = []
    for d in dirs:
        fragment_files.extend(glob(f'{d}/*.yaml'))

    modified = 0
    flagged = []

    for file_path in sorted(fragment_files):
        with open(file_path, encoding='utf-8') as f:
            raw = f.read()

        try:
            content = yaml.safe_load(raw)
        except yaml.YAMLError:
            print(color_message(f"SKIP (invalid YAML): {file_path}", "yellow"))
            continue

        if not isinstance(content, dict):
            continue

        new_raw = _convert_rst_to_md(raw)

        remaining = _RST_REF_RE.findall(new_raw)
        if remaining:
            flagged.append((file_path, remaining))

        if new_raw != raw:
            modified += 1
            if dry_run:
                print(color_message(f"Would modify: {file_path}", "yellow"))
            else:
                with open(file_path, 'w', encoding='utf-8') as f:
                    f.write(new_raw)

    action = "Would modify" if dry_run else "Modified"
    print(color_message(f"\n{action} {modified}/{len(fragment_files)} fragment files.", "green"))

    if flagged:
        print(color_message("\nThe following files contain `ref`_ patterns that need manual review:", "yellow"))
        for path, refs in flagged:
            print(f"  {path}: {refs}")
