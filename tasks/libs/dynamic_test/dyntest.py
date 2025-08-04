"""
Tooling to allow us to dynamically run tests that are impacted by a change
"""

import json
import os
import pathlib

from invoke import Context

from tasks.libs.common.color import Color, color_message


class CoverageDynTestUploader:
    def __init__(self, s3_path: str, coverage_folder: str):
        """
        Inputs:
            - coverage_folder (str): Should be a path to a folder containing coverage information with the following architecture:
                coverage_folder/
                    <1 folder per test suite>/
                        metadata.json
                        coverage/
            - s3_path (str): S3 where the index should be uploaded
        """
        self.s3_path = s3_path
        self.coverage_folder = coverage_folder

    def _convert_coverage_folder_to_txt(self, ctx: Context, folder_path: str) -> str:
        """Convert coverage folder to text format and return the path to the text file."""
        coverage_folder = os.path.join(folder_path, "coverage")
        coverage_txt_file = os.path.join(folder_path, "coverage.txt")

        if not os.path.exists(coverage_folder):
            print(color_message(f"No coverage folder found in {folder_path}", Color.ORANGE))
            return None

        ctx.run(f"go tool covdata textfmt -i={coverage_folder} -o={coverage_txt_file}", echo=True)
        return coverage_txt_file

    def _read_metadata(self, folder_path: str) -> dict[str, str]:
        """Read metadata.json file and return the metadata."""
        metadata_file = os.path.join(folder_path, "metadata.json")

        if not os.path.exists(metadata_file):
            print(color_message(f"No metadata.json found in {folder_path}", Color.ORANGE))
            return {}

        try:
            with open(metadata_file) as f:
                metadata = json.load(f)
                return metadata
        except (OSError, json.JSONDecodeError) as e:
            print(color_message(f"Error reading metadata.json in {folder_path}: {e}", Color.ORANGE))
            return {}

    def _parse_coverage_file(self, coverage_txt_file: str) -> set[str]:
        """Parse coverage.txt file and extract covered packages."""
        covered_packages = set()

        if not os.path.exists(coverage_txt_file):
            return covered_packages

        try:
            with open(coverage_txt_file) as f:
                for line in f:
                    # Coverage format: mode: set
                    # github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe.go:24.13,25.2 2 1
                    # We extract the package path from the file path
                    if line.startswith('mode:'):
                        continue

                    # Extract package from file path
                    # Example: github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe.go:24.13,25.2 2 1
                    parts = line.split(':')
                    if len(parts) >= 2:
                        file_path = parts[0]
                        coverage_part = parts[1]

                        if coverage_part.strip().split(" ")[-1] == "0":
                            continue

                        # Extract package from file path
                        # Remove the .go extension and get the directory
                        if file_path.endswith('.go'):
                            file_path = file_path[:-3]  # Remove .go extension

                        # Split by '/' and get the package path
                        path_parts = file_path.split('/')
                        if len(path_parts) >= 3:  # At least github.com/DataDog/datadog-agent/pkg/...
                            # Get the package path starting from the module
                            package_path = '/'.join(path_parts[2:])  # Skip github.com/DataDog
                            covered_packages.add(package_path)

        except OSError as e:
            print(color_message(f"Error reading coverage file {coverage_txt_file}: {e}", Color.ORANGE))

        return covered_packages

    def compute_index(self, ctx: Context) -> tuple[str, str]:
        """
        Compute the coverage index mapping packages to test suites.

        Returns:
            - str: Path to the index file
            - str: Path to the metadata file
        """
        coverage_path = pathlib.Path(self.coverage_folder)

        if not coverage_path.exists():
            raise FileNotFoundError(f"Coverage folder {self.coverage_folder} does not exist")

        if not coverage_path.is_dir():
            raise ValueError(f"{self.coverage_folder} is not a directory")

        # Index structure: {package_path: [test_suites]}
        package_to_tests_index = {}
        test_suite_metadata = {}

        print(color_message(f"Processing coverage folders in {self.coverage_folder}", Color.GREEN))

        # Process each subdirectory in the coverage output directory
        for folder in coverage_path.iterdir():
            if not folder.is_dir():
                continue

            folder_path = str(folder)
            folder_name = folder.name

            print(color_message(f"Processing coverage folder: {folder_name}", Color.GREEN))

            # Read metadata
            metadata = self._read_metadata(folder_path)
            job_name = metadata.get('job_name', folder_name)

            # Store test suite metadata
            test_suite_metadata[job_name] = {}

            # Convert coverage folder to text format
            coverage_txt_file = self._convert_coverage_folder_to_txt(ctx, folder_path)
            if not coverage_txt_file:
                continue

            # Parse coverage file to get covered packages
            covered_packages = self._parse_coverage_file(coverage_txt_file)

            print(color_message(f"Found {len(covered_packages)} packages covered by {folder_name}", Color.GREEN))
            if covered_packages and job_name not in package_to_tests_index:
                package_to_tests_index[job_name] = {}
            # Update the index
            for package in covered_packages:
                if package not in package_to_tests_index:
                    package_to_tests_index[job_name][package] = []
                package_to_tests_index[job_name][package].append(folder_name)

        # Create index file
        index_file = os.path.join(self.coverage_folder, "package_to_tests_index.json")
        with open(index_file, 'w') as f:
            json.dump(package_to_tests_index, f, indent=2)

        # Create metadata file
        metadata_file = os.path.join(self.coverage_folder, "metadata.json")
        with open(metadata_file, 'w') as f:
            json.dump(test_suite_metadata, f, indent=2)

        print(
            color_message(
                f"Created index with {len(package_to_tests_index)} packages mapped to test suites", Color.GREEN
            )
        )
        print(color_message(f"Index file: {index_file}", Color.GREEN))
        print(color_message(f"Metadata file: {metadata_file}", Color.GREEN))

        return index_file, metadata_file

    def upload_index(self, ctx: Context, index_file: str, metadata_file: str):
        if "S3_PERMANENT_ARTIFACTS_URI" not in os.environ:
            raise ValueError("S3_PERMANENT_ARTIFACTS_URI is not set")

        if "CI_JOB_ID" not in os.environ:
            raise ValueError("CI_JOB_ID is not set")

        if "CI_COMMIT_SHA" not in os.environ:
            raise ValueError("CI_COMMIT_SHA is not set")

        s3_path = self.s3_path
        job_id = os.environ["CI_JOB_ID"]
        commit_sha = os.environ["CI_COMMIT_SHA"]

        ctx.run(f"aws s3 cp {index_file} {s3_path}/dynamic_test/{commit_sha[:8]}/{job_id}/index.json")
        ctx.run(f"aws s3 cp {metadata_file} {s3_path}/dynamic_test/{commit_sha[:8]}/{job_id}/metadata.json --recursive")
