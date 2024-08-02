import requests


def get_latest_version(repository, namespace="datadog") -> str:
    response = requests.get(f"https://hub.docker.com/v2/namespaces/{namespace}/repositories/{repository}/tags")
    response.raise_for_status()

    return sorted(
        [result["name"] for result in response.json()["results"] if result["name"] != "latest"], reverse=True
    )[0]
