import copy
import io
import json
import os
import subprocess
import tarfile
import tempfile
import unittest
from dataclasses import replace
from pathlib import Path
from unittest.mock import patch

import yaml
from invoke import Context
from invoke.exceptions import Exit

from tasks import bazel_parity as parity
from tasks.files_inventory import _compare_inventories
from tasks.libs.package.metadata import (
    RPM_FIELDS,
    compare_metadata,
    normalize_metadata,
    parse_deb_control,
    read_deb_metadata,
    read_rpm_metadata,
)
from tasks.static_quality_gates.experimental_gates import FileInfo

SCOPE = {'product': 'datadog-agent', 'format': 'deb', 'arch': 'amd64'}


def file(path='opt/datadog-agent/agent', size=100, **kwargs):
    return FileInfo(path, size, chmod=0o755, owner=0, group=0, **kwargs)


class InventoryTests(unittest.TestCase):
    def test_historical_growth_threshold_is_unchanged(self):
        for size, changed in ((80, False), (110, False), (111, True)):
            with self.subTest(size=size):
                self.assertEqual(bool(_compare_inventories([file()], [file(size=size)])[2]), changed)

    def test_parity_flags_sizes_in_both_directions_including_zero(self):
        for old, new in ((100, 99), (100, 101), (0, 1), (1, 0), (0, 0)):
            with self.subTest(old=old, new=new):
                _, counts = parity.file_tasks([file(size=old)], [file(size=new)])
                self.assertEqual(counts['mismatch'], int(old != new))

    def test_all_file_attributes_and_directory_grouping(self):
        previous = [file(), file('etc/a'), file('etc/b')]
        current = [replace(file(), chmod=0o700, owner=1, group=2), file('usr/bin/extra')]
        tasks, counts = parity.file_tasks(previous, current)
        self.assertEqual(counts, {'match': 0, 'missing_in_bazel': 2, 'extra_in_bazel': 1, 'mismatch': 1})
        groups = {t['item']: t for t in tasks}
        self.assertEqual(set(groups), {'directory_etc', 'directory_opt', 'directory_usr'})
        self.assertEqual(len(groups['directory_etc']['differences']), 2)
        self.assertEqual(groups['directory_opt']['differences'][0]['flags'], ['permissions', 'owner', 'group'])

    def test_matches_emit_done_tasks(self):
        tasks, counts = parity.file_tasks([file()], [file()])
        parity.apply_expected_differences(tasks, [])
        self.assertEqual(tasks[0]['status'], 'done')
        self.assertEqual(counts['match'], 1)

    def test_installer_path_and_symlink_stamp_normalization(self):
        prefix = 'opt/datadog-packages/run/datadog-agent/'
        previous = [
            file(prefix + '7.76.0-devel.pipeline.123'),
            file(
                'opt/datadog-packages/datadog-agent/stable',
                is_symlink=True,
                symlink_target='/' + prefix + '7.76.0-devel.pipeline.123',
            ),
        ]
        current = [
            file(prefix + '7.76.0-devel.pipeline.98765'),
            replace(previous[1], symlink_target='/' + prefix + '7.76.0-devel.pipeline.98765'),
        ]
        _, counts = parity.file_tasks(previous, current)
        self.assertEqual(counts['match'], 2)
        self.assertEqual(counts['mismatch'], 0)

    def test_symlink_target_and_type_changes_are_significant(self):
        old = file(is_symlink=True, symlink_target='/target')
        for new in (replace(old, symlink_target='target'), file()):
            tasks, counts = parity.file_tasks([old], [new])
            self.assertEqual(counts['mismatch'], 1)
            self.assertIn('symlink', tasks[0]['differences'][0]['flags'])

    def test_normalization_collisions_fail(self):
        with self.assertRaisesRegex(ValueError, 'collision'):
            parity.normalize_inventory(
                [file('opt/datadog-packages/run/datadog-agent/v1'), file('opt/datadog-packages/run/datadog-agent/v2')]
            )


class MetadataTests(unittest.TestCase):
    def test_deb_control_parsing_and_dependencies(self):
        control = (
            'Package: datadog-agent\nVersion: 1:7.76.0-devel.git.123-1\n'
            'Vendor: Datadog <package@datadoghq.com>\n'
            'Depends: libc6 (>= 2.17),\n libgcc-s1 | libgcc1\n'
            'Description: Datadog Monitoring Agent\n .\n More text\n'
        )
        parsed = parse_deb_control(control)
        self.assertEqual(parsed['description'], 'Datadog Monitoring Agent\n.\nMore text')
        normalized = normalize_metadata(parsed, 'deb')
        self.assertEqual(normalized['version'], '1:7.76.0-devel.git.123-1')
        self.assertEqual(normalized['depends'], ['libc6 (>= 2.17)', 'libgcc-s1 | libgcc1'])
        other = dict(parsed, depends='libgcc-s1 | libgcc1, libc6 (>= 2.17)')
        self.assertEqual(normalized, normalize_metadata(other, 'deb'))
        other['depends'] = 'libc6 (>= 2.18)'
        self.assertIn('depends', compare_metadata(normalized, normalize_metadata(other, 'deb')))

    def test_version_stamps_epochs_revisions_and_release_candidates_are_compared(self):
        for previous, current in (
            ('1:7.76.0~devel.git.42-1', '1:7.76.0~localbuild-1'),
            ('1:7.76.0~devel.git.42-1', '1:7.76.0~devel.git.43-1'),
            ('1:7.76.0-1', '2:7.76.0-1'),
            ('1:7.76.0-1', '1:7.76.0-2'),
            ('7.76.0-rc.1', '7.76.0'),
        ):
            with self.subTest(previous=previous, current=current):
                old = normalize_metadata({'version': previous}, 'deb')
                new = normalize_metadata({'version': current}, 'deb')
                self.assertEqual(compare_metadata(old, new)['version'], {'omnibus': previous, 'bazel': current})

    def test_rpm_absolute_dependencies_and_script_interpreters_are_preserved(self):
        old = normalize_metadata({'requires': '/bin/sh\nlibc.so.6'}, 'rpm')
        new = normalize_metadata({'requires': 'bin/sh\nlibc.so.6'}, 'rpm')
        self.assertIn('requires', compare_metadata(old, new))

    def test_dependency_versions_and_release_stamps_are_preserved(self):
        normalized = normalize_metadata(
            {
                'name': 'datadog-agent',
                'version': '7.76.0~devel.git.42',
                'release': '1.pipeline.42',
                'requires': 'other = 7.76.0~devel.git.42',
                'provides': 'datadog-agent = 7.76.0~devel.git.42-1.pipeline.42',
            },
            'rpm',
        )
        self.assertEqual(normalized['requires'], ['other = 7.76.0~devel.git.42'])
        self.assertEqual(normalized['provides'], ['datadog-agent = 7.76.0~devel.git.42-1.pipeline.42'])
        self.assertEqual(normalized['release'], '1.pipeline.42')
        self.assertEqual(normalize_metadata({'release': '1.el7'}, 'rpm')['release'], '1.el7')

    def test_rpm_query_parsing_missing_fields_and_relations(self):
        fields = dict.fromkeys(RPM_FIELDS, '(none)')
        fields.update(
            name='datadog-agent',
            epoch='1',
            version='7.76.0-devel.git.42',
            release='1',
            arch='x86_64',
            summary='Datadog: Monitoring Agent',
        )
        outputs = [
            '\x1f'.join(fields[k] for k in RPM_FIELDS),
            'libc\n/bin/sh',
            '',
            'datadog-agent = 7.76.0-devel.git.42-1\nlibagent.so',
            '',
            'preinstall scriptlet (using /bin/sh):\necho a\necho b',
            '',
            '',
            '/etc/b\n/etc/a',
        ]
        with patch('tasks.libs.package.metadata.subprocess.check_output', side_effect=outputs) as query:
            result = normalize_metadata(read_rpm_metadata(Path('agent.rpm')), 'rpm')
        self.assertEqual(query.call_count, 9)
        self.assertEqual(result['arch'], 'amd64')
        self.assertEqual(result['vendor'], '')
        self.assertEqual(result['summary'], 'Datadog: Monitoring Agent')
        self.assertEqual(result['configfiles'], ['etc/a', 'etc/b'])
        self.assertEqual(result['provides'], ['datadog-agent = 7.76.0-devel.git.42-1', 'libagent.so'])

    def test_scripts_keep_order_and_meaningful_whitespace(self):
        for field in ('preinst', 'scripts', 'triggers', 'filetriggers'):
            with self.subTest(field=field):
                old = normalize_metadata({field: 'echo a\necho b\n'}, 'rpm')
                new = normalize_metadata({field: 'echo b\necho a\n'}, 'rpm')
                self.assertIn(field, compare_metadata(old, new))

    def test_installed_size_tolerance_is_symmetric_and_zero_safe(self):
        for old, new, differs in (
            ('100', '90', False),
            ('100', '110', False),
            ('100', '89', True),
            ('100', '111', True),
            ('0', '1', True),
            ('', '0', True),
        ):
            self.assertEqual(bool(compare_metadata({'installed-size': old}, {'installed-size': new})), differs)

    def test_deb_control_archive_scripts_conffiles_and_vendor(self):
        stream = io.BytesIO()
        with tarfile.open(fileobj=stream, mode='w') as tar:
            for name, value in {'postinst': '#!/bin/sh\necho hello\n', 'conffiles': '/etc/datadog-agent/a\n'}.items():
                member = tarfile.TarInfo('./' + name)
                member.size = len(value)
                tar.addfile(member, io.BytesIO(value.encode()))
        with (
            patch(
                'tasks.libs.package.metadata.subprocess.check_output',
                side_effect=['Package: datadog-agent\nVendor: Datadog', stream.getvalue()],
            ),
        ):
            result = read_deb_metadata(Path('agent.deb'))
        self.assertEqual(result['postinst'], '#!/bin/sh\necho hello\n')
        self.assertEqual(result['vendor'], 'Datadog')
        self.assertEqual(result['preinst'], '')
        self.assertEqual(result['conffiles'], '/etc/datadog-agent/a\n')


class ExpectedDifferencesTests(unittest.TestCase):
    def test_mismatch_exceptions_pin_flags_and_file_values(self):
        tasks, _ = parity.file_tasks([file()], [file(size=90)])
        item = {**SCOPE, **tasks[0]}
        rule = {
            **SCOPE,
            'id': 'size',
            'check': 'file',
            'item': item['item'],
            **item['differences'][0],
            'reason': 'Reviewed size difference',
        }
        parity.apply_expected_differences([item], [rule])
        self.assertEqual(item['status'], 'accepted')
        tasks, _ = parity.file_tasks([file()], [replace(file(size=90), owner=42)])
        item = {**SCOPE, **tasks[0]}
        parity.apply_expected_differences([item], [rule])
        self.assertEqual(item['status'], 'remaining')

    def test_duplicate_exception_ids_and_unknown_scope_are_rejected(self):
        entry = {
            **SCOPE,
            'id': 'vendor',
            'check': 'metadata',
            'item': 'field_vendor',
            'reason': 'Reviewed',
            'omnibus': 'Datadog',
            'bazel': '',
        }
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'exceptions.yaml'
            for entries in ([entry, entry], [{**entry, 'format': 'debian'}]):
                path.write_text(yaml.safe_dump({'differences': entries}))
                with self.assertRaises(ValueError):
                    parity.load_expected_differences(str(path))

    def test_partial_acceptance_does_not_hide_directory_gaps(self):
        task = {
            **SCOPE,
            'check': 'file',
            'item': 'directory_etc',
            'differences': [
                {'path': 'etc/a', 'result': 'missing_in_bazel'},
                {'path': 'etc/b', 'result': 'missing_in_bazel'},
            ],
        }
        rule = {
            **SCOPE,
            'id': 'a',
            'check': 'file',
            'item': 'directory_etc',
            'path': 'etc/a',
            'result': 'missing_in_bazel',
            'reason': 'Reviewed exception',
        }
        self.assertEqual(parity.apply_expected_differences([task], [rule]), [])
        self.assertEqual(task['status'], 'remaining')
        task['differences'].pop()
        parity.apply_expected_differences([task], [rule])
        self.assertEqual(task['status'], 'accepted')

    def test_metadata_exceptions_are_scoped_and_pin_values(self):
        task = {
            **SCOPE,
            'check': 'metadata',
            'item': 'field_vendor',
            'differences': [{'omnibus': 'Datadog', 'bazel': ''}],
        }
        rule = {
            **SCOPE,
            'check': 'metadata',
            'item': 'field_vendor',
            'id': 'vendor',
            'reason': 'Reviewed',
            'omnibus': 'Datadog',
            'bazel': '',
        }
        for changes, status in (({}, 'accepted'), ({'format': 'rpm'}, 'remaining'), ({'bazel': 'Other'}, 'remaining')):
            item = copy.deepcopy(task)
            parity.apply_expected_differences([item], [{**rule, **changes}])
            self.assertEqual(item['status'], status)

    def test_unused_exceptions_and_empty_reasons(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'expected.yaml'
            rule = {
                **SCOPE,
                'id': 'a',
                'check': 'metadata',
                'item': 'field_vendor',
                'reason': '',
                'omnibus': 'Datadog',
                'bazel': '',
            }
            path.write_text(yaml.safe_dump({'differences': [rule]}))
            with self.assertRaisesRegex(ValueError, 'reason'):
                parity.load_expected_differences(str(path))
            rule['reason'] = 'Reviewed'
            path.write_text(yaml.safe_dump({'differences': [rule]}))
            rules = parity.load_expected_differences(str(path))
            self.assertEqual(parity.apply_expected_differences([], rules), ['a'])


class ReportingTests(unittest.TestCase):
    def test_pairing_ignores_other_products_architectures_and_debug_packages(self):
        with tempfile.TemporaryDirectory() as tmp:
            for name in (
                'datadog-agent_7.76.0-1_amd64.deb',
                'datadog-agent-dbg_7.76.0-1_amd64.deb',
                'datadog-agent_7.76.0-1_arm64.deb',
                'datadog-iot-agent_7.76.0-1_amd64.deb',
                'datadog-agent-fips_7.76.0-1_amd64.deb',
                'datadog-agent-7.76.0-1.x86_64.rpm',
            ):
                (Path(tmp) / name).touch()
            self.assertEqual(
                parity.find_packages(tmp, 'datadog-agent', 'deb', 'amd64').name, 'datadog-agent_7.76.0-1_amd64.deb'
            )
            self.assertEqual(parity.find_packages(tmp, 'datadog-agent', 'rpm', 'amd64').suffix, '.rpm')
            (Path(tmp) / 'datadog-agent_7.77.0-1_amd64.deb').touch()
            with self.assertRaisesRegex(ValueError, 'Ambiguous'):
                parity.find_packages(tmp, 'datadog-agent', 'deb', 'amd64')

    def test_tag_limits_lowercase_and_collision_resistance(self):
        tags = parity.build_tags(branch='Feature/My branch', item='a' * 250)
        self.assertEqual(tags[0], 'branch:feature/my_branch')
        self.assertEqual(len(tags[1]), 200)
        self.assertNotEqual(tags[1], parity.build_tags(item='a' * 249 + 'b')[0])
        self.assertTrue(all(t == t.lower() for t in tags))

    def test_submission_gating_and_dry_run_override(self):
        report = {'metrics': [{'name': 'test', 'value': 1, 'tags': []}]}
        for branch, override, dry_run, enabled in (
            ('main', '', False, True),
            ('feature', '', False, False),
            ('7.82.x', '', False, False),
            ('7.82.x', 'true', False, True),
            ('feature', 'true', False, True),
            ('main', 'true', True, False),
            ('', '', False, False),
            ('feature', 'false', False, False),
            ('feature', 'TRUE', False, False),
        ):
            with (
                self.subTest(branch=branch, override=override, dry_run=dry_run),
                patch.dict(os.environ, {'CI_COMMIT_BRANCH': branch, 'BAZEL_PARITY_SUBMIT': override}),
                patch.object(parity, 'send_metrics') as send,
                patch.object(parity, 'create_gauge') as gauge,
            ):
                parity.submit_metrics(report, dry_run)
                self.assertEqual(send.called, enabled)
                self.assertEqual(gauge.called, enabled)

    def test_report_includes_missing_packages_and_metrics(self):
        with tempfile.TemporaryDirectory() as tmp:
            report = parity.build_report(Context(), tmp, tmp, 'test/bazel_parity/expected_differences.yaml')
            self.assertEqual(len(report['packages']), 2)
            self.assertEqual(report['summary']['remaining'], 2)
            self.assertTrue(all(p['error'].startswith('Missing') for p in report['packages']))
            output = str(Path(tmp) / 'report.json')
            markdown = parity.write_report(report, output)
            self.assertIn('0/2 tasks complete', markdown)
            self.assertEqual(json.loads(Path(output).read_text()), report)
            self.assertTrue(Path(tmp, 'report.md').exists())
            self.assertEqual(len(report['metrics']), 2)

    def test_complete_report_and_metadata_failure(self):
        metadata = {'vendor': 'Datadog', 'version': '7.76.0'}
        with (
            tempfile.TemporaryDirectory() as tmp,
            patch.object(parity, 'find_packages', return_value=Path('agent.deb')),
            patch.object(parity, 'measure_inventory', return_value=[file()]),
            patch.dict(parity.FORMATS, {'deb': lambda _: metadata}, clear=True),
        ):
            report = parity.build_report(Context(), tmp, tmp, 'test/bazel_parity/expected_differences.yaml')
            self.assertEqual(report['summary']['remaining'], 0)
            self.assertEqual(report['summary']['done'], 3)
            self.assertEqual(report['packages'][0]['files']['match'], 1)
            self.assertEqual(len(report['metrics']), 7)
            file_metric = next(m for m in report['metrics'] if 'result:match' in m['tags'])
            self.assertEqual(file_metric['value'], 1)
            self.assertEqual(
                {tag for tag in file_metric['tags'] if not tag.startswith('branch:')},
                {'product:datadog-agent', 'format:deb', 'arch:amd64', 'result:match'},
            )
            with patch.dict(parity.FORMATS, {'deb': lambda _: subprocess.check_output(['false'])}, clear=True):
                report = parity.build_report(Context(), tmp, tmp, 'test/bazel_parity/expected_differences.yaml')
            self.assertEqual(report['summary']['remaining'], 1)
            self.assertIn('error', report['packages'][0])

    def test_task_writes_report_before_submitting_and_returns_failure(self):
        report = {'summary': {'remaining': 1}}
        order = []
        with (
            patch.object(parity, 'build_report', return_value=report),
            patch.object(parity, 'write_report', side_effect=lambda *_: order.append('write')),
            patch.object(parity, 'submit_metrics', side_effect=lambda *_: order.append('submit')),
        ):
            with self.assertRaises(Exit):
                parity.check.body(Context(), '/omnibus', '/bazel', dry_run=True)
        self.assertEqual(order, ['write', 'submit'])


if __name__ == '__main__':
    unittest.main()
