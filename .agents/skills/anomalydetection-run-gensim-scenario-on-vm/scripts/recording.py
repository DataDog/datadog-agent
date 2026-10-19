"""Compute the final-flush wait and validate/trim a fetched gensim recording."""

import argparse
import datetime as dt
import json
import math
import shutil
import tarfile
from itertools import pairwise
from pathlib import Path, PurePosixPath


def wait_seconds(episode_file, flush):
    episode = json.loads(episode_file.read_text())
    end = dt.datetime.fromisoformat(episode["end_time"].replace("Z", "+00:00"))
    if end.tzinfo is None or end > dt.datetime.now(dt.timezone.utc):
        raise ValueError("episode end_time must be in the past and timezone-aware")
    return max(0, math.ceil(end.timestamp() + flush - dt.datetime.now(dt.timezone.utc).timestamp()))


def identity(output, filename):
    pods = json.loads((output / filename).read_text())["items"]
    if len(pods) != 1:
        raise ValueError("expected one recorder Agent pod")
    pod = pods[0]
    containers = pod.get("status", {}).get("containerStatuses", [])
    if not containers or any(not item["ready"] or item["restartCount"] for item in containers):
        raise ValueError("Agent containers are unready or have restarted")
    return pod["metadata"]["uid"], {item["name"]: item["containerID"] for item in containers}


def timestamp(value):
    result = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    if result.tzinfo is None:
        raise ValueError("episode timestamps must include a timezone")
    return result.timestamp()


def integer_divisor(value):
    for divisor in (1, 1000, 1000000, 1000000000):
        if 1e9 < value / divisor < 4e9:
            return divisor
    raise ValueError(f"unexpected Time value {value}")


def validate(output, flush):
    import pyarrow as pa
    import pyarrow.compute as pc
    import pyarrow.parquet as pq

    current_identity = identity(output, "agent-fetch.json")
    history_verified = (output / "agent-before.json").exists()
    if (
        history_verified
        and not identity(output, "agent-before.json") == identity(output, "agent-after.json") == current_identity
    ):
        raise ValueError("Agent changed between the episode and fetching")

    with tarfile.open(output / "recording.tar") as archive:
        members = archive.getmembers()
        names = set()
        for member in members:
            path = PurePosixPath(member.name)
            if member.isdir() and member.name in (".", "./"):
                continue
            if (
                not member.isfile()
                or path.is_absolute()
                or len(path.parts) != 1
                or path.name == ".."
                or path.name in names
            ):
                raise ValueError(f"unsafe or duplicate archive entry: {member.name}")
            names.add(path.name)
        for member in members:
            if member.isfile():
                destination = output / "parquet" / PurePosixPath(member.name).name
                with archive.extractfile(member) as source, destination.open("xb") as target:
                    shutil.copyfileobj(source, target)

    episode = json.loads((output / "episode.json").read_text())
    start, end = timestamp(episode["start_time"]), timestamp(episode["end_time"])
    if start >= end:
        raise ValueError("episode window must have positive duration")
    families = {family: {"files": 0, "rows": 0, "times": set()} for family in ("metrics", "logs")}
    dropped = []
    for filename in sorted((output / "parquet").glob("*.parquet")):
        family = next((key for key in families if filename.name.startswith(f"observer-{key}-")), None)
        if family is None:
            raise ValueError(f"unexpected parquet family: {filename.name}")
        table = pq.read_table(filename)
        column = table.column("Time")
        limits = pc.min_max(column).as_py() if len(column) else None
        if not limits or limits["min"] is None:
            dropped.append(filename)
            continue
        if pa.types.is_timestamp(column.type):
            divisor = {"s": 1, "ms": 1000, "us": 1000000, "ns": 1000000000}[column.type.unit]
        elif pa.types.is_integer(column.type):
            divisor = integer_divisor(limits["min"])
        else:
            raise ValueError(f"unsupported Time column: {column.type}")
        times = [value / divisor for value in pc.unique(pc.cast(column, pa.int64())).to_pylist() if value is not None]
        inside = [value for value in times if start <= value <= end]
        if max(times) < start or min(times) > end:
            dropped.append(filename)
            continue
        raw_time = pc.cast(column, pa.int64())
        selected = pc.and_(
            pc.greater_equal(raw_time, int(start * divisor)), pc.less_equal(raw_time, int(end * divisor))
        )
        families[family]["files"] += 1
        families[family]["rows"] += pc.sum(pc.cast(selected, pa.int64())).as_py()
        families[family]["times"].update(inside)

    errors = []
    warnings = [] if history_verified else ["no episode Agent snapshots; restart/replacement history is unknown"]
    for family, stats in families.items():
        times = sorted(stats.pop("times"))
        if not times:
            errors.append(f"no nonempty {family} files overlap the episode")
            stats.update(observed_window=None, max_gap_seconds=None)
            continue
        gaps = [times[0] - start, end - times[-1]] + [later - earlier for earlier, later in pairwise(times)]
        stats.update(observed_window=[times[0], times[-1]], max_gap_seconds=max(gaps))
        if gaps[0] > flush or gaps[1] > flush or any(gap > 2 * flush for gap in gaps[2:]):
            warnings.append(
                f"{family} has sparse timestamps or gaps; inspect coverage before claiming a complete recording"
            )

    report = {
        "usable": not errors,
        "window": [episode["start_time"], episode["end_time"]],
        "flush_seconds": flush,
        "families": families,
        "removed": len(dropped) if not errors else 0,
        "errors": errors,
        "coverage_warnings": warnings,
        "agent_history_verified": history_verified,
    }
    (output / "validation.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report, indent=2))
    if errors:
        raise SystemExit("recording validation failed; keep partial output for diagnosis, do not replay")
    for filename in dropped:
        filename.unlink()
    (output / "recording.tar").unlink()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("wait", "validate"))
    parser.add_argument("path", type=Path)
    parser.add_argument("flush", type=int)
    arguments = parser.parse_args()
    if arguments.flush <= 0:
        parser.error("flush interval must be positive seconds")
    if arguments.command == "wait":
        print(wait_seconds(arguments.path, arguments.flush))
    else:
        validate(arguments.path, arguments.flush)


if __name__ == "__main__":
    main()
