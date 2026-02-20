import json
import os
import shutil
import subprocess

from invoke.exceptions import Exit
from invoke.tasks import task

try:
    import requests
except ImportError:
    requests = None

try:
    import boto3
except ImportError:
    boto3 = None

GITHUB_URL_BASE = "https://api.github.com"
IMAGE_NAME = "kindest/node"

KIND = 'kind'
DOCKER = 'docker'


def _check_dependencies():
    """Check if required dependencies are installed."""
    if requests is None:
        raise Exit(
            "Missing required dependencies: requests\n" "Install with: pip install requests",
            code=1,
        )


def _check_environment():
    missing = []
    if shutil.which(KIND) is None:
        missing.append(KIND)
    if shutil.which(DOCKER) is None:
        missing.append(DOCKER)
    if missing:
        raise Exit(
            f"Missing required binaries: {', '.join(missing)}",
            code=1,
        )


def get_github_rc_releases() -> list[dict[str, str]]:
    """Get RC releases from Kubernetes GitHub repository."""
    releases = []

    # Get the last 100 releases
    # TODO(TBD): Should we fetch all releases or is the last 100 enough?
    url = f"{GITHUB_URL_BASE}/repos/kubernetes/kubernetes/releases?per_page=100&page=1"

    try:
        response = requests.get(url, timeout=30)
        response.raise_for_status()

        for release in response.json():
            tag_name = release.get('tag_name', '')
            # Only return rc tags to build the kind image
            if 'rc' in tag_name:
                releases.append({'tag_name': tag_name})

    except requests.exceptions.RequestException as e:
        raise Exit(f"Error fetching releases from Github: {e}", code=1) from e

    return releases


@task
def get_rc_releases(_) -> list[dict[str, str]]:
    """Get RC releases from Kubernetes GitHub repository (Invoke task wrapper)."""
    return get_github_rc_releases()


def build_kind_node_image(version: str) -> str:
    """Build a kind node image from a Kubernetes version.

    Args:
        version: Kubernetes version tag (e.g., v1.35.0-rc.1)

    Returns:
        The built image name
    """
    _check_dependencies()
    _check_environment()

    if not version:
        raise Exit("Missing version", code=1)

    # Build the kind node image
    image = f"{IMAGE_NAME}:{version}"
    print(f"Building kind node image: {image}")

    try:
        subprocess.run(
            [KIND, 'build', 'node-image', '--image', image, version], capture_output=True, text=True, check=True
        )
        print(f"Successfully built kind node image: {image}")
    except subprocess.CalledProcessError:
        raise Exit("Failed to build kind node image", code=1) from None

    return image


@task
def build_kind_from_version(_, version):
    """Build a kind node image from Kubernetes version (Invoke task wrapper).

    Args:
        version: Kubernetes version (e.g., v1.35.0-rc.1)
    """
    return build_kind_node_image(version)


def _set_github_output(name: str, value: str) -> None:
    """Set a GitHub Actions output variable."""
    github_output = os.getenv('GITHUB_OUTPUT')
    if github_output:
        with open(github_output, 'a') as f:
            f.write(f"{name}={value}\n")
    else:
        print(f"::set-output name={name}::{value}")


@task
def build_rc_images(_, versions):
    """
    Build kind node images for RC versions.

    This task processes new versions and builds Docker images for any RC versions.
    The images are built locally but not pushed.

    Args:
        versions: JSON string or dict of new versions from fetch-versions
                      (e.g., '{"v1.35.0-rc.1": {"tag": "v1.35.0-rc.1", "rc": true}}')

    Outputs (GitHub Actions):
        built_count: Number of RC images built
        built_tags: Space delimited list of built image tags
    """
    # Parse if it's a JSON string
    if isinstance(versions, str):
        try:
            versions = json.loads(versions)
        except json.JSONDecodeError as e:
            raise Exit(f"Invalid JSON in new_versions argument: {e}", code=1) from e

    built_images = []

    for tag, version_data in versions.items():
        if version_data.get('rc'):
            try:
                image = build_kind_node_image(tag)
                built_images.append(image)
                print(f"Successfully built: {image}")

            except Exception as e:
                print(f"Error building RC image {tag}: {e}")
                raise
        else:
            print(f"Skipping non-RC version: {tag}")

    # Set GitHub Actions outputs
    _set_github_output('built_count', str(len(built_images)))
    _set_github_output('built_images', ' '.join(built_images))

    if built_images:
        print(f"\nSuccessfully built {len(built_images)} RC image(s): {', '.join(built_images)}")
    else:
        print("\nNo RC images to build")
