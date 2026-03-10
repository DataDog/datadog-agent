from __future__ import annotations

import secrets
import shlex
import sys
from datetime import date
from pathlib import Path

import yaml
from invoke import Failure

from tasks.libs.common.constants import DEFAULT_INTEGRATIONS_CORE_BRANCH, GITHUB_REPO_NAME
from tasks.libs.common.git import get_default_branch
from tasks.libs.releasing.version import current_version

# Section ordering and display names for the Markdown changelog.
# This is the canonical list of sections. tasks/libs/linter/releasenotes.py derives
# its CHANGELOG_SECTIONS frozenset from this list (plus 'prelude', handled separately).
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
    return Path(changelog_dir) / 'notes' / f'{slug}-{uid}.yaml'


def _add_prelude(ctx, version: str, release_date: date | None = None) -> None:
    branch = DEFAULT_INTEGRATIONS_CORE_BRANCH
    note_path = _new_fragment_path('releasenotes', f'prelude-release-{version}')
    note_path.parent.mkdir(parents=True, exist_ok=True)
    anchor = version.replace('.', '')
    ic_url = f"https://github.com/DataDog/integrations-core/blob/{branch}/AGENT_CHANGELOG.md#datadog-agent-version-{anchor}"

    with open(note_path, 'w', encoding='utf-8') as f:
        f.write(
            f"""prelude: |
  Released on: {release_date or date.today()}

  Please refer to the [{version} tag on integrations-core]({ic_url}) for the list of changes on the Core Checks.
"""
        )

    ctx.run(f"git add {shlex.quote(str(note_path))}")
    print("\nIf not run as part of finish task, commit this with:")
    print(f"git commit -m \"Add prelude for {version} release\"")


def _add_dca_prelude(ctx, version: str, release_date: date | None = None) -> None:
    """Release of the Cluster Agent should be pinned to a version of the Agent."""
    branch = get_default_branch()
    note_path = _new_fragment_path('releasenotes-dca', f'prelude-release-{version}')
    note_path.parent.mkdir(parents=True, exist_ok=True)
    anchor = version.replace('.', '-')
    changelog_url = f"https://github.com/{GITHUB_REPO_NAME}/blob/{branch}/CHANGELOG.md#{anchor}"

    with open(note_path, 'w', encoding='utf-8') as f:
        f.write(
            f"""prelude: |
  Released on: {release_date or date.today()}
  Pinned to datadog-agent v{version}: [CHANGELOG]({changelog_url}).
"""
        )

    ctx.run(f"git add {shlex.quote(str(note_path))}")
    print("\nIf not run as part of finish task, commit this with:")
    print(f"git commit -m \"Add prelude for {version} release\"")


def _assemble_changelog(fragment_dir: str | Path, version: str) -> str:
    """Collect all fragment YAML files and render them into a Markdown section."""
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
                print(f"WARNING: multiple fragments contain 'prelude'; last one wins: {fragment_file}", file=sys.stderr)
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
                    lines.append(f'  {extra}')
            lines.append('')

    return '\n'.join(lines) + '\n'


def update_changelog_generic(ctx, new_version, changelog_dir, changelog_file):
    if new_version is None:
        latest_version = current_version(ctx, 7)
        print(f"Would generate changelog since {latest_version} (dry run, no version specified)")
        return

    # Assemble new content from all fragments in changelog_dir/notes/
    new_content = _assemble_changelog(changelog_dir, new_version)

    # Prepend to existing changelog
    changelog_path = Path(changelog_file)
    if changelog_path.exists():
        existing = changelog_path.read_text(encoding='utf-8')
        changelog_path.write_text(new_content + existing, encoding='utf-8')
    else:
        changelog_path.write_text(new_content, encoding='utf-8')

    # Remove consumed fragments
    notes_path = Path(changelog_dir) / 'notes'
    for fragment in notes_path.glob('*.yaml'):
        ctx.run(f"git rm -f {shlex.quote(str(fragment))}", warn=True)

    ctx.run(f"git add {shlex.quote(str(changelog_file))}")

    print("\nCommit this with:")
    print(f"git commit -m \"Update {changelog_file} for {new_version}\"")
