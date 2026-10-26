"""Read package metadata without installing packages or running their scripts."""

import io
import re
import subprocess
import tarfile
from pathlib import Path

DEB_RELATIONS = {
    'depends',
    'pre-depends',
    'recommends',
    'suggests',
    'enhances',
    'breaks',
    'conflicts',
    'replaces',
    'provides',
}
RPM_RELATIONS = {'requires', 'provides', 'conflicts', 'obsoletes'}
DEB_FIELDS = (
    'package',
    'version',
    'architecture',
    'vendor',
    'maintainer',
    'section',
    'priority',
    'homepage',
    'license',
    'description',
    'installed-size',
    *sorted(DEB_RELATIONS),
)
RPM_FIELDS = (
    'name',
    'epoch',
    'version',
    'release',
    'arch',
    'vendor',
    'license',
    'group',
    'url',
    'packager',
    'summary',
    'description',
)


def parse_deb_control(control: str) -> dict[str, str]:
    fields = {}
    key = None
    for line in control.splitlines():
        if line.startswith((' ', '\t')) and key:
            fields[key] += '\n' + line[1:]
        elif ':' in line:
            key, value = line.split(':', 1)
            key = key.lower()
            fields[key] = value.strip()
    return fields


def read_deb_metadata(package: Path) -> dict:
    control = parse_deb_control(
        subprocess.check_output(['dpkg-deb', '--field', str(package)], text=True, encoding='utf-8')
    )
    result = {key: control.get(key, '') for key in DEB_FIELDS}
    # Read the control archive directly: no extraction of package-provided paths.
    archive = subprocess.check_output(['dpkg-deb', '--ctrl-tarfile', str(package)])
    with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
        for member in tar:
            name = member.name.removeprefix('./')
            if member.isfile() and name in {
                'preinst',
                'postinst',
                'prerm',
                'postrm',
                'config',
                'triggers',
                'conffiles',
            }:
                result[name] = tar.extractfile(member).read().decode('utf-8')
    for name in ('preinst', 'postinst', 'prerm', 'postrm', 'config', 'triggers', 'conffiles'):
        result.setdefault(name, '')
    return result


def read_rpm_metadata(package: Path) -> dict:
    result = {}
    # Delimit field values explicitly; Summary is allowed to contain colons.
    values = subprocess.check_output(
        ['rpm', '-qp', '--queryformat', '\x1f'.join(f'%{{{key.upper()}}}' for key in RPM_FIELDS), str(package)],
        text=True,
        encoding='utf-8',
    )
    parts = values.split('\x1f')
    if len(parts) != len(RPM_FIELDS):
        raise ValueError(f'Invalid RPM header for {package}')
    result.update(zip(RPM_FIELDS, parts, strict=True))
    for field in (*sorted(RPM_RELATIONS), 'scripts', 'triggers', 'filetriggers', 'configfiles'):
        result[field] = subprocess.check_output(['rpm', '-qp', f'--{field}', str(package)], text=True, encoding='utf-8')
    return result


def normalize_path(path: str) -> str:
    path = path.removeprefix('./').lstrip('/')
    # The installer symlink path includes the full pipeline-stamped build version.
    return re.sub(r'(opt/datadog-packages/run/[^/]+/)[^/]+', r'\1<version>', path)


def normalize_metadata(metadata: dict, package_format: str) -> dict:
    result = {}
    for key, raw_value in metadata.items():
        value = raw_value.strip()
        if value == '(none)':
            value = ''
        if key == 'epoch':
            value = value or '0'
        elif key in ('architecture', 'arch'):
            value = {'x86_64': 'amd64', 'aarch64': 'arm64'}.get(value, value)
        else:
            if key in DEB_RELATIONS and package_format == 'deb':
                entries = [re.sub(r'\s+', ' ', v.strip()) for v in value.split(',') if v.strip()]
            elif key in RPM_RELATIONS:
                entries = [v.strip() for v in value.splitlines() if v.strip()]
            else:
                entries = None
            if entries is not None:
                value = sorted(set(entries))
            else:
                if key in {'conffiles', 'configfiles'} or (key == 'triggers' and package_format == 'deb'):
                    value = sorted({normalize_path(v.strip()) for v in value.splitlines() if v.strip()})
                elif key not in {
                    'preinst',
                    'postinst',
                    'prerm',
                    'postrm',
                    'config',
                    'scripts',
                    'triggers',
                    'filetriggers',
                }:
                    value = ' '.join(value.split())
        result[key] = value
    return result


def compare_metadata(previous: dict, current: dict, *, installed_size_tolerance: float = 0.1) -> dict:
    """Return differing normalized fields with their full omnibus/Bazel values."""
    differences = {}
    for field in sorted(previous.keys() | current.keys()):
        old, new = previous.get(field, ''), current.get(field, '')
        if old == new:
            continue
        if field == 'installed-size' and str(old).isdigit() and str(new).isdigit():
            if abs(int(old) - int(new)) <= int(old) * installed_size_tolerance:
                continue
        differences[field] = {'omnibus': old, 'bazel': new}
    return differences
