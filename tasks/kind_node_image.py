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
def build_images(_, versions):
    """
    Build kind node images for specified versions.

    This task processes new versions and builds Docker images.
    The images are built locally but not pushed.

    Args:
        versions: JSON string or dict of new versions from fetch-versions
                      (e.g., '{"v1.35.0-rc.1": {"tag": "v1.35.0-rc.1", "rc": true}}')

    Outputs (GitHub Actions):
        built_count: Number of images built
        built_images: Space delimited list of built image tags
    """
    # Parse if it's a JSON string
    if isinstance(versions, str):
        try:
            versions = json.loads(versions)
        except json.JSONDecodeError as e:
            raise Exit(f"Invalid JSON in new_versions argument: {e}", code=1) from e

    built_images = []

    for tag, version_data in versions.items():
        try:
            image = build_kind_node_image(tag)
            built_images.append(image)
            print(f"Successfully built: {image}")

        except Exception as e:
            print(f"Error building image {tag}: {e}")
            raise

    # Set GitHub Actions outputs
    _set_github_output('built_count', str(len(built_images)))
    _set_github_output('built_images', ' '.join(built_images))

    if built_images:
        print(f"\nSuccessfully built {len(built_images)} image(s): {', '.join(built_images)}")
    else:
        print("\nNo images to build")
