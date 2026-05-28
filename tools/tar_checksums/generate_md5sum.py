#!/usr/bin/env python3
"""Read a tarball and emit a file of the md5 checksums of all plain files in it.

Usage: generate_md5sum.py [input.tar[.gz|.tgz|.xz|.bz2]] <output>

If no input tar is provided, reads from stdin.


Requirements:
Read a .tar file (which might be compressed) and emit a file containing the md5 checksum of each file within that tar file.

- must take the path to the input tar file as a command line arg
  - if no input tar is provided, then use stdin as the file
- must take the path to the output file as a command line arg
- sample of the desired output format:  md5_sum path
```
e3c6a486a70a471110731b1708d232cc  opt/datadog-installer/LICENSE
f9a6f2aa44430e18abbc7363751e3f7c  opt/datadog-installer/LICENSES/THIRD-PARTY-0BSD
3b83ef96387f14655fc854ddc3c6bd57  opt/datadog-installer/LICENSES/THIRD-PARTY-Apache-2.0
11d3feb7137319430849e84dbc75ac27  opt/datadog-installer/LICENSES/THIRD-PARTY-BSD-2-Clause
```
- emitted paths must be relative, with no preceding "./"
- directories and symlinks in the tar file should be ignored.
- support different compression algorithms that are used in our product
  - we do not have to decode the compression from the binary itself, we can use the file name as a hint
  - required for first implementation:  XZ compression, if the file ends in .xz,  gzip compression if the file ends in .gz or .tgz.
"""

import hashlib
import sys
import tarfile


def _open_tar(path):
    """Open a tar archive, selecting the decompression mode from the file extension."""
    if path is None:
        return tarfile.open(fileobj=sys.stdin.buffer, mode='r|*')
    return tarfile.open(path, 'r|*')


def generate_md5sums(tar_path, output_path):
    with open(output_path, 'w') as out:
        with _open_tar(tar_path) as tf:
            for member in tf:
                if not member.isfile():
                    continue

                path = member.name.removeprefix('./')
                f = tf.extractfile(member)
                if f is None:
                    continue

                digest = hashlib.file_digest(f, 'md5').hexdigest()
                out.write(f"{digest}  {path}\n")


def main():
    if len(sys.argv) == 2:
        generate_md5sums(None, sys.argv[1])
    elif len(sys.argv) == 3:
        generate_md5sums(sys.argv[1], sys.argv[2])
    else:
        print(f"Usage: {sys.argv[0]} [input.tar[.gz|.tgz|.xz|.bz2]] <output>", file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()
