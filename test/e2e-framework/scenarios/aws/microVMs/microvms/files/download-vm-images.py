#!/usr/bin/env python

import json
import subprocess
import sys
from pathlib import Path


def usage():
    print(f"Usage: {sys.argv[0]} <data-file>")


def main(data_file):
    with open(data_file) as f:
        download_data = json.load(f)

    images_to_download = []

    for image in download_data:
        checksum_file = Path(image["checksum_path"])
        if not check_integrity(checksum_file):
            print(f"Integrity check failed for checksum file: {checksum_file}, downloading image")
            images_to_download.append(image)

    curl_args = [
        "curl",
        "--no-progress-meter",
        "--fail",
        "--show-error",
        "--retry",
        "3",
        "--parallel",
        "-w",
        "'file: %{url_effective}'\n",  # Better output so we know which file actually failed
        "--parallel-max",  # Default is 50, we want just one thread per download
        str(len(images_to_download)),
    ]
    for image in images_to_download:
        source, path = image["image_source"], image["image_path"]
        csum_source, csum_path = image["checksum_source"], image["checksum_path"]
        print(f"Downloading image: {source} -> {path}")
        curl_args += [source, "-o", path]
        print(f"Downloading checksum: {csum_source} -> {csum_path}")
        curl_args += [csum_source, "-o", csum_path]

    try:
        subprocess.run(curl_args, check=True)
    except subprocess.CalledProcessError:
        print("Failed to download images")
        sys.exit(1)

    failed_integrity = False
    for image in images_to_download:
        csum_path = Path(image["checksum_path"])
        if not check_integrity(csum_path):
            print(f"Integrity check from {csum_path} failed")
            failed_integrity = True

    if failed_integrity:
        print("Some images failed integrity check")
        sys.exit(1)


def check_integrity(checksum_file: Path) -> bool:
    checksum_dir = checksum_file.parent

    try:
        subprocess.run(["sha256sum", "--strict", "--check", checksum_file], cwd=checksum_dir, check=True)
        return True
    except subprocess.CalledProcessError:
        return False


if __name__ == "__main__":
    if len(sys.argv) != 2:
        usage()
        sys.exit(1)

    if sys.argv[1] in ("-h", "--help"):
        usage()
        sys.exit(0)

    main(sys.argv[1])
