import os
import subprocess
import unittest
from dataclasses import replace
from pathlib import Path
from unittest.mock import patch

import yaml
from invoke import Context
from invoke.exceptions import Exit

from tasks import bazel_parity as parity
from tasks.libs.ciproviders.gitlab_api import ReferenceTag
from tasks.libs.package.binaries import binary_signatures, signature_differences
from tasks.libs.package.metadata import RPM_FIELDS, parse_deb_control, read_rpm_metadata
from tasks.static_quality_gates.experimental_gates import FileInfo


def product(name, **kwargs):
    return parity.Product(name, f'//{name}', bazel_job=f'{name}-bazel', omnibus_jobs=(f'{name}-omnibus',), **kwargs)


class TestBazelParity(unittest.TestCase):
    def test_file_differences_are_grouped_by_directory(self):
        agent = FileInfo('opt/datadog-agent/bin/agent', 100, chmod=0o755, owner=0, group=0)
        tasks, counts = parity.file_tasks(
            [agent, FileInfo('etc/a', 0), FileInfo('etc/b', 0)],
            [replace(agent, size_bytes=120), FileInfo('usr/bin/extra', 0)],
        )
        self.assertEqual(counts, {'match': 0, 'missing_in_bazel': 2, 'extra_in_bazel': 1, 'mismatch': 1})
        groups = {item['item']: item['differences'] for item in tasks}
        self.assertEqual(len(groups['directory_etc']), 2)
        self.assertEqual(groups['directory_opt'][0]['flags'], ['size'])
        symlink = replace(agent, is_symlink=True, symlink_target='/target')
        tasks, counts = parity.file_tasks([symlink], [replace(symlink, symlink_target='target2')])
        self.assertEqual(counts['mismatch'], 1)
        self.assertEqual(tasks[0]['differences'][0]['flags'], ['symlink'])

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
            patch.object(parity, 'measure_inventory', return_value=([FileInfo('opt/agent', 1)], {})),
            patch.dict(parity.FORMATS, {'deb': lambda _: next(metadata)}),
            patch.dict(parity.PRODUCTS, {'deb-only': product('deb-only', formats=('deb',))}, clear=True),
        ):
            report = parity.build_report(None, 'omnibus', 'bazel', 'readelf')
        self.assertEqual([p['format'] for p in report['packages']], ['deb'])
        self.assertEqual(report['summary'], {'done': 5, 'remaining': 2})
        items = {item['item']: item for item in report['tasks']}
        self.assertEqual(items['field_vendor']['differences'], [{'omnibus': 'Datadog', 'bazel': ''}])
        self.assertEqual(items['field_depends']['status'], 'remaining')
        self.assertEqual(items['field_installed-size']['status'], 'done')
        self.assertEqual(items['field_version']['status'], 'done')
        self.assertEqual(items['field_signature']['status'], 'done')
        self.assertEqual(items['field_postinst']['status'], 'done')

    def test_bazel_targets(self):
        self.assertEqual(
            product('with-debug', debug_package=True).bazel_targets(),
            ['//with-debug:debian', '//with-debug:debian_dbg', '//with-debug:rpm', '//with-debug:rpm_dbg'],
        )
        self.assertEqual(product('rpm-only', formats=('rpm',)).bazel_targets(), ['//rpm-only:rpm'])
        with self.assertRaises(Exit):
            parity.targets(Context(), 'unknown')

    def test_parity_job_needs_every_registered_package_job(self):
        ci_file = Path(__file__).parents[2] / '.gitlab/test/functional_test/bazel_parity_check.yml'
        yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
        needs = yaml.safe_load(ci_file.read_text())['bazel_parity_check']['needs']
        expected = set()
        for registered in parity.PRODUCTS.values():
            expected.add(registered.bazel_job)
            expected.update(registered.omnibus_jobs)
        self.assertEqual(sorted(needs), sorted(expected))

    def test_metric_tags_and_submission_gating(self):
        tags = parity.metric_tags(path='Opt/My path', item='a' * 250)
        self.assertEqual(tags[0], 'path:opt/my_path')
        self.assertEqual(len(tags[1]), 200)
        self.assertTrue(all(tag == tag.lower() for tag in tags))
        report = {'metrics': [{'name': 'datadog_agent.bazel_parity.task', 'value': 1, 'tags': tags}]}
        for branch, override, enabled in (
            ('main', '', True),
            ('feature', '', False),
            ('7.82.x', '', False),
            ('feature', 'true', True),
        ):
            env = {
                'CI_COMMIT_BRANCH': branch,
                'CI_COMMIT_REF_NAME': branch,
                'CI_COMMIT_SHA': 'abc123',
                'BAZEL_PARITY_SUBMIT': override,
            }
            with (
                self.subTest(branch=branch, override=override),
                patch.dict(os.environ, env),
                patch.object(parity, 'send_metrics') as send,
                patch.object(parity, 'send_event') as event,
                patch.object(parity, 'create_gauge') as gauge,
            ):
                parity.submit_metrics(report)
                self.assertEqual(send.called, enabled)
                self.assertEqual(event.called, enabled)
                if enabled:
                    self.assertIn(f'git.branch:{branch}', gauge.call_args.args[3])
                    self.assertNotIn('git.commit.sha:abc123', gauge.call_args.args[3])
                    self.assertEqual(event.call_args.kwargs['title'], f'Bazel parity: {branch}@abc123')
                    self.assertIn('commit/abc123', event.call_args.kwargs['text'])
                    self.assertEqual(
                        event.call_args.kwargs['tags'],
                        ['bazel_parity', f'git.branch:{branch}', 'git.commit.sha:abc123'],
                    )

    def test_binary_differences_reach_metrics_and_logs(self):
        agent = FileInfo('opt/agent', 10, chmod=0o755)
        static = {'stripped': True, 'interpreter': None, 'needed': [], 'go': {'CGO_ENABLED': '0'}}
        dynamic = {**static, 'stripped': False, 'interpreter': '/lib64/ld.so', 'needed': ['libc.so.6']}
        self.assertEqual(signature_differences(static, dynamic), ['stripped', 'interpreter', 'needed'])
        scope = {'product': 'datadog-agent', 'format': 'deb', 'arch': 'amd64'}
        tasks, counts = parity.file_tasks(
            [agent], [agent], {agent.relative_path: static}, {agent.relative_path: dynamic}
        )
        self.assertEqual(counts['mismatch'], 1)
        report = {
            'summary': {'done': 0, 'remaining': 1},
            'packages': [{**scope, 'files': counts}],
            'tasks': [{**scope, **item} for item in tasks],
        }
        metrics = parity.metric_values(report)
        binary = {
            next(t for t in m['tags'] if t.startswith('reason:')): m['value']
            for m in metrics
            if m['name'].endswith('binary_difference')
        }
        self.assertEqual(binary, {'reason:interpreter': 1, 'reason:needed': 1, 'reason:stripped': 1})
        self.assertFalse(any(tag.startswith(('binary:', 'branch:')) for m in metrics for tag in m['tags']))
        with patch('builtins.print') as printed:
            parity.print_report(report)
        lines = [call.args[0] for call in printed.call_args_list]
        self.assertTrue(all(line.startswith('BAZEL_PARITY ') and '\n' not in line for line in lines))
        self.assertIn(
            'BAZEL_PARITY datadog-agent/deb/amd64 file mismatch opt/agent flags=binary binary=stripped,interpreter,needed'
            ' product=datadog-agent format=deb arch=amd64',
            lines,
        )
        self.assertIn(
            'BAZEL_PARITY datadog-agent/deb/amd64 counts match=0 missing_in_bazel=0 extra_in_bazel=0 mismatch=1'
            ' product=datadog-agent format=deb arch=amd64',
            lines,
        )

    def test_unreadable_binaries_are_reported_per_file(self):
        agent = FileInfo('opt/agent', 10, chmod=0o755)
        good = {'stripped': True, 'go': None}
        bad = {'unreadable': 'truncated'}
        self.assertEqual(signature_differences(good, bad), ['unreadable'])
        self.assertEqual(signature_differences(bad, good), ['unreadable'])
        self.assertEqual(signature_differences(bad, bad), [])
        tasks, counts = parity.file_tasks([agent], [agent], {agent.relative_path: good}, {agent.relative_path: bad})
        self.assertEqual(counts['mismatch'], 1)
        difference = tasks[0]['differences'][0]
        self.assertIn('bazel_error="truncated"', parity.format_difference({'check': 'file'}, difference))
        failing = subprocess.CompletedProcess([], 1, stdout='', stderr='not an ELF\n')
        with (
            patch('tasks.libs.package.binaries.is_elf', return_value=True),
            patch('tasks.libs.package.binaries.subprocess.run', return_value=failing),
        ):
            self.assertEqual(
                binary_signatures('/pkg', ['opt/agent'], 'readelf'), {'opt/agent': {'unreadable': 'not an ELF'}}
            )
        with (
            patch('tasks.libs.package.binaries.is_elf', return_value=True),
            patch('tasks.libs.package.binaries.subprocess.run', side_effect=FileNotFoundError('readelf')),
            self.assertRaises(FileNotFoundError),
        ):
            binary_signatures('/pkg', ['opt/agent'], 'readelf')

    def test_multiline_metadata_logs_only_the_differing_lines(self):
        item = {'check': 'metadata', 'item': 'field_requires'}
        self.assertEqual(
            parity.format_difference(item, {'omnibus': 'a\nb\nb\n', 'bazel': 'a\nb\nc\n'}),
            'metadata requires omnibus_only=["b"] bazel_only=["c"]',
        )
        self.assertEqual(
            parity.format_difference(item, {'omnibus': 'a\nb\n', 'bazel': 'b\na\n'}),
            'metadata requires same lines in a different order',
        )
        self.assertEqual(
            parity.format_difference(
                {'check': 'metadata', 'item': 'field_vendor'}, {'omnibus': 'Datadog', 'bazel': ''}
            ),
            'metadata vendor omnibus="Datadog" bazel=""',
        )
