#!/usr/bin/env python3
"""
Test script for the Rust AI usage native host.
Sends HEALTH_CHECK and SEND_USAGE_EVENT over the native messaging framing.

Usage:
    python3 test_host.py <path_to_rust_exe> [--config=PATH ...]

If no path is provided, defaults to <repo-root>/target/release/ai-prompt-logger-native-host
(repo root is three parents above this script; .exe on Windows). Any extra arguments are forwarded
to the host (e.g. --config=...).
"""

import json
import struct
import subprocess
import sys
from pathlib import Path


def send_message(proc, message: dict) -> dict:
    """Send a message and receive response using native messaging protocol."""
    encoded = json.dumps(message).encode('utf-8')
    proc.stdin.write(struct.pack('<I', len(encoded)))
    proc.stdin.write(encoded)
    proc.stdin.flush()

    raw_length = proc.stdout.read(4)
    if len(raw_length) < 4:
        raise RuntimeError('Failed to read response length')

    response_length = struct.unpack('<I', raw_length)[0]
    response_bytes = proc.stdout.read(response_length)
    return json.loads(response_bytes.decode('utf-8'))


def test_health_check(proc):
    """Test HEALTH_CHECK message."""
    print('\n[TEST] HEALTH_CHECK')
    response = send_message(proc, {'type': 'HEALTH_CHECK'})
    print(f'  Response: {json.dumps(response, indent=2)}')

    assert response['type'] == 'HEALTH_RESULT', f'Expected HEALTH_RESULT, got {response["type"]}'
    assert response['status'] == 'ok', f'Expected status \'ok\', got {response["status"]}'
    print('  ✓ HEALTH_CHECK passed')
    return response


def test_send_usage_event(proc):
    """Test SEND_USAGE_EVENT (success depends on a reachable Agent EVP proxy)."""
    print('\n[TEST] SEND_USAGE_EVENT')
    response = send_message(
        proc,
        {
            'type': 'SEND_USAGE_EVENT',
            'tool': 'test-tool',
            'user_id': 'test-user',
            'approved': True,
        },
    )
    print(f'  Response: {json.dumps(response, indent=2)}')

    assert (
        response['type'] == 'SEND_USAGE_EVENT_RESULT'
    ), f'Expected SEND_USAGE_EVENT_RESULT, got {response.get("type")}'
    assert 'success' in response, 'Missing success field'
    print(f'  ✓ SEND_USAGE_EVENT shape OK (success={response["success"]})')
    return response


def main():
    if len(sys.argv) > 1 and sys.argv[1] in ('-h', '--help'):
        print(__doc__)
        sys.exit(0)

    if len(sys.argv) > 1:
        host_path = Path(sys.argv[1])
        extra_args = sys.argv[2:]
    else:
        exe_name = 'ai-prompt-logger-native-host.exe' if sys.platform == 'win32' else 'ai-prompt-logger-native-host'
        repo_root = Path(__file__).resolve().parents[3]
        host_path = repo_root / 'target' / 'release' / exe_name
        extra_args = []

    if not host_path.exists():
        print(f'ERROR: Host executable not found: {host_path}')
        print('Build with: cargo build -p ai-prompt-logger-native-host --release')
        sys.exit(1)

    cmd = [str(host_path)] + extra_args
    print(f'Testing native host: {cmd}')
    print('=' * 60)

    proc = subprocess.Popen(
        cmd,
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )

    try:
        test_health_check(proc)
        test_send_usage_event(proc)

        print('\n' + '=' * 60)
        print('All tests passed!')
    except Exception as e:
        print(f'\n[ERROR] Test failed: {e}')
        stderr_output = proc.stderr.read().decode('utf-8', errors='replace')
        if stderr_output:
            print(f'\nStderr output:\n{stderr_output}')
        sys.exit(1)
    finally:
        proc.terminate()
        proc.wait()


if __name__ == '__main__':
    main()
