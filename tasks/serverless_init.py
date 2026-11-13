"""Invoke tasks for serverless-init."""

from __future__ import annotations

import sys

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import color_message
from tasks.libs.serverless_init.release_notes import (
    DEPENDENCY_ONLY_ENTRY,
    NO_NOTE_PLACEHOLDER,
    SERVERLESS_INIT_PATHS,
    added_notes,
    format_bullet,
    format_section,
    list_commits,
    note_entries,
    pr_link,
    read_note,
    ref_exists,
    strip_pr_number,
)


@task(
    help={
        'prev': 'Tag of the previous release, e.g. serverless-init-1.11.3',
        'sha': 'Commit to release, e.g. the commit the release notes branch was created from',
        'version': 'Version of the release, e.g. 1.11.4. Adds the heading and the tag link to the section',
    }
)
def release_notes(ctx, prev, sha, version=None):
    """
    Generate the CHANGELOG-SERVERLESS-INIT.md section of a release, from the reno notes added by the
    commits that changed serverless-init between `prev` and `sha`.

    The section is printed on stdout, the commits without a reno note on stderr.
    Review it before adding it to CHANGELOG-SERVERLESS-INIT.md: changes outside of
    the serverless-init paths can also ship in the image, and a note may cover a change that is not
    user-facing yet.
    """
    for ref in (prev, sha):
        if not ref_exists(ctx, ref):
            raise Exit(f"'{ref}' is not a known commit or tag, fetch the tags with `git fetch --tags`.", code=1)

    commits = list_commits(ctx, prev, sha)
    bullets = []
    without_note = []
    for commit in commits:
        found = False
        for path in added_notes(ctx, commit.sha):
            note = read_note(ctx, sha, path)
            for text in note_entries(note):
                bullets.append(format_bullet(text, commit.subject))
                found = True
        if not found:
            without_note.append(commit)

    if not bullets and commits:
        print(
            color_message(
                f"No reno note found: describe the commits below, or replace the placeholder with '{DEPENDENCY_ONLY_ENTRY}'"
                ' if none of them changes the image.',
                'orange',
            ),
            file=sys.stderr,
        )
        bullets = [f'* {NO_NOTE_PLACEHOLDER}']
    elif not bullets:
        print(
            color_message(
                f"Nothing changed under the serverless-init paths: assuming '{DEPENDENCY_ONLY_ENTRY}'."
                ' Check that no change elsewhere (toolchain, shared libraries) ships in the image.',
                'orange',
            ),
            file=sys.stderr,
        )
        bullets = [f'* {DEPENDENCY_ONLY_ENTRY}']

    if without_note:
        print(
            color_message(
                f'\nCommits changing {", ".join(SERVERLESS_INIT_PATHS)} without a reno note (review them):', 'orange'
            ),
            file=sys.stderr,
        )
        for commit in without_note:
            print(
                f'  {commit.sha[:11]} {strip_pr_number(commit.subject)} {pr_link(commit.subject)}'.rstrip(),
                file=sys.stderr,
            )
        print(file=sys.stderr)

    print(format_section(version, bullets), end='')
