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
from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.constants import (
    GITHUB_REPO_NAME,
)
from tasks.libs.common.git import (
    check_clean_branch_state,
    check_uncommitted_changes,
)
from tasks.libs.common.utils import running_in_ci

LICENSE_HEADER = """// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
"""
OCB_VERSION = "0.129.0"

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


def versions_equal(version1, version2, fuzzy=False):
    idx = version1.find("/")
    if idx != -1:
        # version may be in the format of "v1.xx.0/v0.yyy.0"
        version1 = version1[idx + 1 :]
    # strip leading 'v' if present
    if version1.startswith("v"):
        version1 = version1[1:]
    if version2.startswith("v"):
        version2 = version2[1:]
    # Split the version strings by '.'
    parts1 = version1.split(".")
    parts2 = version2.split(".")

    # Compare the first two parts (major and minor versions)
    major_minor_match = parts1[0] == parts2[0] and parts1[1] == parts2[1]

    if fuzzy:
        # If fuzzy matching is enabled, check if major and minor match
        return major_minor_match
    else:
        # Otherwise, check all parts for exact match
        return major_minor_match and parts1 == parts2


def validate_manifest(manifest) -> list:
    """Return a list of components to remove, or empty list if valid.
    If invalid components are found, raise a YAMLValidationError."""

    # validate collector version matches ocb version
    manifest_version = manifest.get("dist", {}).get("otelcol_version")
    if manifest_version and not versions_equal(manifest_version, OCB_VERSION, True):
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
                    module_info = module.split(" ")
                    if len(module_info) == 2:
                        _, module_version = module_info
                        if not versions_equal(module_version, OCB_VERSION, True):
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
                content = content.replace("package main", "package collectorcontribimpl")

                with open(file_path, "w") as f:
                    f.write(content)
                ctx.run(f"gofmt -l -s -w {file_path}")
                print(f"Updated package name and ensured license header in: {file_path}")


def update_go_mod_file(go_mod_path, module_versions):
    print(f"Updating {go_mod_path}")
    # Read all lines from the go.mod file
    with open(go_mod_path) as file:
        lines = file.readlines()

    updated_lines = []
    file_updated = False  # To check if the file was modified

    # Compile a regex for each module to match the module name exactly
    compiled_modules = {module: re.compile(rf"^\s*{re.escape(module)}\s+v[\d\.]+") for module in module_versions.keys()}

    # Regex to match any `require` line
    require_regex = re.compile(r"^require\s+(\S+)\s+v[\d\.]+")

    for line in lines:
        updated_line = line

        # Check for any `require` line case
        require_match = require_regex.match(line)
        if require_match:
            module_name = require_match.group(1)
            for module, version in module_versions.items():
                if module_name == module:
                    print(f"Updating {module_name} to version {version} in {go_mod_path}")
                    updated_line = f"require {module_name} {version}\n"
                    file_updated = True
                    break  # Stop checking once updated

        # General case for other module versions
        else:
            for module, version in module_versions.items():
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


OTEL_COLLECTOR_CORE_REPO = "open-telemetry/opentelemetry-collector"
OTEL_COLLECTOR_CONTRIB_REPO = "open-telemetry/opentelemetry-collector-contrib"


def update_versions_in_ocb_yaml(yaml_file_path, modules_version):
    with open(yaml_file_path) as file:
        data = yaml.safe_load(file)

    # Function to update versions in a list of components
    def update_component_versions(components):
        for i, component in enumerate(components):
            if "gomod" in component:
                parts = component["gomod"].split(" ")
                if len(parts) == 2:
                    version = modules_version.get(parts[0], parts[1])
                    parts[1] = version
                    components[i]["gomod"] = " ".join(parts)

    # Update extensions, receivers, processors, and exporters
    for key in ["extensions", "receivers", "processors", "exporters", "connectors", "providers", "converters"]:
        if key in data:
            update_component_versions(data[key])

    with open(yaml_file_path, "w") as file:
        yaml.dump(data, file, default_flow_style=False)

    print(f"Updated YAML file at {yaml_file_path}")


def fetch_latest_release(repo):
    gh = GithubAPI(repo)
    return gh.latest_release()


class CollectorRepo:
    def __init__(self, repo):
        self.repo = repo
        self.version = self.fetch_latest_release()
        if not self.version:
            raise Exit(
                color_message(f"Failed to fetch the latest release for {repo}", Color.RED),
                code=1,
            )
        self.version_modules = self.fetch_module_versions()
        self.old_version = read_old_version(MANIFEST_FILE)
        if not self.old_version:
            raise Exit(
                color_message(f"Failed to read the old version from {MANIFEST_FILE}", Color.RED),
                code=1,
            )
        self.modules_version = {}
        for k, v in self.version_modules.items():
            for module in v:
                self.modules_version[module] = k

    def get_old_version(self):
        return self.old_version

    def get_modules_version(self):
        return self.modules_version

    def get_version(self):
        return self.version

    def fetch_latest_release(self):
        gh = GithubAPI(self.repo)
        self.version = gh.latest_release_tag()
        return self.version

    def fetch_module_versions(self):
        url = f"https://raw.githubusercontent.com/{self.repo}/refs/tags/{self.version}/versions.yaml"
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


class CollectorVersionUpdater:
    def __init__(self):
        self.core_collector = CollectorRepo(OTEL_COLLECTOR_CORE_REPO)
        self.contrib_collector = CollectorRepo(OTEL_COLLECTOR_CONTRIB_REPO)
        self.modules_version = {}
        self.modules_version.update(self.core_collector.get_modules_version())
        self.modules_version.update(self.contrib_collector.get_modules_version())

    def update_all_go_mod(self):
        update_all_go_mod(self.modules_version)

    def update_ocb_yaml(self):
        update_versions_in_ocb_yaml(
            "./test/otel/testdata/builder-config.yaml",
            self.modules_version,
        )
        update_versions_in_ocb_yaml(
            MANIFEST_FILE,
            self.modules_version,
        )

    def update_files(self):
        files = [
            MANIFEST_FILE,
            "./comp/otelcol/collector/impl/collector.go",
            "./tasks/collector.py",
            "./.gitlab/integration_test/otel.yml",
            "./test/otel/testdata/ocb_build_script.sh",
        ]
        for root, _, testfiles in os.walk("./tasks/unit_tests/testdata/collector"):
            for file in testfiles:
                files.append(os.path.join(root, file))
        collector_version = self.core_collector.get_version()[1:]
        os.environ["OCB_VERSION"] = collector_version
        for file in files:
            update_file(file, self.core_collector.get_old_version(), collector_version)

    def update(self):
        self.update_all_go_mod()
        self.update_ocb_yaml()
        self.update_files()
        print("Update complete.")


@task(post=[tidy])
def update(_):
    updater = CollectorVersionUpdater()
    updater.update()
    print("Update complete.")


@task
def pull_request(ctx):
    # This task should only be run locally
    if not running_in_ci():
        raise Exit(
            f"[{color_message('ERROR', Color.RED)}] This task should only be run locally.",
            code=1,
        )
    # Perform Git operations
    ctx.run('git add .')
    if check_uncommitted_changes(ctx):
        branch_name = f"update-otel-collector-dependencies-{OCB_VERSION}"
        gh = GithubAPI(repository=GITHUB_REPO_NAME)
        ctx.run(f'git switch -c {branch_name}')
        ctx.run(
            f'git commit -m "Update OTel Collector dependencies to {OCB_VERSION} and generate OTel Agent" --no-verify'
        )
        try:
            # don't check if local branch exists; we just created it
            check_clean_branch_state(ctx, gh, branch_name)
        except Exit as e:
            # local branch already exists, so skip error if this is thrown
            if "already exists locally" not in str(e):
                print(e)
                return
        ctx.run(f'git push -u origin {branch_name} --no-verify')  # skip pre-commit hook if installed locally
        gh.create_pr(
            pr_title=f"Update OTel Collector dependencies to v{OCB_VERSION}",
            pr_body=f"This PR updates the dependencies of the OTel Collector to v{OCB_VERSION} and generates the OTel Agent code.",
            target_branch=branch_name,
            base_branch="main",
            draft=True,
        )
    else:
        print("No changes detected, skipping PR creation.")
