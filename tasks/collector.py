import os
import platform
import shutil
import subprocess
import tempfile
import urllib.request

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
OCB_VERSION = "0.104.0"

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

BASE_URL = (
    f"https://github.com/open-telemetry/opentelemetry-collector/releases/download/cmd%2Fbuilder%2Fv{OCB_VERSION}/"
)

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


def find_matching_components(manifest, components_to_match: dict, present: bool) -> list:
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
        raise YAMLValidationError(f"Missing mandatory components in manifest: {', '.join(missing_components)}")

    # determine if conflicting components are included in manifest, and if so, return list to remove
    conflicting_components = find_matching_components(manifest, COMPONENTS_TO_STRIP, True)
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
                content = content.replace("package main", "package collectorcontribimpl")

                with open(file_path, "w") as f:
                    f.write(content)

                print(f"Updated package name and ensured license header in: {file_path}")
