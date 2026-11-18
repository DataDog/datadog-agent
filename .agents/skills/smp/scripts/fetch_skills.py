#!/usr/bin/env python3
"""Fetch the SMP skill collection for the current checkout."""

import argparse
import hashlib
import json
import subprocess
import sys
import tempfile
from pathlib import Path

MARKETPLACE_URL = 'git@github.com:ddoghq/claude-marketplace.git'
PLUGIN_PATHS = {'common': 'smp/common', 'datadog_agent': 'smp/datadog-agent'}
ENTRY_SKILLS = (
    'smp/common/skills/using-smp/SKILL.md',
    'smp/datadog-agent/skills/investigating-datadog-agent-performance/SKILL.md',
)


def git(*args, cwd=None):
    result = subprocess.run(['git', *args], cwd=cwd, text=True, capture_output=True, timeout=120, check=False)
    if result.returncode:
        raise RuntimeError(result.stderr.strip() or result.stdout.strip() or 'Git command failed')
    return result.stdout.strip()


def update_checkout(checkout, ref):
    if Path(git('rev-parse', '--show-toplevel', cwd=checkout)).resolve() != checkout.resolve():
        raise RuntimeError(f'Not a dedicated skill cache: {checkout}')
    if git('config', '--get', 'remote.origin.url', cwd=checkout) != MARKETPLACE_URL:
        raise RuntimeError(f'Unexpected skill source in {checkout}')
    if git('status', '--porcelain', '--untracked-files=all', cwd=checkout):
        raise RuntimeError(f'The skill cache has local changes: {checkout}')
    git('fetch', '--filter=blob:none', '--depth=1', 'origin', ref, cwd=checkout)
    revision = git('rev-parse', 'FETCH_HEAD', cwd=checkout)
    for entry in ENTRY_SKILLS:
        git('cat-file', '-e', f'{revision}:{entry}', cwd=checkout)
    git('checkout', '--quiet', '--detach', revision, cwd=checkout)
    return revision


def fetch_skills(ref, cache_root):
    git('check-ref-format', '--branch', ref)
    cache_root.mkdir(parents=True, exist_ok=True)
    checkout = cache_root / hashlib.sha256(ref.encode()).hexdigest()[:16]
    if checkout.is_symlink():
        raise RuntimeError(f'The skill cache must not be a symbolic link: {checkout}')
    if checkout.exists():
        revision = update_checkout(checkout, ref)
    else:
        with tempfile.TemporaryDirectory(prefix='.fetch-', dir=cache_root) as temporary:
            candidate = Path(temporary) / 'checkout'
            git('init', '--quiet', str(candidate))
            git('remote', 'add', 'origin', MARKETPLACE_URL, cwd=candidate)
            git('config', 'remote.origin.promisor', 'true', cwd=candidate)
            git('config', 'remote.origin.partialclonefilter', 'blob:none', cwd=candidate)
            git('sparse-checkout', 'init', '--cone', cwd=candidate)
            git('sparse-checkout', 'set', *PLUGIN_PATHS.values(), cwd=candidate)
            revision = update_checkout(candidate, ref)
            candidate.rename(checkout)
    return {
        'ref': ref,
        'revision': revision,
        **{name: str((checkout / path).resolve()) for name, path in PLUGIN_PATHS.items()},
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--ref', default='main', help='Marketplace branch or tag (default: main)')
    args = parser.parse_args()
    cache_root = Path(__file__).resolve().parents[4] / '.cache' / 'smp-skills'
    try:
        result = fetch_skills(args.ref, cache_root)
    except (OSError, RuntimeError, subprocess.TimeoutExpired) as error:
        print(f'Could not fetch SMP guidance: {error}', file=sys.stderr)
        return 1
    print(json.dumps(result, indent=2))
    return 0


if __name__ == '__main__':
    sys.exit(main())
