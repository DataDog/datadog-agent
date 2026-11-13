import io
import unittest
from contextlib import redirect_stderr, redirect_stdout
from unittest.mock import MagicMock, patch

from invoke.exceptions import Exit

from tasks import serverless_init
from tasks.libs.serverless_init.release_notes import (
    NO_NOTE_PLACEHOLDER,
    Commit,
    added_notes,
    format_bullet,
    format_section,
    list_commits,
    note_entries,
    pr_link,
    pr_number,
    read_note,
    ref_exists,
    rst_to_markdown,
    strip_pr_number,
)


class TestRstToMarkdown(unittest.TestCase):
    def test_unwraps_lines(self):
        self.assertEqual(
            rst_to_markdown('Fix a bug\nthat spans\n  several lines.\n'), 'Fix a bug that spans several lines.'
        )

    def test_inline_literal(self):
        self.assertEqual(rst_to_markdown('Set ``DD_API_KEY`` to enable it.'), 'Set `DD_API_KEY` to enable it.')

    def test_hyperlink(self):
        self.assertEqual(
            rst_to_markdown('See `the docs <https://example.com/a>`_ for details.'),
            'See [the docs](https://example.com/a) for details.',
        )

    def test_anonymous_hyperlink(self):
        self.assertEqual(rst_to_markdown('See `docs <https://example.com>`__.'), 'See [docs](https://example.com).')


class TestPrLink(unittest.TestCase):
    def test_number(self):
        self.assertEqual(pr_number('fix(serverless-init): a fix (#59170)'), '59170')

    def test_no_number(self):
        self.assertIsNone(pr_number('fix: a fix'))

    def test_number_not_at_the_end(self):
        self.assertIsNone(pr_number('fix (#12) something else'))

    def test_strip_number(self):
        self.assertEqual(strip_pr_number('fix(serverless-init): a fix (#59170)'), 'fix(serverless-init): a fix')
        self.assertEqual(strip_pr_number('fix (#12) something else'), 'fix (#12) something else')

    def test_link(self):
        self.assertEqual(pr_link('a fix (#1)'), '[#1](https://github.com/DataDog/datadog-agent/pull/1)')

    def test_no_link(self):
        self.assertEqual(pr_link('a fix'), '')


class TestNoteEntries(unittest.TestCase):
    def test_all_sections_are_flattened(self):
        note = {'features': ['Add a thing.'], 'fixes': ['Fix a thing.', 'Fix another.']}
        self.assertEqual(note_entries(note), ['Add a thing.', 'Fix a thing.', 'Fix another.'])

    def test_prelude_is_skipped(self):
        self.assertEqual(note_entries({'prelude': 'Release on: today', 'fixes': ['Fix.']}), ['Fix.'])

    def test_string_section(self):
        self.assertEqual(note_entries({'fixes': 'Fix.'}), ['Fix.'])

    def test_empty_and_invalid(self):
        self.assertEqual(note_entries({'fixes': None}), [])
        self.assertEqual(note_entries(None), [])
        self.assertEqual(note_entries(['not', 'a', 'mapping']), [])


class TestFormat(unittest.TestCase):
    def test_bullet_with_pr(self):
        self.assertEqual(
            format_bullet('Fix.', 'fix (#7)'), '* Fix. [#7](https://github.com/DataDog/datadog-agent/pull/7)'
        )

    def test_bullet_without_pr(self):
        self.assertEqual(format_bullet('Fix.', 'fix'), '* Fix.')

    def test_section_with_version(self):
        self.assertEqual(
            format_section('1.11.5', ['* A', '* B']),
            '## 1.11.5\n'
            '[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.11.5)\n'
            '\n'
            '* A\n'
            '* B\n',
        )

    def test_section_without_version(self):
        self.assertEqual(format_section(None, ['* A']), '* A\n')


def _ctx(stdout='', ok=True):
    result = MagicMock(stdout=stdout, ok=ok, failed=not ok)
    return MagicMock(run=MagicMock(return_value=result))


class TestGit(unittest.TestCase):
    def test_ref_exists(self):
        self.assertTrue(ref_exists(_ctx(ok=True), 'serverless-init-1.11.3'))
        self.assertFalse(ref_exists(_ctx(ok=False), 'nope'))

    def test_list_commits(self):
        ctx = _ctx('aaa\tfix a (#1)\nbbb\tfeat: b\n\n')
        self.assertEqual(list_commits(ctx, 'p', 's'), [Commit('aaa', 'fix a (#1)'), Commit('bbb', 'feat: b')])
        command = ctx.run.call_args.args[0]
        self.assertIn('--no-merges --reverse', command)
        self.assertIn('p..s', command)
        self.assertTrue(command.endswith('-- cmd/serverless-init pkg/serverless comp/metadata/serverlessinit'))

    def test_added_notes(self):
        ctx = _ctx('releasenotes/notes/a-0123456789abcdef.yaml\n\n')
        self.assertEqual(added_notes(ctx, 'aaa'), ['releasenotes/notes/a-0123456789abcdef.yaml'])
        self.assertIn('--diff-filter=A', ctx.run.call_args.args[0])

    def test_read_note(self):
        self.assertEqual(read_note(_ctx('fixes:\n  - |\n    Fix.\n'), 's', 'path'), {'fixes': ['Fix.\n']})

    def test_read_note_deleted(self):
        self.assertIsNone(read_note(_ctx(ok=False), 's', 'path'))


class TestReleaseNotes(unittest.TestCase):
    NOTES = {
        'aaa': ['releasenotes/notes/a.yaml'],
        'bbb': ['releasenotes/notes/b.yaml', 'releasenotes/notes/gone.yaml'],
        'ccc': [],
    }
    CONTENT = {
        'releasenotes/notes/a.yaml': {'fixes': ['Fix ``a``.']},
        'releasenotes/notes/b.yaml': {'features': ['Add b.', 'Add c.']},
    }

    def run_task(self, commits, version='1.11.5', known_refs=True, notes=None, content=None):
        out, err = io.StringIO(), io.StringIO()
        with (
            patch('tasks.serverless_init.ref_exists', return_value=known_refs),
            patch('tasks.serverless_init.list_commits', return_value=commits),
            patch('tasks.serverless_init.added_notes', side_effect=lambda _ctx, sha: (notes or self.NOTES)[sha]),
            patch(
                'tasks.serverless_init.read_note',
                side_effect=lambda _ctx, _sha, path: (content or self.CONTENT).get(path),
            ),
            redirect_stdout(out),
            redirect_stderr(err),
        ):
            serverless_init.release_notes(MagicMock(), 'prev', 'sha', version=version)
        return out.getvalue(), err.getvalue()

    def test_section(self):
        out, err = self.run_task([Commit('aaa', 'fix a (#1)'), Commit('bbb', 'feat b (#2)')])
        self.assertEqual(
            out,
            '## 1.11.5\n'
            '[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.11.5)\n'
            '\n'
            '* Fix `a`. [#1](https://github.com/DataDog/datadog-agent/pull/1)\n'
            '* Add b. [#2](https://github.com/DataDog/datadog-agent/pull/2)\n'
            '* Add c. [#2](https://github.com/DataDog/datadog-agent/pull/2)\n',
        )
        self.assertEqual(err, '')

    def test_without_version(self):
        out, _ = self.run_task([Commit('aaa', 'fix a')], version=None)
        self.assertEqual(out, '* Fix `a`.\n')

    def test_commits_without_note_are_reported_on_stderr(self):
        out, err = self.run_task([Commit('aaa', 'fix a (#1)'), Commit('ccc', 'refactor (#3)')])
        self.assertNotIn('refactor', out)
        self.assertIn('without a reno note', err)
        self.assertIn('ccc refactor [#3](https://github.com/DataDog/datadog-agent/pull/3)', err)

    def test_note_deleted_since_is_skipped(self):
        out, err = self.run_task([Commit('ddd', 'feat (#4)')], notes={'ddd': ['releasenotes/notes/gone.yaml']})
        self.assertIn('* TODO:', out)
        self.assertIn('ddd feat [#4](https://github.com/DataDog/datadog-agent/pull/4)', err)

    def test_commits_without_any_note_get_a_placeholder_not_dependency_only(self):
        out, err = self.run_task([Commit('ccc', 'feat: support a new platform (#5)')])
        self.assertIn(f'* {NO_NOTE_PLACEHOLDER}\n', out)
        self.assertNotIn('Dependency updates only', out)
        self.assertIn('ccc feat: support a new platform', err)

    def test_nothing_changed_is_dependency_only(self):
        out, err = self.run_task([])
        self.assertIn('* Dependency updates only\n', out)
        self.assertNotIn('TODO', out)
        self.assertIn('Nothing changed under the serverless-init paths', err)

    def test_unknown_ref(self):
        with self.assertRaises(Exit):
            self.run_task([], known_refs=False)
