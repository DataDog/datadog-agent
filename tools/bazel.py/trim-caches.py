#!/usr/bin/env python3
import os
import stat
import sys
from datetime import datetime, timedelta
from pathlib import Path

_MAX_AGE = timedelta(days=7)  # covers extended weekends
_THROTTLE = timedelta(hours=1)  # a single cleanup per hour should be enough


def _trim(cutoff, *caches):
    for cache in caches:
        print(f"{__file__}: trimming {cache}...", file=sys.stderr)
        try:
            for entry in Path(os.environ[cache]).rglob("*"):
                try:
                    s = entry.stat()
                except FileNotFoundError:  # file (re)moved in-between iterations
                    print(f"{__file__}: {entry} disappeared", file=sys.stderr)
                    continue
                # trim only regular files, considering the most recent of their access and modification times
                if stat.S_ISREG(s.st_mode) and datetime.fromtimestamp(max(s.st_atime, s.st_mtime)) < cutoff:
                    print(f"{__file__}: trimming {entry}...", file=sys.stderr)
                    entry.unlink(missing_ok=True)
        except (FileNotFoundError, PermissionError) as e:  # (sub)dir absent, gone mid-scan, or Windows-locked
            print(f"{__file__}: trimming canceled by {e}", file=sys.stderr)
            continue


def main(last_trim, now):
    if last_trim.exists() and now - datetime.fromtimestamp(last_trim.stat().st_mtime) < _THROTTLE:
        return

    try:
        last_trim.touch()
    except PermissionError:  # possible on Windows if 2+ scripts touch concurrently
        print(f"{__file__}: trimming left to concurrent script", file=sys.stderr)
        return

    _trim(now - _MAX_AGE, "GOCACHE", "GOMODCACHE")
    if sys.platform != "linux":
        sys.exit("success")


if __name__ == "__main__":
    main(Path(os.environ["XDG_CACHE_HOME"]) / "last-trim", datetime.now())
