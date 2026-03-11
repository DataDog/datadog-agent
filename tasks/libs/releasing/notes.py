from __future__ import annotations

import secrets
import sys
from datetime import date
from pathlib import Path

import yaml
from invoke import Failure

from tasks.libs.common.constants import DEFAULT_INTEGRATIONS_CORE_BRANCH, GITHUB_REPO_NAME
from tasks.libs.common.git import get_default_branch
from tasks.libs.releasing.version import current_version

# Section ordering and display names for the Markdown changelog assembler.
# This is the canonical list of sections. tasks/libs/linter/releasenotes_md.py
# derives its CHANGELOG_SECTIONS frozenset from this list (plus 'prelude').
CHANGELOG_SECTIONS = [
    ('upgrade', 'Upgrade Notes'),
    ('features', 'New Features'),
    ('enhancements', 'Enhancement Notes'),
    ('issues', 'Issues'),
    ('known_issues', 'Known Issues'),
    ('deprecations', 'Deprecation Notes'),
    ('security', 'Security Notes'),
    ('fixes', 'Bug Fixes'),
    ('critical', 'Critical Notes'),
    ('other', 'Other Notes'),
]


def _new_fragment_path(changelog_dir: str, slug: str) -> Path:
    """Return a new fragment file path with a unique 16-char hex suffix."""
    uid = secrets.token_hex(8)
    slug = slug.replace('/', '-')
    return Path(changelog_dir) / 'notes' / f'{slug}-{uid}.yaml'


def _assemble_changelog(fragment_dir: str | Path, version: str) -> str:
    """Collect all fragment YAML files and render them into a Markdown section.

    This is the new pure-Python assembler. During the migration period it runs
    alongside reno (which remains authoritative). Fragment content is passed
    through verbatim — RST or Markdown — so the structural comparison holds
    regardless of content format.
    """
    notes_path = Path(fragment_dir) / 'notes'
    fragment_files = sorted(notes_path.glob('*.yaml'))

    sections = {key: [] for key, _ in CHANGELOG_SECTIONS}
    prelude = None

    for fragment_file in fragment_files:
        try:
            with open(fragment_file, encoding='utf-8') as f:
                content = yaml.safe_load(f)
        except (yaml.YAMLError, OSError) as e:
            raise RuntimeError(f"Failed to read fragment {fragment_file}: {e}") from e

        if not content or not isinstance(content, dict):
            continue

        if 'prelude' in content and isinstance(content['prelude'], str):
            if prelude is not None:
                print(
                    f"WARNING: multiple fragments contain 'prelude'; last one wins: {fragment_file}", file=sys.stderr
                )
            prelude = content['prelude'].strip()

        for section_key, _ in CHANGELOG_SECTIONS:
            if section_key in content:
                items = content[section_key]
                if isinstance(items, list):
                    for item in items:
                        if isinstance(item, str):
                            if item.strip():
                                sections[section_key].append(item.strip())
                        elif item is not None:
                            print(
                                f"WARNING: non-string item in '{section_key}' of {fragment_file}: {item!r}",
                                file=sys.stderr,
                            )

    lines = [f'## {version}', '']

    if prelude:
        lines.append(prelude)
        lines.append('')

    for section_key, section_title in CHANGELOG_SECTIONS:
        if sections[section_key]:
            lines.append(f'### {section_title}')
            lines.append('')
            for item in sections[section_key]:
                item_lines = item.split('\n')
                lines.append(f'- {item_lines[0]}')
                for extra in item_lines[1:]:
                    lines.append(f'  {extra}' if extra.strip() else '')
            lines.append('')

    return '\n'.join(lines) + '\n'


def _add_prelude(ctx, version, release_date=None):
    res = ctx.run(f"reno new prelude-release-{version}")
    new_releasenote = res.stdout.split(' ')[-1].strip()  # get the new releasenote file path
    branch = DEFAULT_INTEGRATIONS_CORE_BRANCH

    with open(new_releasenote, "w") as f:
        f.write(
            f"""prelude:
    |
    Release on: {release_date or date.today()}

    - Please refer to the `{version} tag on integrations-core <https://github.com/DataDog/integrations-core/blob/{branch}/AGENT_CHANGELOG.md#datadog-agent-version-{version.replace('.', '')}>`_ for the list of changes on the Core Checks
"""
        )

    ctx.run(f"git add {new_releasenote}")
    print("\nIf not run as part of finish task, commit this with:")
    print(f"git commit -m \"Add prelude for {version} release\"")


def _add_dca_prelude(ctx, version=None, release_date=None):
    """Release of the Cluster Agent should be pinned to a version of the Agent."""

    branch = get_default_branch()

    res = ctx.run(f"reno --rel-notes-dir releasenotes-dca new prelude-release-{version}")
    new_releasenote = res.stdout.split(' ')[-1].strip()  # get the new releasenote file path

    with open(new_releasenote, "w") as f:
        f.write(
            f"""prelude:
    |
    Released on: {release_date or date.today()}
    Pinned to datadog-agent v{version}: `CHANGELOG <https://github.com/{GITHUB_REPO_NAME}/blob/{branch}/CHANGELOG.rst#{version.replace('.', '')}>`_."""
        )

    ctx.run(f"git add {new_releasenote}")
    print("\nIf not run as part of finish task, commit this with:")
    print(f"git commit -m \"Add prelude for {version} release\"")


def update_changelog_generic(ctx, new_version, changelog_dir, changelog_file):
    if new_version is None:
        latest_version = current_version(ctx, 7)
        ctx.run(f"reno -q --rel-notes-dir {changelog_dir} report --ignore-cache --earliest-version {latest_version}")
        return
    new_version_int = list(map(int, new_version.split(".")))

    # removing releasenotes from bugfix on the old minor.
    branching_point = f"{new_version_int[0]}.{new_version_int[1]}.0-devel"
    previous_minor = f"{new_version_int[0]}.{new_version_int[1] - 1}"
    if previous_minor == "7.15":
        previous_minor = "6.15"  # 7.15 is the first release in the 7.x series
    log_result = ctx.run(
        f"git log {branching_point}...remotes/origin/{previous_minor}.x --name-only --oneline | grep {changelog_dir}/notes/ || true"
    )
    log_result = log_result.stdout.replace('\n', ' ').strip()
    if len(log_result) > 0:
        ctx.run(f"git rm --ignore-unmatch {log_result}")

    # generate the new changelog
    ctx.run(
        f"reno --rel-notes-dir {changelog_dir} report --ignore-cache --earliest-version {branching_point} --version {new_version} --no-show-source > /tmp/new_changelog.rst"
    )

    ctx.run(f"git checkout HEAD -- {changelog_dir}")

    # mac's `sed` has a different syntax for the "-i" paramter
    # GNU sed has a `--version` parameter while BSD sed does not, using that to do proper detection.
    try:
        ctx.run("sed --version", hide='both')
        sed_i_arg = "-i"
    except Failure:
        sed_i_arg = "-i ''"
    # remove the old header from the existing changelog
    ctx.run(f"sed {sed_i_arg} -e '1,4d' {changelog_file}")

    # merging to <changelog_file>
    ctx.run(f"cat {changelog_file} >> /tmp/new_changelog.rst && mv /tmp/new_changelog.rst {changelog_file}")

    # commit new CHANGELOG
    ctx.run(f"git add {changelog_file}")

    print("\nCommit this with:")
    print(f"git commit -m \"Update {changelog_file} for {new_version}\"")
