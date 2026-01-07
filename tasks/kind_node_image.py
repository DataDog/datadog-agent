import json
import os
import shutil
import subprocess
import tarfile
import tempfile

from invoke.exceptions import Exit
from invoke.tasks import task

try:
    import requests
except ImportError:
    requests = None

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
            tarball_url = release.get('tarball_url')

            # Only return rc tags that have an associated tarball URL to build the kind image
            if 'rc' in tag_name and tarball_url:
                releases.append({'tag_name': tag_name, 'tarball_url': tarball_url})

    except requests.exceptions.RequestException as e:
        raise Exit(f"Error fetching releases from Github: {e}", code=1) from e

    return releases


@task
def get_rc_releases(_) -> list[dict[str, str]]:
    """Get RC releases from Kubernetes GitHub repository (Invoke task wrapper)."""
    return get_github_rc_releases()


def build_kind_node_image(tag: str, tarball_url: str) -> str:
    """Build a kind node image from a Kubernetes release tarball.

    Args:
        tag: Kubernetes version tag (e.g., v1.35.0-rc.1)
        tarball_url: URL to the Kubernetes source tarball (e.g., https://api.github.com/repos/kubernetes/kubernetes/tarball/v1.35.0-rc.1)

    Returns:
        The built image name
    """
    _check_dependencies()
    _check_environment()

    if not tag:
        raise Exit("Missing tag", code=1)
    if not tarball_url:
        raise Exit("Missing tarball URL", code=1)

    print(f"Building kind node image for {tag}")
    print(f"Downloading tarball from: {tarball_url}")

    # Create a temporary directory for the build
    with tempfile.TemporaryDirectory() as temp_dir:
        tarball_path = os.path.join(temp_dir, 'kubernetes.tar.gz')

        # Download the tarball
        try:
            response = requests.get(tarball_url, timeout=300, stream=True)
            response.raise_for_status()

            with open(tarball_path, 'wb') as f:
                for chunk in response.iter_content(chunk_size=8192):
                    f.write(chunk)

            print(f"Downloaded tarball to {tarball_path}")
        except requests.exceptions.RequestException as e:
            raise Exit(f"Error downloading tarball: {e}", code=1) from e

        # Extract the tarball
        try:
            with tarfile.open(tarball_path, 'r:gz') as tar:
                # Get the root directory name from the tarball
                # GitHub tarballs have a single root directory like kubernetes-kubernetes-<sha>
                members = tar.getmembers()
                if not members:
                    raise Exit("Tarball is empty", code=1)

                # Extract the root directory name from the first member
                # members[0].name is like "kubernetes-kubernetes-abc123/README.md"
                root_dir = members[0].name.split('/')[0]

                # Extract all files to temp_dir
                tar.extractall(path=temp_dir)

                # Build the full source directory path
                source_dir = os.path.join(temp_dir, root_dir)

            print(f"Extracted source to {source_dir}")
        except (tarfile.TarError, OSError) as e:
            raise Exit(f"Error extracting tarball: {e}", code=1) from e

        # Build the kind node image
        image = f"{IMAGE_NAME}:{tag}"
        print(f"Building kind node image: {image}")

        try:
            result = subprocess.run(
                [KIND, 'build', 'node-image', '--image', image, source_dir], capture_output=True, text=True, check=True
            )
            print(result.stdout)
            print(f"Successfully built kind node image: {image}")
        except subprocess.CalledProcessError as e:
            print(f"Error building kind node image:\n{e.stderr}")
            raise Exit(f"Failed to build kind node image: {e}", code=1) from e

        return image


@task
def build_from_source(_, tag, tarball):
    """Build a kind node image from a Kubernetes release tarball (Invoke task wrapper).

    Args:
        tag: Kubernetes version tag (e.g., v1.35.0-rc.1)
        tarball: URL to the Kubernetes source tarball
    """
    return build_kind_node_image(tag, tarball)


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
                      (e.g., '{"v1.35.0-rc.1": {"tag": "v1.35.0-rc.1", "rc": true, "tarball": "https://..."}}')

    Outputs (GitHub Actions):
        built_count: Number of RC images built
        built_tags: Comma-separated list of built image tags
    """
    # Parse if it's a JSON string
    if isinstance(versions, str):
        try:
            versions = json.loads(versions)
        except json.JSONDecodeError as e:
            raise Exit(f"Invalid JSON in new_versions argument: {e}", code=1) from e

    built_images = []

    for tag, version_data in versions.items():
        if version_data.get('rc') and version_data.get('tarball'):
            print(f"\nBuilding RC version: {tag}")
            tarball_url = version_data['tarball']

            try:
                # Build the kind node image
                image = build_kind_node_image(tag, tarball_url)
                built_images.append(image)
                print(f"Successfully built image {image}")

            except Exception as e:
                print(f"Error building RC image {tag}: {e}")
                raise
        else:
            print(f"Skipping non-RC version: {tag}")

    # Set GitHub Actions outputs
    _set_github_output('built_rc_image', 'true')
    _set_github_output('built_count', str(len(built_images)))
    _set_github_output('built_images', ','.join(built_images))

    if built_images:
        print(f"\nSuccessfully built {len(built_images)} RC image(s): {', '.join(built_images)}")
    else:
        print("\nNo RC images to build")
