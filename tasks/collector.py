import os
import platform
import subprocess
import tempfile
import urllib.request

from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.go import generate_licenses, tidy
from tasks.libs.common.color import Color, color_message

LICENSE_HEADER = """// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
"""
OCB_VERSION = "v0.103.1"


@task(post=[tidy, generate_licenses])
def generate(ctx):
    arch = platform.machine()
    system = platform.system()
    base_url = (
        f"https://github.com/open-telemetry/opentelemetry-collector/releases/download/cmd%2Fbuilder%2F{OCB_VERSION}/"
    )

    if system == "Linux":
        if arch == "x86_64":
            binary_name = "ocb_0.103.1_linux_amd64"
        elif arch == "arm64" or arch == "aarch64":
            binary_name = "ocb_0.103.1_linux_arm64"
        else:
            print(f"Unsupported architecture: {arch}")
            return
    elif system == "Darwin":
        if arch == "x86_64":
            binary_name = "ocb_0.103.1_darwin_amd64"
        elif arch == "arm64":
            binary_name = "ocb_0.103.1_darwin_arm64"
        else:
            print(f"Unsupported architecture: {arch}")
            return
    else:
        print(f"Unsupported system: {system}")
        return

    binary_url = f"{base_url}{binary_name}"

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
        config_path = "./comp/otelcol/collector-contrib/impl/manifest.yaml"
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

    # Clean the files with main* in comp/otelcol/collector-contrib/impl
    impl_path = "./comp/otelcol/collector-contrib/impl"
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
