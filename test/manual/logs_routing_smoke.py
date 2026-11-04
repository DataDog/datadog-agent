#!/usr/bin/env python3
"""TEMPORARY real-Agent routing smoke test; remove with the startup wiring."""

import argparse
import collections
import gzip
import hashlib
import http.server
import json
import os
import re
import signal
import socket
import subprocess
import tempfile
import threading
import time
import traceback
from pathlib import Path

RUN = Path(os.environ.get('ROUTING_SMOKE_RESULTS') or tempfile.mkdtemp(prefix='logs-routing-smoke-'))
RUN.mkdir(parents=True, exist_ok=True)


class Intake:
    def __init__(self, name):
        self.name, self.healthy = name, True
        self.lock, self.requests = threading.Lock(), []
        intake = self

        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def do_GET(self):
                body = (
                    json.dumps(
                        {
                            'name': intake.name,
                            'healthy': intake.healthy,
                            'accepted_ids': dict(intake.ids()),
                            'recent_messages': intake.messages()[-5:],
                        }
                    ).encode()
                    if self.path == '/status'
                    else b'{"valid":true}'
                )
                self.send_response(200)
                self.send_header('Content-Length', str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def do_POST(self):
                raw = self.rfile.read(int(self.headers.get('Content-Length', '0')))
                encoding = self.headers.get('Content-Encoding', '')
                try:
                    body = gzip.decompress(raw) if encoding == 'gzip' else raw
                    data = json.loads(body)
                    records = data if isinstance(data, list) else []
                    records = [r for r in records if isinstance(r, dict) and 'id=' in str(r.get('message', ''))]
                except Exception:
                    records = []
                with intake.lock:
                    status = 200 if intake.healthy or not records else 503
                    capture = {
                        'time': time.time(),
                        'path': self.path,
                        'status': status,
                        'encoding': encoding,
                        'encoded_bytes': len(raw),
                        'api_key': self.headers.get('DD-API-KEY'),
                        'records': records,
                    }
                    intake.requests.append(capture)
                    with (RUN / (intake.name + '-requests.jsonl')).open('a') as out:
                        out.write(json.dumps(capture) + '\n')
                self.send_response(status)
                self.send_header('Content-Length', '2')
                self.end_headers()
                try:
                    self.wfile.write(b'{}')
                except (BrokenPipeError, ConnectionResetError):
                    pass

        self.server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.port = self.server.server_address[1]

    def messages(self):
        with self.lock:
            return [r['message'] for q in self.requests if q['status'] == 200 for r in q['records']]

    def ids(self):
        return collections.Counter(m.group(1) for text in self.messages() if (m := re.search(r'\bid=([\w-]+)', text)))

    def payloads(self, prefix):
        with self.lock:
            return [
                q
                for q in self.requests
                if q['status'] == 200 and any(('id=' + prefix) in r['message'] for r in q['records'])
            ]

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()


class Smoke:
    def __init__(self, mode, binary, interactive=False):
        self.interactive = interactive
        self.mode, self.binary = mode, Path(binary).resolve()
        self.root = RUN / mode
        self.root.mkdir(exist_ok=True)
        self.a, self.b = Intake(mode + '-a'), Intake(mode + '-b')
        self.proc, self.console, self.starts = None, None, 0
        self.results = []
        self.x, self.y = self.root / 'x.log', self.root / 'y.log'
        self.x.touch()
        self.y.touch()
        for d in ['config/conf.d/routing-smoke.d', 'registry', 'core']:
            (self.root / d).mkdir(parents=True, exist_ok=True)

        def port():
            with socket.socket() as sock:
                sock.bind(('127.0.0.1', 0))
                return sock.getsockname()[1]

        conf = {
            'api_key': 'a' * 32,
            'hostname': 'routing-smoke',
            'confd_path': str(self.root / 'config/conf.d'),
            'run_path': str(self.root / 'core'),
            'auth_token_file_path': str(self.root / 'core/auth_token'),
            'cmd_port': port(),
            'ipc_port': port(),
            'expvar_port': port(),
            'dogstatsd_port': 0,
            'bind_host': '127.0.0.1',
            'use_dogstatsd': False,
            'dd_url': f'http://127.0.0.1:{self.a.port}',
            'log_level': 'info',
            'log_file': str(self.root / 'agent.log'),
            'remote_configuration': {'enabled': False},
            'telemetry': {'enabled': False},
            'apm_config': {'enabled': False},
            'enable_metadata_collection': False,
            'inventories_enabled': False,
            'process_config': {'process_collection': {'enabled': False}, 'container_collection': {'enabled': False}},
            'logs_enabled': True,
            'logs_config': {
                'force_use_http': True,
                'logs_dd_url': f'127.0.0.1:{self.a.port}',
                'logs_no_ssl': True,
                'use_compression': True,
                'compression_kind': 'gzip',
                'batch_wait': 10,
                'batch_max_size': 1000,
                'batch_max_concurrent_send': 2,
                'file_scan_period': 0.1,
                'run_path': str(self.root / 'registry'),
                'stop_grace_period': 1,
                'close_timeout': 1,
                'sender_backoff_base': 0.1,
                'sender_backoff_factor': 2,
                'sender_backoff_max': 1,
                'processing_rules': [
                    {
                        'type': 'mask_sequences',
                        'name': 'route_after_processing',
                        'pattern': 'route=RAW',
                        'replace_placeholder': 'route=A',
                    },
                    {
                        'type': 'mask_sequences',
                        'name': 'redact_secret',
                        'pattern': 'secret=unmasked',
                        'replace_placeholder': 'secret=masked',
                    },
                    {'type': 'exclude_at_match', 'name': 'drop', 'pattern': 'DROP'},
                ],
            },
        }
        (self.root / 'config/datadog.yaml').write_text(json.dumps(conf, indent=2))
        source = {
            'logs': [
                {
                    'type': 'file',
                    'path': str(p),
                    'service': 'routing-smoke',
                    'source': 'routing-smoke',
                    'start_position': 'beginning',
                }
                for p in [self.x, self.y]
            ]
        }
        (self.root / 'config/conf.d/routing-smoke.d/conf.yaml').write_text(json.dumps(source, indent=2))
        self.manifest = {
            'mode': mode,
            'binary': str(self.binary),
            'binary_sha256': hashlib.sha256(self.binary.read_bytes()).hexdigest(),
            'started': time.time(),
            'commands': [],
            'checks': self.results,
        }
        self.save()

    def pause(self, message):
        if not self.interactive:
            return
        print(
            '\nA:',
            f'http://127.0.0.1:{self.a.port}/status',
            'healthy=',
            self.a.healthy,
            'accepted=',
            sum(self.a.ids().values()),
            flush=True,
        )
        print(
            'B:',
            f'http://127.0.0.1:{self.b.port}/status',
            'healthy=',
            self.b.healthy,
            'accepted=',
            sum(self.b.ids().values()),
            flush=True,
        )
        print('X checkpoint / file bytes:', self.offset(self.x), '/', self.x.stat().st_size, flush=True)
        print('Y checkpoint / file bytes:', self.offset(self.y), '/', self.y.stat().st_size, flush=True)
        print('Results:', self.root, flush=True)
        input(message + ' [Enter] ')

    def save(self):
        (self.root / 'results.json').write_text(json.dumps(self.manifest, indent=2))

    def mark(self, name, **detail):
        self.results.append({'name': name, 'pass': True, **detail})
        self.save()
        print('PASS', self.mode, name, detail, flush=True)

    def wait(self, name, predicate, timeout=40):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if predicate():
                return
            if self.proc and self.proc.poll() is not None:
                raise AssertionError(f'Agent exited {self.proc.returncode} while waiting for {name}')
            time.sleep(0.05)
        raise AssertionError('Timed out: ' + name)

    def start(self):
        self.starts += 1
        self.console = (self.root / f'agent-{self.starts}.console.log').open('w')
        env = {k: v for k, v in os.environ.items() if not k.startswith(('DD_', 'DATADOG_', 'AWS_'))}
        if self.mode == 'routed':
            env['DD_LOG_ROUTING_SMOKE_B'] = f'127.0.0.1:{self.b.port}'
        cmd = [
            str(self.binary),
            'run',
            '--cfgpath',
            str(self.root / 'config'),
            '--pidfile',
            str(self.root / 'agent.pid'),
        ]
        self.manifest['commands'].append(cmd)
        self.save()
        self.proc = subprocess.Popen(
            cmd, cwd=self.root, env=env, stdout=self.console, stderr=subprocess.STDOUT, start_new_session=True
        )

        def ready():
            self.console.flush()
            text = (self.root / f'agent-{self.starts}.console.log').read_text(errors='replace')
            return 'logs-agent started' in text and 'Starting a new tailer' in text

        self.wait('Agent and real file tailers ready', ready, 90)
        self.mark('Agent startup', pid=self.proc.pid, start=self.starts)

    def stop(self, hard=False):
        if self.proc and self.proc.poll() is None:
            before = time.monotonic()
            self.proc.send_signal(signal.SIGKILL if hard else signal.SIGTERM)
            try:
                code = self.proc.wait(timeout=20)
            except subprocess.TimeoutExpired as err:
                self.proc.kill()
                self.proc.wait()
                raise AssertionError('Agent did not stop within 20 seconds') from err
            elapsed = time.monotonic() - before
            self.console.close()
            self.proc = None
            self.mark('Crash process' if hard else 'Bounded graceful shutdown', seconds=elapsed, exit=code)
            if not hard:
                assert code == 0, f'Graceful shutdown exit={code}'

    def append(self, path, lines):
        with path.open('a') as out:
            out.write(''.join(s + '\n' for s in lines))
            out.flush()
            os.fsync(out.fileno())
        return path.stat().st_size

    def registry(self):
        try:
            return json.loads((self.root / 'registry/registry.json').read_text())
        except (FileNotFoundError, json.JSONDecodeError):
            return {}

    def offset(self, path):
        entries = self.registry().get('Registry', {})
        entry = entries.get('file:' + str(path), {})
        return int(entry.get('Offset', 0))

    def snapshot(self, name):
        (self.root / (name + '-registry.json')).write_text(json.dumps(self.registry(), indent=2))

    def received(self, intake, ids):
        return all(intake.ids()[i] >= 1 for i in ids)

    def fault(self, prefix):
        before = self.x.stat().st_size
        self.wait('safe persisted X prefix', lambda: self.offset(self.x) == before)
        self.a.healthy = False
        ids = [f'{prefix}-{i:03d}' for i in range(96)]
        self.append(self.x, [f'route=AB id={i}' for i in ids])
        self.wait('B receives the first source window', lambda: sum(self.b.ids()[i] > 0 for i in ids) >= 64)
        assert self.offset(self.x) == before, 'B alone advanced X checkpoint'
        assert not any(self.a.ids()[i] for i in ids), 'A accepted a record during its outage'
        return before, ids

    def run(self):
        self.start()
        if self.mode == 'baseline':
            end = self.append(self.x, ['route=A id=baseline-control'])
            self.wait('default HTTP delivery', lambda: self.a.ids()['baseline-control'] > 0)
            self.wait('default persisted checkpoint', lambda: self.offset(self.x) == end)
            self.mark('Default provider delivers and persists a file checkpoint')
            self.stop()
            return
        self.pause('Agent running. Send routing and processing examples?')
        self.append(self.x, ['route=AB id=prime-x'])
        self.append(self.y, ['route=B id=prime-y'])
        self.wait(
            'priming records',
            lambda: self.received(self.a, ['prime-x']) and self.received(self.b, ['prime-x', 'prime-y']),
        )
        self.wait(
            'primed disk checkpoints',
            lambda: self.offset(self.x) == self.x.stat().st_size and self.offset(self.y) == self.y.stat().st_size,
        )
        self.mark('Real disk auditor primed')
        end = self.append(
            self.x,
            [
                'route=A id=basic-a',
                'route=B id=basic-b',
                'route=AB id=basic-ab',
                'route=NONE id=basic-excluded',
                'route=AB id=basic-dropped DROP',
                'route=RAW id=basic-processed secret=unmasked',
            ],
        )
        self.wait(
            'selective routing',
            lambda: (
                self.received(self.a, ['basic-a', 'basic-ab', 'basic-processed'])
                and self.received(self.b, ['basic-b', 'basic-ab'])
            ),
        )
        self.wait('checkpoint includes local outcomes', lambda: self.offset(self.x) == end)
        assert {i for i in self.a.ids() if i.startswith('basic-')} == {'basic-a', 'basic-ab', 'basic-processed'}
        assert {i for i in self.b.ids() if i.startswith('basic-')} == {'basic-b', 'basic-ab'}
        message = next(t for t in self.a.messages() if 'id=basic-processed ' in t)
        assert 'route=A' in message and 'secret=masked' in message and 'unmasked' not in message
        self.mark(
            'Exact recipients, exclusion, processing drop, and processed-content selection',
            A=['basic-a', 'basic-ab', 'basic-processed'],
            B=['basic-b', 'basic-ab'],
        )
        self.pause('Routing checks passed. Test batching and stop intake A?')
        ids = [f'batch-{i:03d}' for i in range(64)]
        started = time.monotonic()
        end = self.append(self.x, [f'route=A id={i}' for i in ids])
        self.wait('pressure flush of full window', lambda: self.received(self.a, ids), 9)
        elapsed = time.monotonic() - started
        payloads = self.a.payloads('batch-')
        assert len(payloads) == 1 and len(payloads[0]['records']) == 64, 'Pressure fragmented the window'
        assert payloads[0]['encoding'] == 'gzip', 'Real compressor did not send gzip'
        self.wait('pressure checkpoint persisted', lambda: self.offset(self.x) == end)
        self.mark('One gzip payload for a full 64-record window before the 10-second timer', seconds=elapsed)
        before, ids = self.fault('outage')
        yids = [f'healthy-y-{i:03d}' for i in range(128)]
        yend = self.append(self.y, [f'route=B id={i}' for i in yids])
        self.wait(
            'unrelated source advances through B', lambda: self.received(self.b, yids) and self.offset(self.y) == yend
        )
        assert self.offset(self.x) == before
        assert sum(self.b.ids()[i] > 0 for i in ids) == 64, 'X exceeded its source window'
        self.snapshot('during-a-outage')
        self.mark('A outage holds X at 64 pending while Y delivers and persists independently')
        self.pause('A is failing; X is held and Y continued through B. Recover A?')
        self.a.healthy = True
        self.wait(
            'A recovery drains X',
            lambda: (
                self.received(self.a, ids)
                and self.received(self.b, ids)
                and self.offset(self.x) == self.x.stat().st_size
            ),
            60,
        )
        self.mark('A recovery resumes X and advances its checkpoint')
        self.pause('Recovery passed. Test graceful shutdown and restart?')
        before, ids = self.fault('shutdown')
        self.stop()
        assert self.offset(self.x) == before, 'Shutdown advanced unresolved X'
        self.snapshot('after-graceful-outage-shutdown')
        self.mark('Shutdown preserves unresolved checkpoint')
        self.a.healthy = True
        self.start()
        self.wait(
            'graceful restart replay',
            lambda: self.received(self.a, ids) and self.offset(self.x) == self.x.stat().st_size,
            60,
        )
        self.mark('Restart replays retained file data after outage shutdown')
        self.pause('Restart replay passed. Test SIGKILL and crash recovery?')
        before, ids = self.fault('crash')
        self.snapshot('before-sigkill')
        self.stop(hard=True)
        assert self.offset(self.x) == before, 'Crash test did not preserve the intended disk prefix'
        self.a.healthy = True
        self.start()
        self.wait(
            'crash restart replay',
            lambda: (
                self.received(self.a, ids)
                and self.received(self.b, ids)
                and self.offset(self.x) == self.x.stat().st_size
            ),
            60,
        )
        self.snapshot('after-crash-replay')
        duplicates = sum(self.b.ids()[i] > 1 for i in ids)
        self.mark(
            'SIGKILL restart recovers retained records with safe checkpoint; B duplicates allowed',
            duplicated_b_ids=duplicates,
        )
        for intake, expected in [(self.a, 'a' * 32), (self.b, 'b' * 32)]:
            with intake.lock:
                assert all(
                    q['api_key'] == expected for q in intake.requests if q['records']
                ), 'Wrong endpoint credential'
        self.mark('Endpoint-specific credentials preserved')
        self.stop()

    def close(self):
        if self.proc and self.proc.poll() is None:
            self.proc.kill()
            self.proc.wait()
        if self.console and not self.console.closed:
            self.console.close()
        self.a.close()
        self.b.close()
        self.save()


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('mode', choices=['baseline', 'routed'])
    parser.add_argument('binary')
    parser.add_argument('--interactive', action='store_true')
    args = parser.parse_args()
    smoke = Smoke(args.mode, args.binary, args.interactive)
    try:
        smoke.run()
        smoke.manifest['pass'] = True
    except BaseException as exc:
        smoke.manifest['pass'] = False
        smoke.manifest['error'] = str(exc)
        (smoke.root / 'failure.txt').write_text(traceback.format_exc())
        traceback.print_exc()
        raise
    finally:
        smoke.close()
