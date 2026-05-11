#!/usr/bin/env python3
"""Read a tarball and emit a file of the md5 checksums of all plain files in it.

Usage: generate_md5sum.py [input.tar[.gz|.tgz|.xz|.bz2]] <output>

If no input tar is provided, reads from stdin.
"""

import hashlib
import sys
import tarfile


def _open_tar(path):
    """Open a tar archive, selecting the decompression mode from the file extension."""
    if path.endswith('.gz') or path.endswith('.tgz'):
        return tarfile.open(path, 'r|gz')
    if path.endswith('.xz'):
        return tarfile.open(path, 'r|xz')
    if path.endswith('.bz2'):
        return tarfile.open(path, 'r|bz2')
    return tarfile.open(path, 'r|')


def generate_md5sums(tar_path, output_path):
    if tar_path is None:
        tf = tarfile.open(fileobj=sys.stdin.buffer, mode='r|*')
    else:
        tf = _open_tar(tar_path)

    with tf, open(output_path, 'w') as out:
        for member in tf:
            if not member.isfile():
                continue

            path = member.name.lstrip('./')

            f = tf.extractfile(member)
            if f is None:
                continue

            h = hashlib.md5()
            with f:
                while True:
                    chunk = f.read(65536)
                    if not chunk:
                        break
                    h.update(chunk)

            out.write(f"{h.hexdigest()}  {path}\n")


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
