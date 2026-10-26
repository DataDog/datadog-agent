import os
import unittest
from dataclasses import replace
from pathlib import Path
from unittest.mock import patch

from tasks import bazel_parity as parity
from tasks.files_inventory import _compare_inventories
from tasks.libs.package.metadata import RPM_FIELDS, parse_deb_control, read_rpm_metadata
from tasks.static_quality_gates.experimental_gates import FileInfo


class BazelParityTests(unittest.TestCase):
    def test_inventory_grouping_and_size_checks(self):
        original = FileInfo('opt/datadog-agent/bin/agent', 100, chmod=0o755, owner=0, group=0)
        for size in (99, 101):
            current = replace(original, size_bytes=size)
            self.assertEqual(_compare_inventories([original], [current]), ([], [], {}))
            tasks, counts = parity.file_tasks(
                [original, FileInfo('etc/a', 0), FileInfo('etc/b', 0)],
                [current, FileInfo('usr/bin/extra', 0)],
            )
            self.assertEqual(counts, {'match': 0, 'missing_in_bazel': 2, 'extra_in_bazel': 1, 'mismatch': 1})
            groups = {item['item']: item['differences'] for item in tasks}
            self.assertEqual(len(groups['directory_etc']), 2)
            self.assertEqual(groups['directory_opt'][0]['flags'], ['size'])
        for old, new in ((0, 1), (1, 0)):
            _, counts = parity.file_tasks([replace(original, size_bytes=old)], [replace(original, size_bytes=new)])
            self.assertEqual(counts['mismatch'], 1)

    def test_metadata_parsing_preserves_values(self):
        control = parse_deb_control(
            'Package: datadog-agent\nVersion: 1:7.76.0~devel.git.42-1\n'
            'Depends: libc6 (>= 2.17),\n libgcc-s1 | libgcc1\n'
        )
        self.assertEqual(control['version'], '1:7.76.0~devel.git.42-1')
        self.assertEqual(control['depends'], 'libc6 (>= 2.17),\nlibgcc-s1 | libgcc1')
        fields = dict.fromkeys(RPM_FIELDS, '(none)')
        fields.update(version='7.76.0~devel.git.42', arch='x86_64', summary='Datadog: Monitoring Agent')
        with patch(
            'tasks.libs.package.metadata.subprocess.check_output',
            side_effect=['\x1f'.join(fields[k] for k in RPM_FIELDS)] + [''] * 8,
        ):
            metadata = read_rpm_metadata(Path('agent.rpm'))
        self.assertEqual({k: metadata[k] for k in RPM_FIELDS}, fields)

    def test_report_keeps_metadata_differences(self):
        previous = {
            'vendor': 'Datadog',
            'version': '7.76.0~devel.git.42',
            'depends': 'libc6, libgcc-s1',
            'installed-size': '100',
            'postinst': '#!/bin/sh\necho hello\n',
        }
        current = {**previous, 'vendor': '', 'depends': 'libgcc-s1, libc6', 'installed-size': '99'}
        metadata = iter((previous, current))
        with (
            patch.object(parity, 'find_packages', return_value=Path('agent.deb')),
            patch.object(parity, 'measure_inventory', return_value=[FileInfo('opt/agent', 1)]),
            patch.dict(parity.FORMATS, {'deb': lambda _: next(metadata)}, clear=True),
        ):
            report = parity.build_report(None, 'omnibus', 'bazel')
        self.assertEqual(report['summary'], {'done': 3, 'remaining': 3})
        items = {item['item']: item for item in report['tasks']}
        self.assertEqual(items['field_vendor']['differences'], [{'omnibus': 'Datadog', 'bazel': ''}])
        self.assertEqual(items['field_depends']['status'], 'remaining')
        self.assertEqual(items['field_installed-size']['status'], 'remaining')
        self.assertEqual(items['field_version']['status'], 'done')
        self.assertEqual(items['field_postinst']['status'], 'done')

    def test_metric_tags_and_submission_gating(self):
        tags = parity.build_tags(branch='Feature/My branch', item='a' * 250)
        self.assertEqual(tags[0], 'branch:feature/my_branch')
        self.assertEqual(len(tags[1]), 200)
        self.assertTrue(all(tag == tag.lower() for tag in tags))
        report = {'metrics': [{'name': 'datadog_agent.bazel_parity.task', 'value': 1, 'tags': tags}]}
        for branch, override, dry_run, enabled in (
            ('main', '', False, True),
            ('feature', '', False, False),
            ('7.82.x', '', False, False),
            ('feature', 'true', False, True),
            ('main', 'true', True, False),
        ):
            with (
                self.subTest(branch=branch, override=override, dry_run=dry_run),
                patch.dict(os.environ, {'CI_COMMIT_BRANCH': branch, 'BAZEL_PARITY_SUBMIT': override}),
                patch.object(parity, 'send_metrics') as send,
                patch.object(parity, 'create_gauge'),
            ):
                parity.submit_metrics(report, dry_run)
                self.assertEqual(send.called, enabled)
