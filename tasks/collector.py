import os
import platform
import re
import shutil
import subprocess
import tempfile
import urllib.request

import requests
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
OCB_VERSION = "0.108.0"

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

MANIFEST_FILE = "./comp/otelcol/collector-contrib/impl/manifest.yaml"


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

def versions_equal(version1, version2):
    # strip leading 'v' if present
    if version1.startswith('v'):
        version1 = version1[1:]
    if version2.startswith('v'):
        version2 = version2[1:]
    # Split the version strings by '.'
    parts1 = version1.split('.')
    parts2 = version2.split('.')
    # Compare the first two parts (major and minor versions)
    return parts1[0] == parts2[0] and parts1[1] == parts2[1]

def validate_manifest(manifest) -> list:
    """Return a list of components to remove, or empty list if valid.
    If invalid components are found, raise a YAMLValidationError."""

    # validate collector version matches ocb version
    manifest_version = manifest.get("dist", {}).get("otelcol_version")
    if manifest_version and not versions_equal(manifest_version, OCB_VERSION):
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
                    print(f"module: {module}")
                    module_version = module.split(" ")[1]
                    if not versions_equal(module_version, OCB_VERSION):
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
        run_command = f"{binary_path} --config {MANIFEST_FILE} --skip-compilation"
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
        with open(MANIFEST_FILE) as file:
            manifest = yaml.safe_load(file)
            output_path = manifest["dist"]["output_path"]
            components_to_remove = validate_manifest(manifest)
    except Exception as e:
        raise Exit(
            color_message(f"Failed to read manifest file: {e}", Color.RED),
            code=1,
        ) from e

    if components_to_remove:
        strip_invalid_components(MANIFEST_FILE, components_to_remove)

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


def fetch_core_module_versions(version):
    """
    Fetch versions.yaml from the provided URL and build a map of modules with their versions.
    """
    url = f"https://raw.githubusercontent.com/open-telemetry/opentelemetry-collector/{version}/versions.yaml"
    print(f"Fetching versions from {url}")

    try:
        response = requests.get(url)
        response.raise_for_status()  # Raises an HTTPError if the HTTP request returned an unsuccessful status code
    except requests.exceptions.RequestException as e:
        raise Exit(
            color_message(f"Failed to fetch the YAML file: {e}", Color.RED),
            code=1,
        ) from e

    yaml_content = response.content

    try:
        data = yaml.safe_load(yaml_content)
    except yaml.YAMLError as e:
        raise Exit(
            color_message(f"Failed to parse YAML content: {e}", Color.RED),
            code=1,
        ) from e

    version_modules = {}
    for _, details in data.get("module-sets", {}).items():
        version = details.get("version", "unknown")
        for module in details.get("modules", []):
            version_modules[version] = version_modules.get(version, []) + [module]
    return version_modules


def update_go_mod_file(go_mod_path, collector_version_modules):
    print(f"Updating {go_mod_path}")
    # Read all lines from the go.mod file
    with open(go_mod_path) as file:
        lines = file.readlines()

    updated_lines = []
    file_updated = False  # To check if the file was modified

    # Compile a regex for each module to match the module name exactly
    compiled_modules = {
        module: re.compile(rf"^\s*{re.escape(module)}\s+v[\d\.]+")
        for _, modules in collector_version_modules.items()
        for module in modules
    }
    for line in lines:
        updated_line = line
        for version, modules in collector_version_modules.items():
            for module in modules:
                module_regex = compiled_modules[module]
                match = module_regex.match(line)
                if match:
                    print(f"Updating {module} to version {version} in {go_mod_path}")
                    updated_line = f"{match.group(0).split()[0]} {version}\n"
                    file_updated = True
                    break  # Stop checking other modules once we find a match
            if updated_line != line:
                break  # If the line was updated, stop checking other versions
        updated_lines.append(updated_line)

    # Write the updated lines back to the file only if changes were made
    if file_updated:
        with open(go_mod_path, "w") as file:
            file.writelines(updated_lines)
        print(f"{go_mod_path} updated.")
    else:
        print(f"No changes made to {go_mod_path}.")


def update_all_go_mod(collector_version_modules):
    for root, _, files in os.walk("."):
        if "go.mod" in files:
            go_mod_path = os.path.join(root, "go.mod")
            update_go_mod_file(go_mod_path, collector_version_modules)
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
    collector_version = fetch_latest_release(repo)
    if collector_version:
        print(f"Latest release for {repo}: {collector_version}")
        version_modules = fetch_core_module_versions(collector_version)
        update_all_go_mod(version_modules)
        old_version = read_old_version(MANIFEST_FILE)
        if old_version:
            collector_version = collector_version[1:]
            update_file(MANIFEST_FILE, old_version, collector_version)
            update_file(
                "./comp/otelcol/collector/impl/collector.go",
                old_version,
                collector_version,
            )
            update_file("./tasks/collector.py", old_version, collector_version)
            for root, _, files in os.walk("./tasks/unit_tests/testdata/collector"):
                for file in files:
                    update_file(
                        os.path.join(root, file), old_version, collector_version
                    )

    else:
        print(f"Failed to fetch the latest release for {repo}")

    print("Core collector update complete.")


def update_versions_in_yaml(yaml_file_path, new_version, component_prefix):
    with open(yaml_file_path) as file:
        data = yaml.safe_load(file)

    # Function to update versions in a list of components
    def update_component_versions(components):
        for i, component in enumerate(components):
            if "gomod" in component and component_prefix in component["gomod"]:
                parts = component["gomod"].split(" ")
                if len(parts) == 2:
                    parts[1] = new_version
                    components[i]["gomod"] = " ".join(parts)

    # Update extensions, receivers, processors, and exporters
    for key in ["extensions", "receivers", "processors", "exporters", "connectors"]:
        if key in data:
            update_component_versions(data[key])

    with open(yaml_file_path, "w") as file:
        yaml.dump(data, file, default_flow_style=False)

    print(
        f"Updated YAML file at {yaml_file_path} with new version {new_version} for components matching '{component_prefix}'."
    )


def update_collector_contrib():
    print("Updating the collector-contrib version in all go.mod files.")
    repo = "open-telemetry/opentelemetry-collector-contrib"
    modules = ["github.com/open-telemetry/opentelemetry-collector-contrib"]
    collector_version = fetch_latest_release(repo)
    if collector_version:
        print(f"Latest release for {repo}: {collector_version}")
        version_modules = {
            collector_version: modules,
        }
        update_all_go_mod(version_modules)
        update_versions_in_yaml(
            MANIFEST_FILE,
            collector_version,
            "github.com/open-telemetry/opentelemetry-collector-contrib",
        )

    else:
        print(f"Failed to fetch the latest release for {repo}")
    print("Collector-contrib update complete.")


@task
def update(ctx):
    update_core_collector()
    update_collector_contrib()
    print("Update complete.")
