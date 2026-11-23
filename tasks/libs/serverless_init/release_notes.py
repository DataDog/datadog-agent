"""Generation of the CHANGELOG-SERVERLESS-INIT.md section of a serverless-init release.

serverless-init has no release notes of its own: contributors add reno notes to `releasenotes/notes/`
like for any other change, and the section of a release is built from the notes added by the commits
that changed serverless-init between two release tags. We detect that a commit changed serverless-init
by checking if it touches one of its paths or adds a reno note with `serverless-init` in its filename.
The second rule lets a change outside of those paths (a shared library, for example) opt in.
"""

from __future__ import annotations

import re
from dataclasses import dataclass

import yaml

from tasks.libs.common.constants import GITHUB_REPO_NAME
from tasks.libs.common.utils import join_command
from tasks.libs.linter.releasenotes import RENO_SECTIONS

# Paths that make up the serverless-init image. Changes elsewhere (shared libraries, dependencies)
# can also ship in the image, so the output of the task still has to be reviewed.
SERVERLESS_INIT_PATHS = ('cmd/serverless-init', 'pkg/serverless', 'comp/metadata/serverlessinit')
RENO_NOTES_PATHSPEC = 'releasenotes/notes/*.yaml'
# A reno note named like this is relevant to serverless-init wherever the change it describes lives
SERVERLESS_INIT_NOTES_PATHSPEC = 'releasenotes/notes/*serverless-init*.yaml'
DEPENDENCY_ONLY_ENTRY = 'Dependency updates only'
# Printed when commits changed serverless-init but none of them has a reno note, so that an unreviewed
# section cannot be mistaken for a complete one
NO_NOTE_PLACEHOLDER = 'TODO: describe the changes, no reno note was found for them'
# The sections of a reno note that hold release notes. The prelude is only used by the Agent release notes
_NOTE_SECTIONS = RENO_SECTIONS - {'prelude'}

_PR_NUMBER_RE = re.compile(r'\(#(\d+)\)\s*$')
_RST_LINK_RE = re.compile(r'`([^`<]+?)\s*<([^>]+)>`_{1,2}')


@dataclass(frozen=True)
class Commit:
    sha: str
    subject: str


def pr_number(subject: str) -> str | None:
    """Returns the PR number GitHub appends to the subject of a squash-merged commit"""
    match = _PR_NUMBER_RE.search(subject)
    return match.group(1) if match else None


def strip_pr_number(subject: str) -> str:
    """Returns the subject of a commit without the PR number GitHub appends to it"""
    return _PR_NUMBER_RE.sub('', subject).rstrip()


def pr_link(subject: str) -> str:
    """Returns the markdown link to the PR of a commit, or an empty string if it has none"""
    number = pr_number(subject)
    return f'[#{number}](https://github.com/{GITHUB_REPO_NAME}/pull/{number})' if number else ''


def rst_to_markdown(text: str) -> str:
    """Converts the inline reStructuredText of a reno note to markdown on a single line"""
    text = ' '.join(text.split())
    text = _RST_LINK_RE.sub(r'[\1](\2)', text)
    return text.replace('``', '`')


def note_entries(note: object) -> list[str]:
    """Returns the markdown text of every entry of a parsed reno note"""
    if not isinstance(note, dict):
        return []
    entries = []
    for section, items in note.items():
        if section not in _NOTE_SECTIONS:
            continue
        for item in [items] if isinstance(items, str) else items or []:
            if isinstance(item, str) and item.strip():
                entries.append(rst_to_markdown(item))
    return entries


def format_bullet(text: str, subject: str) -> str:
    link = pr_link(subject)
    return f'* {text} {link}' if link else f'* {text}'


def format_section(version: str | None, bullets: list[str]) -> str:
    """Returns the changelog section, with its heading and tag link if a version is given"""
    lines = []
    if version:
        lines += [
            f'## {version}',
            f'[Agent Tag](https://github.com/{GITHUB_REPO_NAME}/releases/tag/serverless-init-{version})',
            '',
        ]
    return '\n'.join(lines + bullets) + '\n'


def list_commits(ctx, prev: str, sha: str) -> list[Commit]:
    """Lists the commits between `prev` and `sha` that changed serverless-init, oldest first"""
    # `--pretty=oneline` prints `<full sha> <subject>`. A `--format` string would need `%`, which cmd.exe expands
    pathspecs = [*SERVERLESS_INIT_PATHS, SERVERLESS_INIT_NOTES_PATHSPEC]
    command = join_command(
        ['git', 'log', '--pretty=oneline', '--no-merges', '--reverse', f'{prev}..{sha}', '--', *pathspecs]
    )
    res = ctx.run(command, hide=True, encoding='utf-8')
    commits = []
    for line in res.stdout.splitlines():
        commit_sha, _, subject = line.partition(' ')
        if commit_sha:
            commits.append(Commit(commit_sha, subject))
    return commits


def added_notes(ctx, commit_sha: str) -> list[str]:
    """Lists the reno notes added by a commit"""
    command = join_command(
        ['git', 'show', '--diff-filter=A', '--name-only', '--format=', commit_sha, '--', RENO_NOTES_PATHSPEC]
    )
    res = ctx.run(command, hide=True, encoding='utf-8')
    return [line for line in res.stdout.splitlines() if line.strip()]


def read_note(ctx, sha: str, path: str) -> object | None:
    """Reads a reno note as it is at `sha`, or returns None if it no longer exists there"""
    res = ctx.run(join_command(['git', 'show', f'{sha}:{path}']), hide=True, warn=True, encoding='utf-8')
    if res.failed:
        return None
    return yaml.safe_load(res.stdout)
