import requests
from requests.adapters import HTTPAdapter
from requests.packages.urllib3.util.retry import Retry  # type: ignore


def get_latest_version(repository: str, namespace: str = "datadog") -> str:
    session = requests.session()
    retries = Retry(total=3, backoff_factor=1)
    session.mount('https://', HTTPAdapter(max_retries=retries))

    response = session.get(f"https://hub.docker.com/v2/namespaces/{namespace}/repositories/{repository}/tags")
    response.raise_for_status()

    return sorted(
        [result["name"] for result in response.json()["results"] if result["name"] != "latest"], reverse=True
    )[0]
