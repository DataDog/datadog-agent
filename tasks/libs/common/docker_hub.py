import requests
import semver
from requests.adapters import HTTPAdapter
from requests.packages.urllib3.util.retry import Retry  # type: ignore


def get_latest_version(repository: str, namespace: str = "datadog") -> str:
    session = requests.session()
    retries = Retry(total=3, backoff_factor=1)
    session.mount('https://', HTTPAdapter(max_retries=retries))

    response = session.get(f"https://hub.docker.com/v2/namespaces/{namespace}/repositories/{repository}/tags")
    response.raise_for_status()

    latest_release = semver.VersionInfo(0)

    for version in response.json()["results"]:
        name = version["name"]
        if semver.VersionInfo.isvalid(name):
            current_version = semver.VersionInfo.parse(name)
            if current_version > latest_release:
                latest_release = current_version

    return str(latest_release)
