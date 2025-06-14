import tempfile
import xml.etree.ElementTree as ET

import requests
from invoke.context import Context
from invoke.exceptions import Exit

from tasks.libs.common.download import download

DEB_TESTING_BUCKET_URL = "https://apttesting.datad0g.com"
RPM_TESTING_BUCKET_URL = "https://yumtesting.datad0g.com"


def get_rpm_package_url(ctx: Context, pipeline_id: int, package_name: str, arch: str):
    arch2 = "x86_64" if arch == "amd64" else "aarch64"
    packages_url = f"{RPM_TESTING_BUCKET_URL}/testing/pipeline-{pipeline_id}-a7/7/{arch2}"

    repomd_url = f"{packages_url}/repodata/repomd.xml"
    response = requests.get(repomd_url, timeout=None)
    response.raise_for_status()
    repomd = ET.fromstring(response.text)

    primary = next((data for data in repomd.findall('.//{*}data') if data.get('type') == 'primary'), None)
    assert primary is not None, f"Could not find primary data in {repomd_url}"
    location = primary.find('{*}location')
    assert location is not None, f"Could not find location for primary data in {repomd_url}"

    filename = tempfile.mktemp()
    primary_url = f"{packages_url}/{location.get('href')}"
    download(primary_url, filename)
    res = ctx.run(f"gunzip --stdout {filename}", hide=True)
    assert res

    primary = ET.fromstring(res.stdout.strip())
    for package in primary.findall('.//{*}package'):
        if package.get('type') != 'rpm':
            continue
        name = package.find('{*}name')
        if name is None or name.text != package_name:
            continue
        location = package.find('{*}location')
        assert location is not None, f"Could not find location for {package_name} in {primary_url}"
        return f"{packages_url}/{location.get('href')}"
    raise Exit(code=1, message=f"Could not find package {package_name} in {primary_url}")


def get_deb_package_url(_: Context, pipeline_id: int, package_name: str, arch: str):
    arch2 = arch
    if arch == "amd64":
        arch2 = "x86_64"

    packages_url = f"{DEB_TESTING_BUCKET_URL}/dists/pipeline-{pipeline_id}-a7-{arch2}/7/binary-{arch}/Packages"

    filename = _deb_get_filename_for_package(packages_url, package_name)
    return f"{DEB_TESTING_BUCKET_URL}/{filename}"


def _deb_get_filename_for_package(packages_url: str, target_package_name: str) -> str:
    response = requests.get(packages_url, timeout=None)
    response.raise_for_status()

    packages = [
        f"Package:{content}" if not content.startswith("Package:") else content
        for content in response.text.split("\nPackage:")
    ]

    for package in packages:
        package_name = None
        package_filename = None
        for line in package.split('\n'):
            match line.split(': ')[0]:
                case "Package":
                    package_name = line.split(': ', 1)[1]
                    continue
                case "Filename":
                    package_filename = line.split(': ', 1)[1]
                    continue

        if target_package_name == package_name:
            if package_filename is None:
                raise Exit(code=1, message=f"Could not find filename for {target_package_name} in {packages_url}")
            return package_filename

    raise Exit(code=1, message=f"Could not find filename for {target_package_name} in {packages_url}")
