"""Read package metadata without installing packages or running their scripts."""

import io
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
    # Buffer only metadata/scripts, not the package payload; no extraction of package-provided paths.
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
