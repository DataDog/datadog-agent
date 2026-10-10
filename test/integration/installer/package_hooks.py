# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

"""Exercise built installers and real OCI layouts in disposable Linux containers.

This is deliberately not a mock of the installer or its hooks. The inner runner
uses the public installer CLI, its real database, downloader and extractor.
No installer is ever run on the host. See README.md for the safety boundary.
"""

import argparse
import hashlib
import http.server
import io
import json
import os
import select
import signal
import ssl
import subprocess
import tarfile
import tempfile
import threading
import time
import uuid
from pathlib import Path

EVENTS = (
    'preInstall',
    'postInstall',
    'preRemove',
    'preStartExperiment',
    'postStartExperiment',
    'preStopExperiment',
    'postStopExperiment',
    'prePromoteExperiment',
    'postPromoteExperiment',
    'postStartConfigExperiment',
    'preStopConfigExperiment',
    'postPromoteConfigExperiment',
    'resumeConfigExperiment',
    'preInstallExtension',
    'postInstallExtension',
    'preRemoveExtension',
)
APM = 'datadog-apm-inject'
AGENT = 'datadog-agent'
LAYER = 'application/vnd.datadog.package.layer.v1.tar+zstd'
EXTENSION_LAYER = 'application/vnd.datadog.package.extension.layer.v1.tar+zstd'
MANIFEST = 'application/vnd.oci.image.manifest.v1+json'
EVENT_LOG = Path('/tmp/package-hook-events.jsonl')
LEGACY_MARKER = Path('/usr/bin/dd-host-install')
PACKAGE_ROOT = Path('/opt/datadog-packages')
HOOK = '''#!/usr/local/bin/python3
import json
import os
from pathlib import Path
import signal
import sys

ctx = json.load(sys.stdin)
root = Path(ctx['package_path'])
event = {'context': ctx, 'cwd': os.getcwd(), 'resolved_root': str(root.resolve()), 'exists': root.is_dir(),
         'version': (root / 'version').read_text().strip()}
with open('/tmp/package-hook-events.jsonl', 'a') as stream:
    stream.write(json.dumps(event) + '\\n')
action = ACTION
if action == 'fail':
    sys.stderr.write('fixture hook failure\\n')
    sys.exit(42)
if action == 'output':
    sys.stdout.write('stdout-must-not-be-captured' * 100000)
    sys.stderr.write('api_key: ' + 'a' * 32 + '\\n' + 'X' * 131072)
    sys.exit(17)
if action in ('timeout', 'inherited-pipe'):
    signal.signal(signal.SIGTERM, signal.SIG_IGN)
    child = os.fork()
    if child == 0:
        while True:
            signal.pause()
    Path('/tmp/package-hook-pids.json').write_text(json.dumps([os.getpid(), child]))
    if action == 'inherited-pipe':
        sys.exit(0)
    if os.getenv('HOOK_READY_FIFO'):
        with open(os.environ['HOOK_READY_FIFO'], 'w') as stream:
            stream.write('ready')
    while True:
        signal.pause()
'''


def json_bytes(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':')).encode()


def write_blob(root, content, media_type):
    digest = hashlib.sha256(content).hexdigest()
    (root / 'blobs' / 'sha256').mkdir(parents=True, exist_ok=True)
    (root / 'blobs' / 'sha256' / digest).write_bytes(content)
    return {'mediaType': media_type, 'digest': f'sha256:{digest}', 'size': len(content)}


def make_layer(root, entries, media_type, annotations=None):
    archive = io.BytesIO()
    with tarfile.open(fileobj=archive, mode='w') as tar:
        for name, content, mode, link in entries:
            info = tarfile.TarInfo(name)
            info.mode = mode
            if link is not None:
                info.type = tarfile.SYMTYPE
                info.linkname = link
            elif content is None:
                info.type = tarfile.DIRTYPE
            else:
                info.size = len(content)
            tar.addfile(info, io.BytesIO(content) if content is not None else None)
    raw = archive.getvalue()
    compressed = subprocess.run(['zstd', '-q', '-c'], input=raw, capture_output=True, check=True).stdout
    layer = write_blob(root, compressed, media_type)
    if annotations:
        layer['annotations'] = annotations
    return layer, 'sha256:' + hashlib.sha256(raw).hexdigest()


def make_layout(root, architecture, package, version, hooks=None, malformed=None, bundled_installer=None):
    root.mkdir(parents=True)
    entries = [('version', version.encode(), 0o644, None)]
    # A legacy injector fixture can install without shipping or activating an injector.
    # Its real recipe creates dd-host-install, which is our fallback observable.
    if hooks is not None:
        entries.append(('hooks', None, 0o755, None))
        for event, action in hooks.items():
            entries.append((f'hooks/{event}', HOOK.replace('ACTION', repr(action)).encode(), 0o755, None))
    if malformed == 'directory-file':
        entries.append(('hooks', b'not a directory', 0o644, None))
    elif malformed == 'directory-symlink':
        entries.append(('hooks', None, 0o777, '/tmp'))
    elif malformed == 'event-symlink':
        entries.append(('hooks/preInstall', None, 0o777, '/bin/true'))
    elif malformed == 'not-executable':
        entries.append(('hooks/preInstall', b'#!/bin/sh\nexit 0\n', 0o644, None))
    if bundled_installer:
        entries.append(('embedded/bin/installer', bundled_installer.read_bytes(), 0o755, None))

    layer, diff_id = make_layer(root, entries, LAYER)
    extension, extension_diff_id = make_layer(
        root,
        [('extension.txt', b'extension fixture\n', 0o644, None)],
        EXTENSION_LAYER,
        {'com.datadoghq.package.extension.name': 'fixture-extension'},
    )
    config = write_blob(
        root,
        json_bytes(
            {
                'architecture': architecture,
                'os': 'linux',
                'rootfs': {'type': 'layers', 'diff_ids': [diff_id, extension_diff_id]},
            }
        ),
        'application/vnd.oci.image.config.v1+json',
    )
    manifest = write_blob(
        root,
        json_bytes(
            {
                'schemaVersion': 2,
                'mediaType': MANIFEST,
                'config': config,
                'layers': [layer, extension],
                'annotations': {
                    'com.datadoghq.package.name': package,
                    'com.datadoghq.package.version': version,
                    'com.datadoghq.package.size': str(sum(len(content or b'') for _, content, _, _ in entries)),
                },
            }
        ),
        MANIFEST,
    )
    manifest['platform'] = {'os': 'linux', 'architecture': architecture}
    (root / 'index.json').write_bytes(json_bytes({'schemaVersion': 2, 'manifests': [manifest]}))
    (root / 'oci-layout').write_bytes(json_bytes({'imageLayoutVersion': '1.0.0'}))


def make_fixtures(root, architecture, baseline):
    all_hooks = dict.fromkeys(EVENTS, 'success')
    specs = [
        ('legacy', APM, '0.1.0', None, None, None),
        ('owned-v1', APM, '1.0.0', all_hooks, None, None),
        ('owned-v2', APM, '2.0.0', all_hooks, None, None),
        ('empty', APM, '1.0.0', {}, None, None),
        ('post-only', APM, '1.0.0', {'postInstall': 'success'}, None, None),
        ('fail-pre', APM, '1.0.0', {'preInstall': 'fail'}, None, None),
        ('fail-upgrade-pre', APM, '2.0.0', {'preInstall': 'fail'}, None, None),
        ('fail-post', APM, '1.0.0', {'postInstall': 'fail'}, None, None),
        ('fail-remove', APM, '1.0.0', {'preRemove': 'fail'}, None, None),
        ('timeout', APM, '1.0.0', {'preInstall': 'timeout'}, None, None),
        ('inherited-pipe', APM, '1.0.0', {'preInstall': 'inherited-pipe'}, None, None),
        ('output', APM, '1.0.0', {'postInstall': 'output'}, None, None),
        ('directory-file', APM, '1.0.0', None, 'directory-file', None),
        ('directory-symlink', APM, '1.0.0', None, 'directory-symlink', None),
        ('event-symlink', APM, '1.0.0', {}, 'event-symlink', None),
        ('not-executable', APM, '1.0.0', {}, 'not-executable', None),
        ('agent-v1', AGENT, '1.0.0', all_hooks, None, baseline),
        ('agent-v2', AGENT, '2.0.0', all_hooks, None, baseline),
    ]
    for name, package, version, hooks, malformed, bundled in specs:
        make_layout(root / name, architecture, package, version, hooks, malformed, bundled)


class RecordingIntake:
    def __init__(self):
        self.payloads = []
        payloads = self.payloads

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_POST(self):
                payloads.append(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
                self.send_response(200)
                self.end_headers()

            def log_message(self, *_args):
                pass

        self.server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        tls.load_cert_chain('/fixtures/telemetry.crt', '/fixtures/telemetry.key')
        self.server.socket = tls.wrap_socket(self.server.socket, server_side=True)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def environment(self):
        return {
            'DD_SITE': f'hooks.test:{self.server.server_port}',
            'DD_API_KEY': '0' * 32,
            'DATADOG_SAMPLING_PRIORITY': '2',
            'SSL_CERT_FILE': '/fixtures/telemetry.crt',
            'DD_APM_INSTRUMENTATION_ENABLED': 'none',
        }

    def spans(self):
        return [
            span
            for payload in self.payloads
            if payload['request_type'] == 'traces'
            for trace in payload['payload']['traces']
            for span in trace
        ]

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()


def events():
    if not EVENT_LOG.exists():
        return []
    return [json.loads(line) for line in EVENT_LOG.read_text().splitlines()]


def assert_events(expected):
    actual = events()
    assert [(event['version'], event['context']['hook']) for event in actual] == expected, actual
    for event in actual:
        ctx = event['context']
        assert set(ctx) == {
            'package',
            'package_type',
            'package_path',
            'hook',
            'upgrade',
            'windows_args',
            'extension',
        }, ctx
        assert ctx['package_type'] == 'oci', ctx
        assert event['exists'], event
        assert event['cwd'] == event['resolved_root'], event
    return actual


def assert_processes_stopped():
    pids = json.loads(Path('/tmp/package-hook-pids.json').read_text())
    for pid in pids:
        stat = Path(f'/proc/{pid}/stat')
        assert not stat.exists() or stat.read_text().split()[2] == 'Z', f'hook process {pid} still running'


def run_inside(scenario):
    assert Path('/.dockerenv').exists(), 'refusing to execute the installer outside a disposable Docker container'
    assert os.environ.get('DD_PACKAGE_HOOKS_E2E') == 'disposable', 'missing disposable-container guard'
    intake = RecordingIntake()
    environment = os.environ | intake.environment()
    receipts = []

    def run(*args, binary='/installer/new', ok=True, extra_env=None):
        started = time.monotonic()
        result = subprocess.run(
            [binary, *args], input='', text=True, capture_output=True, timeout=35, env=environment | (extra_env or {})
        )
        receipt = {
            'command': [binary, *args],
            'exit_code': result.returncode,
            'duration_seconds': time.monotonic() - started,
            'stdout': result.stdout,
            'stderr': result.stderr,
        }
        receipts.append(receipt)
        print(json.dumps(receipt), flush=True)
        assert (result.returncode == 0) == ok, receipt
        return result

    def install(name, **kwargs):
        return run('install', f'file:///fixtures/{name}', **kwargs)

    try:
        if scenario.startswith('matrix-'):
            _, installer, package = scenario.split('-')
            binary = f'/installer/{installer}'
            fixture = 'legacy' if package == 'old' else 'owned-v1'
            install(fixture, binary=binary)
            legacy = installer == 'old' or package == 'old'
            assert LEGACY_MARKER.exists() == legacy
            if legacy:
                assert_events([])
            else:
                assert_events([('1.0.0', 'preInstall'), ('1.0.0', 'postInstall')])
            run('remove', APM, binary=binary)
            assert not LEGACY_MARKER.exists()
            assert not (PACKAGE_ROOT / APM).exists()
        elif scenario in ('empty', 'post-only'):
            install(scenario)
            assert not LEGACY_MARKER.exists()
            run('remove', APM)
            assert_events([] if scenario == 'empty' else [('1.0.0', 'postInstall')])
            assert not (PACKAGE_ROOT / APM).exists()
        elif scenario == 'upgrade':
            install('owned-v1')
            install('owned-v2')
            run('remove', APM)
            actual = assert_events(
                [
                    ('1.0.0', 'preInstall'),
                    ('1.0.0', 'postInstall'),
                    ('1.0.0', 'preRemove'),
                    ('2.0.0', 'preInstall'),
                    ('2.0.0', 'postInstall'),
                    ('2.0.0', 'preRemove'),
                ]
            )
            assert [event['context']['upgrade'] for event in actual] == [False, False, True, True, True, False]
            assert '/tmp-i-' in actual[0]['context']['package_path']
            assert not LEGACY_MARKER.exists()
        elif scenario == 'mixed-upgrade':
            install('legacy')
            assert LEGACY_MARKER.exists()
            install('owned-v1')
            assert not LEGACY_MARKER.exists()
            install('legacy')
            assert LEGACY_MARKER.exists()
            assert_events([('1.0.0', 'preInstall'), ('1.0.0', 'postInstall'), ('1.0.0', 'preRemove')])
            run('remove', APM)
        elif scenario in (
            'fail-pre',
            'fail-post',
            'directory-file',
            'directory-symlink',
            'event-symlink',
            'not-executable',
        ):
            install(scenario, ok=False)
            run('is-installed', APM, ok=False)
            assert not LEGACY_MARKER.exists()
            if scenario == 'fail-pre':
                assert not (PACKAGE_ROOT / APM / 'stable').exists()
            if scenario in ('fail-pre', 'fail-post'):
                expected = 'preInstall' if scenario == 'fail-pre' else 'postInstall'
                assert_events([('1.0.0', expected)])
                failed = [span for span in intake.spans() if span.get('metrics', {}).get('exit_code') == 42]
                assert failed and failed[0]['error'] == 1 and failed[0]['duration'] > 0, intake.spans()
        elif scenario == 'fail-remove':
            install(scenario)
            run('remove', APM, ok=False)
            run('is-installed', APM)
            assert (PACKAGE_ROOT / APM / 'stable').exists()
            assert_events([('1.0.0', 'preRemove')])
            assert not LEGACY_MARKER.exists()
        elif scenario == 'fail-upgrade-pre':
            install('owned-v1')
            install('fail-upgrade-pre', ok=False)
            run('is-installed', APM)
            assert (PACKAGE_ROOT / APM / 'stable' / 'version').read_text() == '1.0.0'
            assert not LEGACY_MARKER.exists()
            assert_events(
                [
                    ('1.0.0', 'preInstall'),
                    ('1.0.0', 'postInstall'),
                    ('1.0.0', 'preRemove'),
                    ('2.0.0', 'preInstall'),
                ]
            )
            run('remove', APM)
        elif scenario == 'timeout':
            result = install('timeout', ok=False, extra_env={'DD_INSTALLER_PACKAGE_HOOK_TIMEOUT': '500ms'})
            assert 'deadline exceeded' in result.stderr, result.stderr
            assert receipts[-1]['duration_seconds'] < 10
            assert_processes_stopped()
            run('is-installed', APM, ok=False)
            assert not LEGACY_MARKER.exists()
            spans = [span for span in intake.spans() if span.get('meta', {}).get('hook.source') == 'package']
            assert any(span.get('meta', {}).get('hook.timed_out') == 'true' for span in spans), spans
        elif scenario == 'inherited-pipe':
            result = install(scenario, ok=False)
            assert 'WaitDelay' in result.stderr, result.stderr
            assert receipts[-1]['duration_seconds'] < 10
            assert_processes_stopped()
            run('is-installed', APM, ok=False)
            assert not LEGACY_MARKER.exists()
        elif scenario == 'cancellation':
            ready_path = '/tmp/package-hook-ready'
            os.mkfifo(ready_path)
            ready = os.open(ready_path, os.O_RDONLY | os.O_NONBLOCK)
            proc = subprocess.Popen(
                ['/installer/new', 'install', 'file:///fixtures/timeout'],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                env=environment | {'HOOK_READY_FIFO': ready_path, 'DD_INSTALLER_PACKAGE_HOOK_TIMEOUT': '1h'},
            )
            try:
                assert select.select([ready], [], [], 10)[0], 'hook did not reach cancellation handshake'
                assert os.read(ready, 16) == b'ready'
                proc.send_signal(signal.SIGINT)
                stdout, stderr = proc.communicate(timeout=20)
                assert proc.returncode != 0, (stdout, stderr)
                assert 'context canceled' in stderr, stderr
                assert_processes_stopped()
                print(json.dumps({'cancellation_exit_code': proc.returncode, 'stdout': stdout, 'stderr': stderr}))
            finally:
                os.close(ready)
                if proc.poll() is None:
                    proc.kill()
                    proc.wait()
        elif scenario == 'output':
            result = install('output', ok=False)
            assert 'stdout-must-not-be-captured' not in result.stdout + result.stderr
            assert 'a' * 32 not in result.stderr
            assert len(result.stderr) < 20000, len(result.stderr)
            assert 'truncated' in result.stderr
            spans = [span for span in intake.spans() if span.get('metrics', {}).get('exit_code') == 17]
            assert spans and spans[0]['duration'] > 0, intake.spans()
            assert len(spans[0]['meta']['hook.stderr']) <= 16384
        elif scenario == 'experiment':
            install('owned-v1')
            run('install-experiment', 'file:///fixtures/owned-v2')
            run('remove-experiment', APM)
            run('install-experiment', 'file:///fixtures/owned-v2')
            run('promote-experiment', APM)
            run('remove', APM)
            assert_events(
                [
                    ('1.0.0', 'preInstall'),
                    ('1.0.0', 'postInstall'),
                    ('1.0.0', 'preStartExperiment'),
                    ('2.0.0', 'postStartExperiment'),
                    ('2.0.0', 'preStopExperiment'),
                    ('1.0.0', 'postStopExperiment'),
                    ('1.0.0', 'preStartExperiment'),
                    ('2.0.0', 'postStartExperiment'),
                    ('1.0.0', 'prePromoteExperiment'),
                    ('2.0.0', 'postPromoteExperiment'),
                    ('2.0.0', 'preRemove'),
                ]
            )
        elif scenario == 'config-extension':
            install('owned-v1')
            operations = json.dumps({'deployment_id': 'hooks-e2e', 'file_operations': []})
            run('install-config-experiment', APM, operations)
            run('remove-config-experiment', APM)
            run('install-config-experiment', APM, operations)
            run('promote-config-experiment', APM)
            run('extension', 'install', 'file:///fixtures/owned-v1', 'fixture-extension')
            run('extension', 'remove', APM, 'fixture-extension')
            run('remove', APM)
            actual = assert_events(
                [
                    ('1.0.0', event)
                    for event in (
                        'preInstall',
                        'postInstall',
                        'postStartConfigExperiment',
                        'preStopConfigExperiment',
                        'postStartConfigExperiment',
                        'postPromoteConfigExperiment',
                        'preInstallExtension',
                        'postInstallExtension',
                        'preRemoveExtension',
                        'preRemove',
                    )
                ]
            )
            assert [event['context']['extension'] for event in actual[6:9]] == ['fixture-extension'] * 3
        elif scenario == 'old-bundled-installer':
            install('agent-v1')
            run('install-experiment', 'file:///fixtures/agent-v2')
            run('remove-experiment', AGENT)
            run('remove', AGENT)
            assert_events(
                [
                    ('1.0.0', 'preInstall'),
                    ('1.0.0', 'postInstall'),
                    ('1.0.0', 'preStartExperiment'),
                    ('2.0.0', 'postStartExperiment'),
                    ('2.0.0', 'preStopExperiment'),
                    ('1.0.0', 'postStopExperiment'),
                    ('1.0.0', 'preRemove'),
                ]
            )
        else:
            raise AssertionError(f'unknown scenario {scenario}')
        print(
            json.dumps({'scenario': scenario, 'result': 'PASS', 'events': events(), 'telemetry_spans': intake.spans()})
        )
    finally:
        intake.close()


def run_host(args):
    installer = Path(args.installer).resolve(strict=True)
    baseline = Path(args.baseline_installer).resolve(strict=True)
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    docker = ['docker'] + (['--host', args.docker_host] if args.docker_host else [])
    image = json.loads(subprocess.check_output([*docker, 'image', 'inspect', args.image]))[0]
    # Resolve locally and always use the immutable image ID. Do not pull an image or
    # reconfigure the user's Docker context as a side effect of running a test.
    image_id = image['Id']
    architecture = image['Architecture']
    scenarios = [
        'matrix-old-old',
        'matrix-old-new',
        'matrix-new-old',
        'matrix-new-new',
        'empty',
        'post-only',
        'upgrade',
        'mixed-upgrade',
        'fail-pre',
        'fail-upgrade-pre',
        'fail-post',
        'fail-remove',
        'directory-file',
        'directory-symlink',
        'event-symlink',
        'not-executable',
        'timeout',
        'inherited-pipe',
        'cancellation',
        'output',
        'experiment',
        'config-extension',
        'old-bundled-installer',
    ]
    if args.scenario:
        scenarios = args.scenario.split(',')
    summary = {
        'image': image_id,
        'architecture': architecture,
        'installer_sha256': hashlib.sha256(installer.read_bytes()).hexdigest(),
        'baseline_sha256': hashlib.sha256(baseline.read_bytes()).hexdigest(),
        'scenarios': [],
    }
    with tempfile.TemporaryDirectory(prefix='installer-package-hooks-', dir=output) as temporary:
        fixtures = Path(temporary)
        make_fixtures(fixtures, architecture, baseline)
        subprocess.run(
            [
                'openssl',
                'req',
                '-x509',
                '-newkey',
                'rsa:2048',
                '-nodes',
                '-days',
                '1',
                '-subj',
                '/CN=instrumentation-telemetry-intake.hooks.test',
                '-addext',
                'subjectAltName=DNS:instrumentation-telemetry-intake.hooks.test',
                '-keyout',
                str(fixtures / 'telemetry.key'),
                '-out',
                str(fixtures / 'telemetry.crt'),
            ],
            check=True,
            capture_output=True,
        )
        for scenario in scenarios:
            name = f'installer-package-hooks-{uuid.uuid4().hex[:12]}'
            command = [
                *docker,
                'run',
                '--rm',
                '--init',
                '--name',
                name,
                '--label',
                'com.datadoghq.test=installer-package-hooks',
                '--network',
                'none',
                '--cap-drop',
                'ALL',
                '--security-opt',
                'no-new-privileges',
                '--pids-limit',
                '256',
                '--memory',
                '1g',
                '--cpus',
                '2',
                '--add-host',
                'instrumentation-telemetry-intake.hooks.test:127.0.0.1',
                '--env',
                'DD_PACKAGE_HOOKS_E2E=disposable',
                '--mount',
                f'type=bind,src={installer},dst=/installer/new,readonly',
                '--mount',
                f'type=bind,src={baseline},dst=/installer/old,readonly',
                '--mount',
                f'type=bind,src={fixtures},dst=/fixtures,readonly',
                '--mount',
                f'type=bind,src={Path(__file__).resolve()},dst=/package_hooks.py,readonly',
                image_id,
                'python3',
                '/package_hooks.py',
                '--inside',
                scenario,
            ]
            started = time.monotonic()
            print(f'Running {scenario} ({image_id})', flush=True)
            try:
                result = subprocess.run(command, text=True, capture_output=True, timeout=240)
            except subprocess.TimeoutExpired as error:
                # Preserve diagnostics even when the outer safety deadline fires.
                stdout = error.stdout or b''
                stderr = error.stderr or b''
                result = subprocess.CompletedProcess(
                    command,
                    124,
                    stdout.decode(errors='replace'),
                    stderr.decode(errors='replace') + '\nContainer scenario exceeded 240s\n',
                )
            finally:
                # This exact randomly named container is the only cleanup target.
                subprocess.run([*docker, 'rm', '-f', name], capture_output=True, check=False)
            (output / f'{scenario}.log').write_text(result.stdout + result.stderr)
            summary['scenarios'].append(
                {
                    'scenario': scenario,
                    'exit_code': result.returncode,
                    'duration_seconds': time.monotonic() - started,
                    'command': command,
                }
            )
            (output / 'summary.json').write_text(json.dumps(summary, indent=2))
            print(f'{scenario}: {"PASS" if result.returncode == 0 else "FAIL"}', flush=True)
            if result.returncode:
                print(result.stdout + result.stderr)
                raise SystemExit(result.returncode)
    print(json.dumps({'result': 'PASS', 'scenarios': len(scenarios), 'summary': str(output / 'summary.json')}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--inside', help=argparse.SUPPRESS)
    parser.add_argument('--installer')
    parser.add_argument('--baseline-installer')
    parser.add_argument('--image')
    parser.add_argument('--docker-host')
    parser.add_argument('--output')
    parser.add_argument('--scenario')
    args = parser.parse_args()
    if args.inside:
        run_inside(args.inside)
    else:
        if not all((args.installer, args.baseline_installer, args.image, args.output)):
            parser.error('--installer, --baseline-installer, --image and --output are required')
        run_host(args)


if __name__ == '__main__':
    main()
