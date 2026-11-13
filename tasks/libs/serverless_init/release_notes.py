"""Generation of the CHANGELOG-SERVERLESS-INIT.md section of a serverless-init release.

serverless-init has no release notes of its own: contributors add reno notes to `releasenotes/notes/`
like for any other change, and the section of a release is built from the notes added by the commits
that changed serverless-init between two release tags.
"""

from __future__ import annotations

import re
import shlex
from dataclasses import dataclass

import yaml

from tasks.libs.common.constants import GITHUB_REPO_NAME

# Paths that make up the serverless-init image. Changes elsewhere (shared libraries, dependencies)
# can also ship in the image, so the output of the task still has to be reviewed.
SERVERLESS_INIT_PATHS = ('cmd/serverless-init', 'pkg/serverless', 'comp/metadata/serverlessinit')
RENO_NOTES_PATHSPEC = 'releasenotes/notes/*.yaml'
DEPENDENCY_ONLY_ENTRY = 'Dependency updates only'
# Printed when commits changed serverless-init but none of them has a reno note, so that an unreviewed
# section cannot be mistaken for a complete one
NO_NOTE_PLACEHOLDER = 'TODO: describe the changes, no reno note was found for them'

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
        # The prelude is only used by the Agent release notes
        if section == 'prelude':
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


def ref_exists(ctx, ref: str) -> bool:
    return ctx.run(f'git rev-parse --verify --quiet {shlex.quote(ref + "^{commit}")}', hide=True, warn=True).ok


def list_commits(ctx, prev: str, sha: str) -> list[Commit]:
    """Lists the commits between `prev` and `sha` that changed serverless-init, oldest first"""
    res = ctx.run(
        f'git log --no-merges --reverse --format=%H%x09%s {shlex.quote(prev)}..{shlex.quote(sha)}'
        f' -- {" ".join(SERVERLESS_INIT_PATHS)}',
        hide=True,
    )
    commits = []
    for line in res.stdout.splitlines():
        commit_sha, _, subject = line.partition('\t')
        if commit_sha:
            commits.append(Commit(commit_sha, subject))
    return commits


def added_notes(ctx, commit_sha: str) -> list[str]:
    """Lists the reno notes added by a commit"""
    res = ctx.run(
        f'git show --diff-filter=A --name-only --format= {shlex.quote(commit_sha)} -- {shlex.quote(RENO_NOTES_PATHSPEC)}',
        hide=True,
    )
    return [line for line in res.stdout.splitlines() if line.strip()]


def read_note(ctx, sha: str, path: str) -> object | None:
    """Reads a reno note as it is at `sha`, or returns None if it no longer exists there"""
    res = ctx.run(f'git show {shlex.quote(sha)}:{shlex.quote(path)}', hide=True, warn=True)
    if res.failed:
        return None
    return yaml.safe_load(res.stdout)
