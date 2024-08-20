import os
import platform
import shutil
import subprocess
from sys import modules
import tempfile
import urllib.request
import requests
import re

import yaml
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.go import tidy
from tasks.libs.common.color import Color, color_message


LICENSE_HEADER = """// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
"""
OCB_VERSION = "0.107.0"

MANDATORY_COMPONENTS = {
    "extensions": [
        "zpagesextension",
        "healthcheckextension",
        "pprofextension",
    ],
    "receivers": [
        "prometheusreceiver",
    ],
}

COMPONENTS_TO_STRIP = {
    "connectors": [
        "datadogconnector",
    ],
    "exporters": [
        "datadogexporter",
    ],
    "receivers": [
        "awscontainerinsightreceiver",
    ],
}

BASE_URL = f"https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/cmd%2Fbuilder%2Fv{OCB_VERSION}/"

BINARY_NAMES_BY_SYSTEM_AND_ARCH = {
    "Linux": {
        "x86_64": f"ocb_{OCB_VERSION}_linux_amd64",
        "arm64": f"ocb_{OCB_VERSION}_linux_arm64",
        "aarch64": f"ocb_{OCB_VERSION}_linux_arm64",
    },
    "Darwin": {
        "x86_64": f"ocb_{OCB_VERSION}_darwin_amd64",
        "arm64": f"ocb_{OCB_VERSION}_darwin_arm64",
    },
}


class YAMLValidationError(Exception):
    def __init__(self, message):
        super().__init__(message)


def find_matching_components(
    manifest, components_to_match: dict, present: bool
) -> list:
    """Given a manifest and dict of components to match, if present=True, return list of
    components found, otherwise return list of components missing."""
    res = []
    for component_type, components in components_to_match.items():
        for component in components:
            found_component = False
            components_matching_component_type = manifest.get(component_type)
            if components_matching_component_type:
                for module in components_matching_component_type:
                    if module.get("gomod").find(component) != -1:
                        found_component = True
                        if present:
                            res.append(component)
                        break
            if not present and not found_component:
                res.append(component)
    return res


def validate_manifest(manifest) -> list:
    """Return a list of components to remove, or empty list if valid.
    If invalid components are found, raise a YAMLValidationError."""

    # validate collector version matches ocb version
    manifest_version = manifest.get("dist", {}).get("otelcol_version")
    if manifest_version and manifest_version != OCB_VERSION:
        raise YAMLValidationError(
            f"Collector version ({manifest_version}) in manifest does not match required OCB version ({OCB_VERSION})"
        )

    # validate component versions matches ocb version
    module_types = ["extensions", "exporters", "processors", "receivers", "connectors"]
    for module_type in module_types:
        components = manifest.get(module_type)
        if components:
            for component in components:
                for module in component.values():
                    if module.find(OCB_VERSION) == -1:
                        raise YAMLValidationError(
                            f"Component {module}) in manifest does not match required OCB version ({OCB_VERSION})"
                        )

    # validate mandatory components are present
    missing_components = find_matching_components(manifest, MANDATORY_COMPONENTS, False)
    if missing_components:
        raise YAMLValidationError(
            f"Missing mandatory components in manifest: {', '.join(missing_components)}"
        )

    # determine if conflicting components are included in manifest, and if so, return list to remove
    conflicting_components = find_matching_components(
        manifest, COMPONENTS_TO_STRIP, True
    )
    return conflicting_components


def strip_invalid_components(file_path, components_to_remove):
    lines = []
    try:
        with open(file_path) as file:
            lines = file.readlines()
    except Exception as e:
        raise Exit(
            color_message(f"Failed to read manifest file: {e}", Color.RED),
            code=1,
        ) from e
    try:
        with open(file_path, "w") as file:
            for line in lines:
                if any(component in line for component in components_to_remove):
                    continue
                file.write(line)
    except Exception as e:
        raise Exit(
            color_message(f"Failed to write to manifest file: {e}", Color.RED),
            code=1,
        ) from e


@task(post=[tidy])
def generate(ctx):
    arch = platform.machine()
    system = platform.system()

    if system not in BINARY_NAMES_BY_SYSTEM_AND_ARCH:
        print(f"Unsupported system: {system}")
        return
    if arch not in BINARY_NAMES_BY_SYSTEM_AND_ARCH[system]:
        print(f"Unsupported architecture: {arch}")
        return
    binary_name = BINARY_NAMES_BY_SYSTEM_AND_ARCH[system][arch]

    binary_url = f"{BASE_URL}{binary_name}"

    config_path = "./comp/otelcol/collector-contrib/impl/manifest.yaml"
    with tempfile.TemporaryDirectory() as tmpdirname:
        binary_path = os.path.join(tmpdirname, binary_name)
        print(f"Downloading {binary_url} to {binary_path}...")

        try:
            urllib.request.urlretrieve(binary_url, binary_path)
            os.chmod(binary_path, 0o755)
            print(f"Downloaded to {binary_path}")
        except Exception as e:
            raise Exit(
                color_message("Error: Failed to download the binary", Color.RED),
                code=1,
            ) from e

        # Run the binary with specified options
        run_command = f"{binary_path} --config {config_path} --skip-compilation"
        print(f"Running command: {run_command}")

        try:
            result = ctx.run(run_command)
            print(f"Binary output:\n{result.stdout}")
        except subprocess.CalledProcessError as e:
            raise Exit(
                color_message(
                    f"Error: Failed to run the binary: {e} output:\n {e.stderr}",
                    Color.RED,
                ),
                code=1,
            ) from e

    # Read the output path from the manifest file
    impl_path = "./comp/otelcol/collector-contrib/impl"
    output_path = None
    components_to_remove = []
    try:
        with open(config_path) as file:
            manifest = yaml.safe_load(file)
            output_path = manifest["dist"]["output_path"]
            components_to_remove = validate_manifest(manifest)
    except Exception as e:
        raise Exit(
            color_message(f"Failed to read manifest file: {e}", Color.RED),
            code=1,
        ) from e

    if components_to_remove:
        strip_invalid_components(config_path, components_to_remove)

    if output_path != impl_path:
        files_to_copy = ["components.go", "go.mod"]
        for file_name in files_to_copy:
            source = os.path.join(output_path, file_name)
            dest = os.path.join(impl_path, file_name)
            print(f"Copying {source} to {dest}")
            try:
                shutil.copy(source, dest)
            except Exception as e:
                raise Exit(
                    color_message(f"Failed to copy components.go file: {e}", Color.RED),
                    code=1,
                ) from e

    # Clean the files with main* in comp/otelcol/collector-contrib/impl
    for filename in os.listdir(impl_path):
        if filename.startswith("main"):
            file_path = os.path.join(impl_path, filename)
            print(f"Removing file: {file_path}")
            os.remove(file_path)

    # Rename package main to package collectorcontribimpl and ensure license header in comp/otelcol/collector-contrib/impl
    for root, _, files in os.walk(impl_path):
        for file in files:
            if file.endswith(".go"):
                file_path = os.path.join(root, file)
                with open(file_path) as f:
                    content = f.read()

                # Ensure license header
                if not content.startswith(LICENSE_HEADER):
                    content = LICENSE_HEADER + "\n" + content

                # Rename package
                content = content.replace(
                    "package main", "package collectorcontribimpl"
                )

                with open(file_path, "w") as f:
                    f.write(content)

                print(
                    f"Updated package name and ensured license header in: {file_path}"
                )


GITHUB_API_URL = "https://api.github.com/repos"


def fetch_latest_release(repo):
    url = f"{GITHUB_API_URL}/{repo}/releases/latest"
    print(f"Fetching latest release from {url} for {repo}")
    response = requests.get(url)
    if response.status_code == 200:
        data = response.json()
        return data["tag_name"]
    else:
        return None


def update_go_mod_file(go_mod_path, collector_version, collector_modules):
    print(f"Updating {go_mod_path} with version {collector_version}")
    updated_lines = []
    with open(go_mod_path) as file:
        for line in file:
            for module in collector_modules:
                module_regex = re.compile(rf"^(\s*{re.escape(module)}\S*)\s+v[\d\.]+")
                match = module_regex.match(line)
                if match:
                    print(f"Updating {module} in {go_mod_path}")
                    # Replace the version with the new version
                    line = f"{match.group(1)} {collector_version}\n"
                updated_lines.append(line)
    # Write the updated lines back to the file
    with open(go_mod_path, "w") as file:
        file.writelines(updated_lines)


def update_all_go_mod(collector_version, collector_modules):
    for root, _, files in os.walk("."):
        if "go.mod" in files:
            go_mod_path = os.path.join(root, "go.mod")
            update_go_mod_file(go_mod_path, collector_version, collector_modules)
    print("All go.mod files updated.")


def read_old_version(filepath):
    """Reads the old version from the manifest.yaml file."""
    version_regex = re.compile(r"^\s*version:\s+([\d\.]+)")
    with open(filepath) as file:
        for line in file:
            match = version_regex.match(line)
            if match:
                return match.group(1)
    return None


def update_file(filepath, old_version, new_version):
    """Updates all instances of the old version to the new version in the file."""
    print(f"Updating all instances of {old_version} to {new_version} in {filepath}")
    with open(filepath) as file:
        content = file.read()

    # Replace all occurrences of the old version with the new version
    updated_content = content.replace(old_version, new_version)

    # Write the updated content back to the file
    with open(filepath, "w") as file:
        file.write(updated_content)

    print(f"Updated all instances of {old_version} to {new_version} in {filepath}")


def update_core_collector():
    print(
        "Updating the core collector version in all go.mod files and manifest.yaml file."
    )
    repo = "open-telemetry/opentelemetry-collector"
    modules = ["go.opentelemetry.io/collector"]
    collector_version = fetch_latest_release(repo)
    if collector_version:
        print(f"Latest release for {repo}: {collector_version}")
        update_all_go_mod(collector_version, modules)
        manifest_path = "./comp/otelcol/collector-contrib/impl/manifest.yaml"
        old_version = read_old_version(manifest_path)
        if old_version:
            update_file(manifest_path, old_version, collector_version[1:])
            update_file(
                "./comp/otelcol/collector/impl/collector.go",
                old_version,
                collector_version,
            )
            update_file("./tasks/collector.py", old_version, collector_version[1:])

    else:
        print(f"Failed to fetch the latest release for {repo}")

    print("Core collector update complete.")


def update_collector_contrib():
    print("Updating the collector-contrib version in all go.mod files.")
    repo = "open-telemetry/opentelemetry-collector-contrib"
    modules = ["github.com/open-telemetry/opentelemetry-collector-contrib"]
    collector_version = fetch_latest_release(repo)
    if collector_version:
        print(f"Latest release for {repo}: {collector_version}")
        update_all_go_mod(collector_version, modules)
    else:
        print(f"Failed to fetch the latest release for {repo}")
    print("Collector-contrib update complete.")


@task
def update(ctx):
    update_core_collector()
    update_collector_contrib()
