# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

"""Invoke-free artifact inventories. No output discovery by recency, no cache."""

import hashlib
import json
import os
import platform
import re
import subprocess
import tempfile
from pathlib import Path


def invalidate(path):
    if path:
        Path(path).unlink(missing_ok=True)


def file_record(path):
    path = Path(path).absolute()
    if path.is_symlink() or not path.is_file():
        raise ValueError(f"Not a regular artifact: {path}")
    with path.open('rb') as file:
        digest = hashlib.file_digest(file, 'sha256').hexdigest()
    return {"path": str(path), "mode": path.stat().st_mode & 0o777, "sha256": digest}


def tree_record(root):
    root = Path(root).resolve(strict=True)
    files = []
    for path in sorted(root.rglob('*')):
        if path.is_symlink():
            target = os.readlink(path)
            resolved = path.resolve(strict=True)
            if os.path.isabs(target) or not resolved.is_relative_to(root):
                raise ValueError(f"Unsafe artifact symlink: {path}")
            record = {"path": str(path.relative_to(root)), "mode": path.lstat().st_mode & 0o777, "link": target}
        elif path.is_file():
            record = file_record(path)
            record['path'] = str(path.relative_to(root))
        elif path.is_dir():
            continue
        else:
            raise ValueError(f"Unsupported runtime entry: {path}")
        files.append(record)
    if not files:
        raise ValueError(f"Empty artifact tree: {root}")
    return {"root": str(root), "files": files}


def source_provenance(producer):
    """Hash tracked content (including edits), with names; not a dirty boolean.

    Untracked source is included, generated/ignored build outputs are excluded.
    This is provenance, not a reusable cache key or reproducibility claim.
    """
    names = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard']).split(
        b'\0'
    )
    digest = hashlib.sha256()
    for name in sorted(set(names) - {b'', b'.e2ectl-artifact-build.lock'}):
        path = Path(os.fsdecode(name))
        digest.update(name + b'\0')
        if path.is_symlink():
            digest.update(os.fsencode(os.readlink(path)))
        elif path.is_file():
            with path.open('rb') as file:
                digest.update(hashlib.file_digest(file, 'sha256').digest())
        else:
            digest.update(b'<missing>')
    return {
        "producer": producer,
        "commit": subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip(),
        "sourceSHA256": digest.hexdigest(),
    }


def target():
    arch = {'x86_64': 'amd64', 'aarch64': 'arm64', 'arm64': 'arm64'}.get(platform.machine().lower())
    if (
        platform.system() != 'Linux'
        or arch is None
        or os.getenv('GOOS', 'linux') != 'linux'
        or os.getenv('GOARCH', arch) != arch
    ):
        raise ValueError('Artifact export currently requires native Linux amd64/arm64')
    return {"os": "linux", "arch": arch}


def write_result(path, result):
    if result['schema'] != 1 or sum(key in result for key in ('binary', 'image', 'package')) != 1:
        raise ValueError('Result requires schema 1 and one payload')
    parent = Path(path).absolute().parent
    parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(mode='w', dir=parent, delete=False) as file:
        try:
            json.dump(result, file, indent=2)
            file.flush()
            os.fsync(file.fileno())
            os.replace(file.name, path)
        finally:
            Path(file.name).unlink(missing_ok=True)


def binary_result(binary, runtime, assets, provenance):
    runtime = str(Path(runtime).resolve(strict=True))
    if not runtime.endswith('/dev/embedded'):
        raise ValueError('Binary artifact export requires the Bazel dev/embedded runtime')
    if (
        provenance.get('producer') != 'invoke-binary'
        or not re.fullmatch(r'[a-f0-9]{64}', provenance.get('sourceSHA256', ''))
        or provenance.get('options', {}).get('runtimeLayout') != 'bazel-embedded-absolute-prefix'
    ):
        raise ValueError(
            'Binary capability evidence requires the native core-source producer and observed source content'
        )
    return {
        "schema": 1,
        "target": target(),
        "provenance": provenance,
        # Trusted native core-source attestation; file/runtime inventories bind
        # the evidence to this output. It is not inferred from a version/tag.
        "profile": {"RouteContract": "agent-outbound-v1", "Roles": ["core-agent"]},
        "binary": {
            "executable": file_record(binary),
            "runtime": tree_record(runtime),
            "runtimePrefix": runtime,
            "assets": tree_record(assets),
        },
    }


def validate_image_reference(reference):
    if not re.fullmatch(
        r'[a-zA-Z0-9.-]+(:[0-9]+)?/[a-zA-Z0-9/._-]+(:[a-zA-Z0-9_][a-zA-Z0-9._-]*|@sha256:[a-f0-9]{64})', reference
    ):
        raise ValueError('Artifact export requires registry-qualified image references')


def image_record(reference):
    images = json.loads(subprocess.check_output(['docker', 'image', 'inspect', reference], text=True))
    image = images[0]
    return {"reference": reference, "id": image['Id']}, {"os": image['Os'], "arch": image['Architecture']}


def package_result(path, provenance):
    name, version, arch = subprocess.check_output(
        ['dpkg-deb', '-f', str(path), 'Package', 'Version', 'Architecture'], text=True
    ).splitlines()
    fields = [line.split(': ', 1)[-1] for line in (name, version, arch)]
    name, version, arch = fields
    if name != 'datadog-agent' or Path(path).suffix != '.deb':
        raise ValueError('Expected datadog-agent DEB output')
    dependencies = subprocess.check_output(['dpkg-deb', '-f', str(path), 'Depends'], text=True).strip()
    contents = subprocess.check_output(['dpkg-deb', '--contents', str(path)], text=True)
    paths = {line.split()[-1] for line in contents.splitlines() if line.startswith('-')}
    roles = ['agent'] if './opt/datadog-agent/bin/agent/agent' in paths else []
    if not roles:
        raise ValueError('DEB does not contain the core Agent executable')
    roles.extend(
        role
        for role in (
            'trace-agent',
            'process-agent',
            'security-agent',
            'system-probe',
            'installer',
            'trace-loader',
            'privateactionrunner',
        )
        if './opt/datadog-agent/embedded/bin/' + role in paths
    )
    return {
        "schema": 1,
        "target": {"os": "linux", "arch": arch},
        "provenance": provenance,
        "package": {
            "file": file_record(path),
            "format": "deb",
            "name": name,
            "version": version,
            "roles": roles,
            "dependencies": dependencies,
        },
    }
