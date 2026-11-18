import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import fetch_skills


class FetchSkillsTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.source = self.root / 'marketplace'
        self.cache = self.root / 'cache'
        fetch_skills.git('init', '--quiet', '--initial-branch=main', str(self.source))
        for path in fetch_skills.ENTRY_SKILLS:
            file = self.source / path
            file.parent.mkdir(parents=True, exist_ok=True)
            file.write_text('Initial guidance\n')
        unrelated = self.source / 'unrelated/skills/other/SKILL.md'
        unrelated.parent.mkdir(parents=True)
        unrelated.write_text('Unrelated guidance\n')
        self.commit()
        source_patch = patch.object(fetch_skills, 'MARKETPLACE_URL', str(self.source))
        source_patch.start()
        self.addCleanup(source_patch.stop)

    def commit(self):
        fetch_skills.git('add', '.', cwd=self.source)
        fetch_skills.git(
            '-c',
            'user.name=Test',
            '-c',
            'user.email=test@example.com',
            '-c',
            'commit.gpgsign=false',
            'commit',
            '--quiet',
            '-m',
            'Update fixture',
            cwd=self.source,
        )
        return fetch_skills.git('rev-parse', 'HEAD', cwd=self.source)

    def test_fetches_only_client_plugins_and_refreshes_the_cache(self):
        result = fetch_skills.fetch_skills('main', self.cache)
        common = Path(result['common'])
        self.assertTrue((common / 'skills/using-smp/SKILL.md').is_file())
        self.assertTrue(
            (Path(result['datadog_agent']) / 'skills/investigating-datadog-agent-performance/SKILL.md').is_file()
        )
        self.assertFalse((common.parents[1] / 'unrelated').exists())
        (self.source / fetch_skills.ENTRY_SKILLS[0]).write_text('Updated guidance\n')
        revision = self.commit()
        updated = fetch_skills.fetch_skills('main', self.cache)
        self.assertEqual(updated['revision'], revision)
        self.assertEqual(updated['common'], result['common'])
        self.assertEqual((common / 'skills/using-smp/SKILL.md').read_text(), 'Updated guidance\n')

    def test_preview_ref_has_a_separate_cache(self):
        baseline = fetch_skills.fetch_skills('main', self.cache)
        fetch_skills.git('checkout', '--quiet', '-b', 'preview/smp', cwd=self.source)
        (self.source / fetch_skills.ENTRY_SKILLS[0]).write_text('Preview guidance\n')
        revision = self.commit()
        preview = fetch_skills.fetch_skills('preview/smp', self.cache)
        self.assertEqual(preview['revision'], revision)
        self.assertNotEqual(preview['common'], baseline['common'])
        self.assertEqual((Path(baseline['common']) / 'skills/using-smp/SKILL.md').read_text(), 'Initial guidance\n')

    def test_supports_git_ssh_url_rewrites(self):
        source = 'git@example.com:marketplace.git'
        config = {
            'GIT_CONFIG_COUNT': '1',
            'GIT_CONFIG_KEY_0': f'url.{self.source}.insteadOf',
            'GIT_CONFIG_VALUE_0': source,
        }
        with patch.object(fetch_skills, 'MARKETPLACE_URL', source), patch.dict(os.environ, config):
            result = fetch_skills.fetch_skills('main', self.cache)
        self.assertTrue((Path(result['common']) / 'skills/using-smp/SKILL.md').is_file())

    def test_rejects_local_changes_without_overwriting_them(self):
        result = fetch_skills.fetch_skills('main', self.cache)
        file = Path(result['common']) / 'skills/using-smp/SKILL.md'
        file.write_text('Local changes\n')
        with self.assertRaisesRegex(RuntimeError, 'local changes'):
            fetch_skills.fetch_skills('main', self.cache)
        self.assertEqual(file.read_text(), 'Local changes\n')

    def test_rejects_an_unexpected_remote(self):
        result = fetch_skills.fetch_skills('main', self.cache)
        checkout = Path(result['common']).parents[1]
        fetch_skills.git('remote', 'set-url', 'origin', 'git@example.com:other/source.git', cwd=checkout)
        with self.assertRaisesRegex(RuntimeError, 'Unexpected skill source'):
            fetch_skills.fetch_skills('main', self.cache)

    def test_incomplete_source_does_not_replace_valid_guidance(self):
        result = fetch_skills.fetch_skills('main', self.cache)
        (self.source / fetch_skills.ENTRY_SKILLS[0]).unlink()
        self.commit()
        with self.assertRaises(RuntimeError):
            fetch_skills.fetch_skills('main', self.cache)
        checkout = Path(result['common']).parents[1]
        self.assertEqual(fetch_skills.git('rev-parse', 'HEAD', cwd=checkout), result['revision'])

    def test_invalid_ref_does_not_create_a_cache(self):
        with self.assertRaises(RuntimeError):
            fetch_skills.fetch_skills('--upload-pack=other', self.cache)
        self.assertFalse(self.cache.exists())

    def test_fetch_failure_leaves_no_partial_checkout(self):
        with self.assertRaises(RuntimeError):
            fetch_skills.fetch_skills('missing-branch', self.cache)
        self.assertEqual(list(self.cache.iterdir()), [])


if __name__ == '__main__':
    unittest.main()
