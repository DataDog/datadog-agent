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
    try:
        with open(config_path) as file:
            manifest = yaml.safe_load(file)
            output_path = manifest["dist"]["output_path"]
    except Exception as e:
        raise Exit(
            color_message(f"Failed to read manifest file: {e}", Color.RED),
            code=1,
        ) from e

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

    # Rename package main to package collectorcontrib and ensure license header in comp/otelcol/collector-contrib/impl
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
                content = content.replace("package main", "package collectorcontrib")

                with open(file_path, "w") as f:
                    f.write(content)

                print(f"Updated package name and ensured license header in: {file_path}")
