# SPDX-FileCopyrightText: 2024-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: BSD-3-Clause
from __future__ import annotations

import os
import pathlib
import sys
from contextlib import contextmanager
from functools import cached_property
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from collections.abc import Generator

    from _typeshed import FileDescriptorLike

# There is special recognition in Mypy for `sys.platform`, not `os.name`
# https://github.com/python/cpython/blob/09d7319bfe0006d9aa3fc14833b69c24ccafdca6/Lib/pathlib.py#L957
if sys.platform == 'win32':
    _PathBase = pathlib.WindowsPath
else:
    _PathBase = pathlib.PosixPath

disk_sync = os.fsync
# https://mjtsai.com/blog/2022/02/17/apple-ssd-benchmarks-and-f_fullsync/
# https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/fsync.2.html
if sys.platform == 'darwin':
    import fcntl

    if hasattr(fcntl, 'F_FULLFSYNC'):

        def disk_sync(fd: FileDescriptorLike) -> None:
            fcntl.fcntl(fd, fcntl.F_FULLFSYNC)


# TODO: Inherit directly from pathlib.Path when we upgrade to Python 3.12
# https://docs.python.org/3/whatsnew/3.12.html#pathlib
class Path(_PathBase):
    @cached_property
    def long_id(self) -> str:
        """
        Returns a unique identifier for the current path.
        """
        from base64 import urlsafe_b64encode
        from hashlib import sha256

        path = str(self)
        # Handle case-insensitive filesystems
        if sys.platform == 'win32' or sys.platform == 'darwin':
            path = path.casefold()

        digest = sha256(path.encode('utf-8')).digest()
        return urlsafe_b64encode(digest).decode('utf-8')

    @cached_property
    def id(self) -> str:
        return self.long_id[:8]

    def ensure_dir(self) -> None:
        self.mkdir(parents=True, exist_ok=True)

    def expand(self) -> Path:
        return Path(os.path.expanduser(os.path.expandvars(self)))

    def write_atomic(self, data: str | bytes, *args: Any, **kwargs: Any) -> None:
        from tempfile import mkstemp

        fd, path = mkstemp(dir=self.parent)
        with os.fdopen(fd, *args, **kwargs) as f:
            f.write(data)
            f.flush()
            disk_sync(fd)

        os.replace(path, self)

    @contextmanager
    def as_cwd(self) -> Generator[Path, None, None]:
        origin = os.getcwd()
        os.chdir(self)

        try:
            yield self
        finally:
            os.chdir(origin)


@contextmanager
def temp_directory() -> Generator[Path, None, None]:
    from tempfile import TemporaryDirectory

    with TemporaryDirectory() as d:
        yield Path(d).resolve()
