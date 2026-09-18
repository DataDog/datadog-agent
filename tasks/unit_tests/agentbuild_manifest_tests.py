# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

import json
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock, patch

from invoke import Exit

from tasks.libs.agentbuild.manifest import binary_result, file_record, invalidate, tree_record, write_result


class TestAgentBuildManifest(unittest.TestCase):
    def test_exact_inventory_checksum_and_relative_links(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'lib.so.1').write_bytes(b'runtime')
            (root / 'lib.so').symlink_to('lib.so.1')
            tree = tree_record(root)
            self.assertEqual([f['path'] for f in tree['files']], ['lib.so', 'lib.so.1'])
            self.assertEqual(tree['files'][0]['link'], 'lib.so.1')
            self.assertEqual(len(tree['files'][1]['sha256']), 64)
            (root / 'escape').symlink_to('/etc/passwd')
            with self.assertRaises(ValueError):
                tree_record(root)

    def test_stale_manifest_removed_before_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'result.json'
            write_result(path, {'schema': 1, 'image': {'id': 'test'}})
            self.assertEqual(json.loads(path.read_text())['schema'], 1)
            invalidate(path)
            with self.assertRaises(ValueError):
                write_result(path, {'schema': 1, 'image': {}, 'binary': {}})
            self.assertFalse(path.exists())

    def test_legacy_runtime_export_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaisesRegex(ValueError, 'Bazel dev/embedded'):
                binary_result('/not-used', directory, '/not-used', {})

    def test_native_binary_result_binds_core_profile_to_output_inventory(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            runtime = root / 'dev/embedded'
            runtime.mkdir(parents=True)
            (runtime / 'library').write_bytes(b'runtime')
            assets = root / 'assets'
            assets.mkdir()
            (assets / 'config').write_bytes(b'config')
            binary = root / 'agent'
            binary.write_bytes(b'compiled fixture')
            provenance = {
                'producer': 'invoke-binary',
                'sourceSHA256': 'a' * 64,
                'options': {'runtimeLayout': 'bazel-embedded-absolute-prefix'},
            }
            with patch('tasks.libs.agentbuild.manifest.target', return_value={'os': 'linux', 'arch': 'amd64'}):
                result = binary_result(binary, runtime, assets, provenance)
            self.assertEqual(result['profile'], {'RouteContract': 'agent-outbound-v1', 'Roles': ['core-agent']})
            self.assertEqual(result['binary']['executable'], file_record(binary))
            self.assertEqual(result['binary']['runtime'], tree_record(runtime))
            with self.assertRaises(ValueError):
                binary_result(binary, runtime, assets, {'producer': 'unknown'})

    def test_symlink_executable_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'agent'
            path.symlink_to('/bin/sh')
            with self.assertRaises(ValueError):
                file_record(path)

    @unittest.skipUnless(shutil.which('dpkg-deb'), 'native DEB metadata tool unavailable')
    def test_package_result_inspects_real_archive_metadata_and_roles(self):
        from tasks.libs.agentbuild.manifest import package_result

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / 'root'
            (root / 'DEBIAN').mkdir(parents=True)
            (root / 'DEBIAN/control').write_text(
                'Package: datadog-agent\nVersion: 7.83.0-1\nArchitecture: amd64\nMaintainer: Fixture <fixture@example.com>\nDepends: libc6\nDescription: offline artifact fixture\n'
            )
            binary = root / 'opt/datadog-agent/bin/agent/agent'
            binary.parent.mkdir(parents=True)
            binary.write_text('fixture bytes, not an installed Agent')
            binary.chmod(0o755)
            archive = Path(directory) / 'agent.deb'
            subprocess.run(['dpkg-deb', '--build', str(root), str(archive)], check=True, capture_output=True)
            result = package_result(archive, {'producer': 'existing-package'})
            self.assertEqual(result['target'], {'os': 'linux', 'arch': 'amd64'})
            self.assertEqual(result['package']['roles'], ['agent'])
            self.assertEqual(result['package']['dependencies'], 'libc6')
            self.assertEqual(result['package']['file'], file_record(archive))

    def test_repack_explicit_base_does_not_resolve_latest_nightly(self):
        from tasks.omnibus import build_repackaged_agent

        ctx = Mock()
        ctx.run.return_value.stdout = 'amd64\n'
        with (
            patch('tasks.omnibus.os.path.exists', return_value=False),
            patch('tasks.omnibus.requests.get') as network,
            patch('tasks.omnibus.get_omnibus_env', return_value={}),
            patch('tasks.omnibus.bundle_install_omnibus'),
            patch('tasks.omnibus.omnibus_run_task') as build,
        ):
            build_repackaged_agent.body(
                ctx, base_package_url='https://example.com/agent.deb', base_package_sha256='a' * 64
            )
            network.assert_not_called()
            self.assertEqual(
                build.call_args.kwargs['env']['OMNIBUS_REPACKAGE_SOURCE_URL'], 'https://example.com/agent.deb'
            )
            self.assertNotIn('package_dir', build.call_args.kwargs)

    def test_repack_explicit_base_preserves_safety_prompt(self):
        from tasks.omnibus import build_repackaged_agent

        with (
            patch('tasks.omnibus.os.path.exists', return_value=True),
            patch('tasks.omnibus.yes_no_question', return_value=False) as prompt,
            patch('tasks.omnibus._clear_agent_install_directory') as clear,
        ):
            with self.assertRaisesRegex(Exit, 'cancelled'):
                build_repackaged_agent.body(
                    Mock(), base_package_url='https://example.com/agent.deb', base_package_sha256='a' * 64
                )
            prompt.assert_called_once()
            clear.assert_not_called()

    def test_repack_rejects_missing_checksum_before_prompt_or_command(self):
        from tasks.omnibus import build_repackaged_agent

        ctx = Mock()
        with patch('tasks.omnibus.yes_no_question') as prompt:
            with self.assertRaises(Exit):
                build_repackaged_agent.body(ctx, base_package_url='https://example.com/agent.deb')
            prompt.assert_not_called()
            ctx.run.assert_not_called()
