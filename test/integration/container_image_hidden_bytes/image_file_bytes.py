#!/usr/bin/env python3
"""Read one `docker image save` archive on stdin; report its logical file-data size."""

import hashlib
import json
import shutil
import sys
import tarfile
import tempfile


def image_file_bytes(archive):
    manifest = json.load(archive.extractfile("manifest.json"))
    if len(manifest) != 1:
        raise ValueError("expected one platform image")
    image = manifest[0]
    config = archive.extractfile(image["Config"]).read()
    total = 0
    for layer_name in image["Layers"]:
        with archive.extractfile(layer_name) as layer_file, tarfile.open(fileobj=layer_file, mode="r|*") as layer:
            for entry in layer:
                # Hardlink aliases carry no new data; whiteouts describe deletion.
                if entry.isfile() and not entry.name.rsplit("/", 1)[-1].startswith(".wh."):
                    total += entry.size
    return {"image_id": "sha256:" + hashlib.sha256(config).hexdigest(), "uncompressed_size": total}


def main():
    # Docker may place the manifest after the layers. Spool without extracting
    # archive paths or holding file contents in memory; remove on exit.
    with tempfile.TemporaryFile() as saved_image:
        shutil.copyfileobj(sys.stdin.buffer, saved_image)
        saved_image.seek(0)
        with tarfile.open(fileobj=saved_image) as archive:
            print(json.dumps(image_file_bytes(archive)))


if __name__ == "__main__":
    main()
