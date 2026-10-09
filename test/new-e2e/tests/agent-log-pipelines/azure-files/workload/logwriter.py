#!/usr/bin/env python3
"""Log writer of the Azure Files E2E suite, for a stock Python image.

The Java writer next to this file (LogWriterService.java, the RollingFile
appender of log4j2.xml, and Crc64.java) needs a custom image. This port runs
on the stock image the suite uses by default, from the workload ConfigMap, and
reproduces every part of the Java writer that the suite's assertions read:

- records: the same message, level and log4j2 LOG_PATTERN line, written with
  one write per line, and the raw padding, without a newline, that brings each
  completed file to its target size;
- rotation: the RollingFile TimeBasedTriggeringPolicy at the minute resolution
  of its filePattern. The first record of a new UTC minute closes the active
  file, renames it to app.log.<ddMMyyyy_HHmm of the minute it was opened in>,
  and only then opens app.log again;
- schedule: Spring's fixed-delay tick, the head record and head pause, the fill
  to the period's target, and the deferral of a first period too short to fill;
- ledger: `crc64` prints what Crc64.java prints, so ledger.sh writes the same
  ledger with either helper.

Unset, the settings below keep that behaviour. The Java writer has none of
them:

- LOGWRITER_ROTATION_MODE: rename (the default, above), copytruncate,
  delete-recreate or gzip; see RollingFile and CopyTruncateFile;
- LOGWRITER_PERIOD_MS: the rotation period, 60000 by default. Periods start at
  multiples of it since the epoch. A period that is not a whole number of
  minutes names its records and files to the second;
- LOGWRITER_RATE_BYTES_PER_SEC: above zero, records are written at this rate,
  shared by all streams, from the end of the head pause to the end of the
  period, instead of filling the period to its target size at once. They can
  be batched into writes of up to LOGWRITER_BUFFER_BYTES;
- LOGWRITER_STREAMS: above zero, that many independent streams under
  svc-1/app.log to svc-<N>/app.log, each with its own run ID
  (<run ID>-svc-<i>), sequences, journal and ledger;
- LOGWRITER_PAYLOAD_BYTES, LOGWRITER_CONSOLE_RECORDS: the record payload size,
  and whether records are echoed to stdout;
- LOGWRITER_MAX_ROTATED_FILES: above zero, the oldest rotated files beyond
  that many are deleted.

Journal: before it renames, copies, truncates, compresses or deletes a file,
the writer appends one line for the period that file holds to periods.jsonl
next to it: the sequences and bytes it wrote, the sequences it failed to
write, and, for copytruncate, the sequences written between the start of the
copy and the truncation, which a reader may lose without being at fault.
ledger.sh records those lines in the ledger, so the ledger also covers files
that are gone or compressed by the time it looks at the share.

Commands:

    logwriter.py
        Runs the writer, configured through the LOGWRITER_* variables of the
        Java writer (application.properties) and the ones above.
    logwriter.py crc64 bytes|line PATH
        Prints Go's hash/crc64 ISO checksum of the first 2048 bytes or the
        first line of PATH, like Crc64.java.
    logwriter.py selftest --start 2026-08-13T12:00:10Z --rotations 4
        Runs the same writer on a simulated clock, whose sleeps return at once,
        until every stream has rotated the given number of files and finished
        compressing them.
"""

import argparse
import collections
import datetime
import functools
import gzip
import hashlib
import json
import os
import re
import shutil
import signal
import sys
import threading
import time
import traceback
import uuid

ACTIVE_LOG_NAME = "app.log"
JOURNAL_NAME = "periods.jsonl"
STREAM_DIR_PREFIX = "svc-"
DEFAULT_LOG_DIR = "/mnt/azure-files"
MINUTE_MS = 60 * 1000
DEFAULT_PAYLOAD_BYTES = 128
MAX_PADDING_BYTES = 512
RUN_ID_PATTERN = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,62}")
TARGET_BYTES_PATTERN = re.compile(r"[+-]?[0-9]+")
MIN_SEQUENCE_TARGET_BYTES = 511
MAX_SEQUENCE_TARGET_BYTES = 1024 * 1024

ROTATION_MODES = ("rename", "copytruncate", "delete-recreate", "gzip")
MAX_STREAMS = 64
MAX_RATE_BYTES_PER_SEC = 64 * 1024 * 1024
MAX_BUFFER_BYTES = 16 * 1024 * 1024
MAX_PAYLOAD_BYTES = 64 * 1024
# A paced writer that fell further behind than this, for example while the
# share stalled, skips the backlog instead of writing it in one burst.
MAX_CATCH_UP_MS = 5000
COPY_CHUNK_BYTES = 1024 * 1024
# A delete of a compressed file that fails is retried once a second, this many
# times.
DELETE_ATTEMPTS = 10

# log4j2.xml renders every line with
#   %d{yyyy-MM-dd HH:mm:ss.SSS}  %-5level %pid --- [%20t] %-40c{1.} : %msg%n
# in UTC (TZ and -Duser.timezone). %t is the Spring scheduler thread that runs
# every tick, or the main thread that starts the writer, %pid is 1 in the
# container, and %c{1.} shortens each package of the logger name to its first
# letter.
THREAD_NAME = "scheduling-1"
MAIN_THREAD_NAME = "main"
DATA_LOGGER = "c.d.e.l.LogWriterService"
STATUS_LOGGER = "l.status"
SCHEDULER_LOGGER = "o.s.s.s.TaskUtils$LoggingErrorHandler"
# log4j2.xml filePattern: ${LOG_FILE}.%d{ddMMyyyy_HHmm}
ROTATED_SUFFIX_FORMAT = "%d%m%Y_%H%M"
# LogWriterService.PERIOD_FORMAT: yyyyMMdd'T'HHmm'Z'
PERIOD_FORMAT = "%Y%m%dT%H%MZ"
# A period that is not a whole number of minutes is named to the second.
SECOND_ROTATED_SUFFIX_FORMAT = "%d%m%Y_%H%M%S"
SECOND_PERIOD_FORMAT = "%Y%m%dT%H%M%SZ"

CRC64_ISO_POLYNOMIAL = 0xD800000000000000
CRC64_MASK = (1 << 64) - 1
CRC64_HEAD_BYTES = 2048


def utc(seconds):
    return datetime.datetime.fromtimestamp(seconds, datetime.timezone.utc)


def iso_ms(milliseconds):
    seconds, millis = divmod(milliseconds, 1000)
    return utc(seconds).strftime("%Y-%m-%dT%H:%M:%S") + f".{millis:03d}Z"


@functools.lru_cache(maxsize=256)
def format_timestamp(event_ms):
    seconds, millis = divmod(event_ms, 1000)
    return utc(seconds).strftime("%Y-%m-%d %H:%M:%S") + f".{millis:03d}"


def format_line(event_ms, level, logger, pid, message, thread=THREAD_NAME):
    return f"{format_timestamp(event_ms)}  {level:<5} {pid} --- [{thread:>20}] {logger:<40} : {message}\n"


def level_of(sequence):
    if sequence % 20 == 0:
        return "ERROR"
    if sequence % 10 == 0:
        return "WARN"
    return "INFO"


def write_all(fd, data):
    view = memoryview(data)
    while view:
        view = view[os.write(fd, view) :]


class SystemClock:
    def now_ms(self):
        return time.time_ns() // 1_000_000

    def sleep_ms(self, milliseconds):
        if milliseconds > 0:
            time.sleep(milliseconds / 1000)


class SimulatedClock:
    """A clock whose sleeps advance it instead of waiting."""

    def __init__(self, start_ms):
        self.current_ms = start_ms

    def now_ms(self):
        return self.current_ms

    def sleep_ms(self, milliseconds):
        if milliseconds > 0:
            self.current_ms += milliseconds


class Console:
    """The Console appender of log4j2.xml, shared by the streams."""

    def __init__(self, stream, pid):
        self.stream = stream
        self.pid = pid
        self.lock = threading.Lock()

    def log(self, event_ms, level, logger, message, thread=THREAD_NAME):
        line = format_line(event_ms, level, logger, self.pid, message, thread)
        with self.lock:
            self.stream.write(line)
            self.stream.flush()


class Periods:
    """The rotation periods: UTC minutes like the Java writer, or
    LOGWRITER_PERIOD_MS, aligned on the epoch."""

    def __init__(self, period_ms):
        self.period_ms = period_ms
        if period_ms % MINUTE_MS == 0:
            self.name_format, self.suffix_format = PERIOD_FORMAT, ROTATED_SUFFIX_FORMAT
        else:
            self.name_format, self.suffix_format = SECOND_PERIOD_FORMAT, SECOND_ROTATED_SUFFIX_FORMAT

    def floor(self, milliseconds):
        return milliseconds - milliseconds % self.period_ms

    def name(self, milliseconds):
        return utc(self.floor(milliseconds) // 1000).strftime(self.name_format)

    def suffix(self, file_time_ms):
        return utc(file_time_ms // 1000).strftime(self.suffix_format)


def should_defer_initial_period(now_ms, periods, head_pause_ms, fill_runway_ms):
    remaining_ms = periods.floor(now_ms) + periods.period_ms - now_ms
    return remaining_ms <= head_pause_ms + fill_runway_ms


def read_int(environ, name, default, minimum, maximum):
    configured = environ.get(name, "").strip()
    if not configured:
        return default
    if not re.fullmatch(r"[0-9]+", configured):
        raise ValueError(f"{name} must be a whole number, got {configured!r}")
    value = int(configured)
    if value < minimum or value > maximum:
        raise ValueError(f"{name} must be between {minimum} and {maximum}, got {value}")
    return value


def read_bool(environ, name, default):
    configured = environ.get(name, "").strip().lower()
    if not configured:
        return default
    if configured not in ("true", "false"):
        raise ValueError(f"{name} must be true or false, got {configured!r}")
    return configured == "true"


class Config:
    """application.properties, read from the same environment variables, and
    the settings the Java writer does not have."""

    def __init__(self, environ):
        self.log_dir = environ.get("LOGWRITER_LOG_DIR", DEFAULT_LOG_DIR)
        self.interval_ms = int(environ.get("LOGWRITER_INTERVAL_MS", "250"))
        self.initial_delay_ms = int(environ.get("LOGWRITER_INITIAL_DELAY_MS", "1000"))
        self.target_bytes = int(environ.get("LOGWRITER_TARGET_BYTES", "83000"))
        self.target_bytes_sequence = environ.get("LOGWRITER_TARGET_BYTES_SEQUENCE", "")
        self.head_pause_ms = int(environ.get("LOGWRITER_HEAD_PAUSE_MS", "5000"))
        self.initial_fill_runway_ms = int(environ.get("LOGWRITER_INITIAL_FILL_RUNWAY_MS", "10000"))
        self.max_records_per_period = int(environ.get("LOGWRITER_MAX_RECORDS_PER_PERIOD", "2000"))
        self.run_id = read_run_id(environ.get("LOGWRITER_RUN_ID", ""))
        self.host = environ.get("HOSTNAME", "unknown")

        self.rotation_mode = environ.get("LOGWRITER_ROTATION_MODE", "").strip() or "rename"
        if self.rotation_mode not in ROTATION_MODES:
            raise ValueError(f"LOGWRITER_ROTATION_MODE must be one of {', '.join(ROTATION_MODES)}")
        self.period_ms = read_int(environ, "LOGWRITER_PERIOD_MS", MINUTE_MS, 1000, 24 * 60 * MINUTE_MS)
        if self.period_ms % 1000 != 0:
            raise ValueError("LOGWRITER_PERIOD_MS must be whole seconds")
        self.rate_bytes_per_sec = read_int(environ, "LOGWRITER_RATE_BYTES_PER_SEC", 0, 0, MAX_RATE_BYTES_PER_SEC)
        self.buffer_bytes = read_int(environ, "LOGWRITER_BUFFER_BYTES", 0, 0, MAX_BUFFER_BYTES)
        self.streams = read_int(environ, "LOGWRITER_STREAMS", 0, 0, MAX_STREAMS)
        self.payload_bytes = read_int(environ, "LOGWRITER_PAYLOAD_BYTES", DEFAULT_PAYLOAD_BYTES, 1, MAX_PAYLOAD_BYTES)
        self.console_records = read_bool(environ, "LOGWRITER_CONSOLE_RECORDS", True)
        self.max_rotated_files = read_int(environ, "LOGWRITER_MAX_ROTATED_FILES", 0, 0, 1_000_000)
        day_ms = 24 * 60 * MINUTE_MS
        self.delete_pause_ms = read_int(environ, "LOGWRITER_DELETE_PAUSE_MS", 1000, 0, day_ms)
        self.gzip_delay_ms = read_int(environ, "LOGWRITER_GZIP_DELAY_MS", 5000, 0, day_ms)
        self.copytruncate_hold_ms = read_int(environ, "LOGWRITER_COPYTRUNCATE_HOLD_MS", 3000, 0, day_ms)


def read_run_id(configured):
    if not configured:
        return str(uuid.uuid4())
    if not RUN_ID_PATTERN.fullmatch(configured):
        raise ValueError("LOGWRITER_RUN_ID must match [A-Za-z0-9][A-Za-z0-9_.-]{0,62}")
    return configured


def parse_target_bytes_sequence(configured, fallback):
    if not configured.strip():
        if fallback <= 0:
            raise ValueError("target bytes must be positive")
        return [fallback]
    values = configured.split(",")
    # String.split drops trailing empty values.
    while values and values[-1] == "":
        values.pop()
    if not values:
        raise ValueError("invalid target byte sequence: " + configured)
    targets = []
    for value in values:
        value = value.strip()
        if not TARGET_BYTES_PATTERN.fullmatch(value):
            raise ValueError("invalid target byte sequence: " + configured)
        target = int(value)
        if target < MIN_SEQUENCE_TARGET_BYTES or target > MAX_SEQUENCE_TARGET_BYTES:
            raise ValueError("target byte sequence values must be between 511 and 1048576")
        targets.append(target)
    return targets


class Stream:
    """One active file: its directory, its name in the journal ("" at the root
    of the log directory), and the run ID of its records."""

    def __init__(self, directory, name, run_id):
        self.directory = directory
        self.name = name
        self.run_id = run_id


def streams_of(config):
    if config.streams == 0:
        return [Stream(config.log_dir, "", config.run_id)]
    streams = []
    for index in range(1, config.streams + 1):
        name = STREAM_DIR_PREFIX + str(index)
        run_id = config.run_id + "-" + name
        if not RUN_ID_PATTERN.fullmatch(run_id):
            raise ValueError(f"stream run ID {run_id} is longer than 63 characters; shorten LOGWRITER_RUN_ID")
        streams.append(Stream(os.path.join(config.log_dir, name), name, run_id))
    return streams


def add_range(ranges, first, last):
    if ranges and ranges[-1][1] == first - 1:
        ranges[-1][1] = last
    else:
        ranges.append([first, last])


class PeriodStats:
    """What the writer put into one active file: its journal line."""

    def __init__(self):
        self.period = ""
        self.target_bytes = 0
        self.first_sequence = 0
        self.last_sequence = 0
        self.records = 0
        self.bytes = 0
        # [first, last] ranges of the sequences that could not be written.
        self.unwritten = []

    def empty(self):
        return self.first_sequence == 0

    def written(self, period, target, first, last, count):
        self._assign(period, target, first, last)
        self.records += last - first + 1
        self.bytes += count

    def failed(self, period, target, first, last):
        self._assign(period, target, first, last)
        add_range(self.unwritten, first, last)

    def padded(self, count):
        self.bytes += count

    def _assign(self, period, target, first, last):
        if self.first_sequence == 0:
            self.period = period
            self.target_bytes = target
            self.first_sequence = first
        self.last_sequence = last


def read_head(path):
    """The head of a file as the ledger checksums it, or None."""
    try:
        return first_bytes(path, CRC64_HEAD_BYTES), first_line(path)
    except OSError:
        return None


def head_checksums(head):
    if head is None:
        return {"first_2048_sha256": "", "first_2048_crc64": "", "first_line_sha256": "", "first_line_crc64": ""}
    first_2048, line = head
    return {
        "first_2048_sha256": hashlib.sha256(first_2048).hexdigest(),
        "first_2048_crc64": f"0x{crc64_iso(first_2048):x}",
        "first_line_sha256": hashlib.sha256(line).hexdigest(),
        "first_line_crc64": f"0x{crc64_iso(line):x}",
    }


class RollingFile:
    """The AppRollingFile appender of log4j2.xml, for the rename, gzip and
    delete-recreate modes.

    Like RollingFileManager.checkRollover with a TimeBasedTriggeringPolicy of
    interval 1 and modulate true, every event first checks the rollover. An
    event at or past the next period boundary closes the active file, and only
    then opens the active path again to write the event. Only records go
    through here: the writer's padding opens the path on its own, like
    Files.write in Java, and never rolls over.

    Once the writer has journalled the closed file, the rollover:
    - rename: renames it after the period in which the policy last rolled over
      or started (the pattern's prevFileTime);
    - gzip: renames it the same way, then, LOGWRITER_GZIP_DELAY_MS later,
      compresses it to <rotated name>.gz and deletes it;
    - delete-recreate: waits LOGWRITER_DELETE_PAUSE_MS and deletes it, so the
      event creates a new file at the same path.
    """

    def __init__(self, writer, path, clock, periods, mode):
        self.writer = writer
        self.path = path
        self.periods = periods
        self.mode = mode
        # RollingFileManager starts from the existing file's creation time,
        # which Java 8 reports as the modification time on Linux, or from now.
        try:
            initial_ms = os.stat(path).st_mtime_ns // 1_000_000
        except FileNotFoundError:
            initial_ms = 0
        if initial_ms <= 0:
            initial_ms = clock.now_ms()
        self.file_time_ms = periods.floor(initial_ms)
        self.next_rollover_ms = self.file_time_ms + periods.period_ms
        self.fd = None
        self.rotations = 0

    def append(self, event_ms, data):
        if event_ms >= self.next_rollover_ms:
            rotated_file_time_ms = self.file_time_ms
            self.file_time_ms = self.periods.floor(event_ms)
            self.next_rollover_ms = self.file_time_ms + self.periods.period_ms
            self._rollover(rotated_file_time_ms)
        if self.fd is None:
            # FileOutputStream(path, true): O_WRONLY | O_CREAT | O_APPEND.
            self.fd = os.open(self.path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o666)
        # immediateFlush: one write per event.
        write_all(self.fd, data)

    def active_size(self):
        """The size the writer fills to its target, like Files.size."""
        try:
            return os.stat(self.path).st_size
        except FileNotFoundError:
            return 0

    def _rollover(self, rotated_file_time_ms):
        if self.fd is not None:
            os.close(self.fd)
            self.fd = None
        stats = self.writer.take_stats()
        if self.mode == "delete-recreate":
            rotated = self._delete(stats)
        else:
            rotated = self._rotate(self.path + "." + self.periods.suffix(rotated_file_time_ms), stats)
        if rotated:
            self.rotations += 1

    def _rotate(self, target, stats):
        """FileRenameAction.execute without renameEmptyFiles."""
        name = os.path.basename(target)
        try:
            size = os.stat(self.path).st_size
        except FileNotFoundError:
            self.writer.journal_period(stats, name, "missing", None)
            return False
        if size == 0:
            # An empty active file is deleted rather than rotated.
            self.writer.journal_period(stats, name, "empty", None)
            os.remove(self.path)
            return False
        if self.mode == "gzip":
            self.writer.journal_period(stats, name, "compressed", read_head(self.path), archive=name + ".gz")
        else:
            self.writer.journal_period(stats, name, "renamed", read_head(self.path))
        if not self._move(target):
            return False
        if self.mode == "gzip":
            due_ms = self.writer.clock.now_ms() + self.writer.config.gzip_delay_ms
            self.writer.schedule(CompressJob(self.writer, target, target + ".gz", due_ms))
        self.writer.retain(name)
        return True

    def _move(self, target):
        try:
            # Files.move with ATOMIC_MOVE and REPLACE_EXISTING.
            os.replace(self.path, target)
            return True
        except OSError as error:
            print(f"rename {self.path} to {target} failed, copying instead: {error}", file=sys.stderr, flush=True)
        # Log4j falls back to copying the file and deleting it, or truncating
        # it when it cannot be deleted.
        try:
            shutil.copyfile(self.path, target)
        except OSError as error:
            print(f"copy {self.path} to {target} failed: {error}", file=sys.stderr, flush=True)
            return False
        try:
            os.remove(self.path)
        except OSError:
            with open(self.path, "w"):
                pass
        return True

    def _delete(self, stats):
        try:
            size = os.stat(self.path).st_size
        except FileNotFoundError:
            self.writer.journal_period(stats, ACTIVE_LOG_NAME, "missing", None)
            return False
        self.writer.journal_period(stats, ACTIVE_LOG_NAME, "deleted", read_head(self.path))
        self.writer.sleep(self.writer.config.delete_pause_ms)
        try:
            os.remove(self.path)
        except FileNotFoundError:
            return False
        except OSError as error:
            # The next record appends to the file that is still there.
            print(f"delete {self.path} failed: {error}", file=sys.stderr, flush=True)
            return False
        self.writer.status(
            "INFO", f"active_file_deleted run_id={self.writer.stream.run_id} path={self.path} bytes={size}"
        )
        return size > 0


class CopyTruncateFile:
    """logrotate's copytruncate, for the copytruncate mode.

    The writer opens app.log once, with O_APPEND, and never reopens it. At
    each period boundary a rotator, which the writer runs between its own
    writes (LogWriter.service), copies app.log to app.log.<suffix of the
    period> while the writer keeps appending, waits
    LOGWRITER_COPYTRUNCATE_HOLD_MS, and truncates app.log in place. The
    writer's next append then lands at the new end of the file, offset 0.

    What the writer appends between the start of the copy and the truncation
    is in the copy only if the copy reached it, and in app.log only until the
    truncation, so a reader of app.log can lose it without being at fault:
    those sequences are journalled as at risk. Everything appended before the
    copy started stayed in app.log for at least the hold.
    """

    def __init__(self, writer, path, clock, periods):
        self.writer = writer
        self.path = path
        self.periods = periods
        try:
            initial_ms = os.stat(path).st_mtime_ns // 1_000_000
        except FileNotFoundError:
            initial_ms = 0
        if initial_ms <= 0:
            initial_ms = clock.now_ms()
        self.file_time_ms = periods.floor(initial_ms)
        self.next_rollover_ms = self.file_time_ms + periods.period_ms
        self.fd = None
        self.rotations = 0
        self.job = None

    def append(self, event_ms, data):
        if self.fd is None:
            self.fd = os.open(self.path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o666)
        write_all(self.fd, data)

    def active_size(self):
        """The bytes of the current period: app.log also holds the previous
        period until the truncation."""
        return self.writer.stats.bytes

    def wake_ms(self):
        return None if self.job is not None else self.next_rollover_ms

    def service(self, now_ms):
        if self.job is not None or now_ms < self.next_rollover_ms:
            return
        rotated_file_time_ms = self.file_time_ms
        self.file_time_ms = self.periods.floor(now_ms)
        self.next_rollover_ms = self.file_time_ms + self.periods.period_ms
        target = self.path + "." + self.periods.suffix(rotated_file_time_ms)
        stats = self.writer.take_stats()
        try:
            size = os.stat(self.path).st_size
        except FileNotFoundError:
            size = 0
        if size == 0:
            # notifempty
            self.writer.journal_period(stats, os.path.basename(target), "empty", None)
            return
        self.job = CopyTruncateJob(self, stats, read_head(self.path), target, self.writer.sequence, now_ms)
        self.writer.schedule(self.job)


class CopyTruncateJob:
    """One copytruncate rotation: copy in chunks, hold, journal, truncate."""

    def __init__(self, owner, stats, head, target, copy_start_sequence, now_ms):
        self.owner = owner
        self.writer = owner.writer
        self.stats = stats
        self.head = head
        self.target = target
        self.copy_start_sequence = copy_start_sequence
        self.due_ms = now_ms
        self.source = None
        self.copy = None
        self.copied = False

    def step(self, now_ms):
        if not self.copied:
            if self.source is None:
                self.source = open(self.owner.path, "rb", buffering=0)
                self.copy = open(self.target, "wb")
            chunk = self.source.read(COPY_CHUNK_BYTES)
            if chunk:
                self.copy.write(chunk)
                return False
            self._close()
            self.copied = True
            self.due_ms = now_ms + self.writer.config.copytruncate_hold_ms
            return False

        name = os.path.basename(self.target)
        at_risk = []
        if self.writer.sequence > self.copy_start_sequence:
            at_risk = [[self.copy_start_sequence + 1, self.writer.sequence]]
        self.writer.journal_period(self.stats, name, "copied", self.head, at_risk=at_risk)
        try:
            os.truncate(self.owner.path, 0)
        except OSError as error:
            print(f"truncate {self.owner.path} failed: {error}", file=sys.stderr, flush=True)
        self.owner.rotations += 1
        self.owner.job = None
        self.writer.retain(name)
        return True

    def abort(self):
        """Nothing was truncated: the period is journalled without risk."""
        self._close()
        self.owner.job = None
        self.writer.journal_period(self.stats, os.path.basename(self.target), "copy-failed", self.head)

    def _close(self):
        for handle in (self.source, self.copy):
            if handle is not None:
                handle.close()
        self.source = self.copy = None


class CompressJob:
    """Compresses a rotated file to <name>.gz, then deletes it."""

    def __init__(self, writer, path, archive_path, due_ms):
        self.writer = writer
        self.path = path
        self.archive_path = archive_path
        self.due_ms = due_ms
        self.source = None
        self.archive = None
        self.compressed = False
        self.delete_attempts = 0

    def step(self, now_ms):
        if not self.compressed:
            if self.source is None:
                try:
                    self.source = open(self.path, "rb")
                except FileNotFoundError:
                    # Already deleted, by the retention for example.
                    return True
                self.archive = gzip.GzipFile(self.archive_path, "wb", compresslevel=6, mtime=now_ms // 1000)
            chunk = self.source.read(COPY_CHUNK_BYTES)
            if chunk:
                self.archive.write(chunk)
                return False
            self._close()
            self.compressed = True
        try:
            os.remove(self.path)
        except FileNotFoundError:
            return True
        except OSError as error:
            self.delete_attempts += 1
            print(f"delete {self.path} after compressing it failed: {error}", file=sys.stderr, flush=True)
            if self.delete_attempts >= DELETE_ATTEMPTS:
                return True
            self.due_ms = now_ms + 1000
            return False
        self.writer.status(
            "INFO", f"rotated_file_compressed run_id={self.writer.stream.run_id} archive={self.archive_path}"
        )
        return True

    def abort(self):
        self._close()

    def _close(self):
        for handle in (self.source, self.archive):
            if handle is not None:
                handle.close()
        self.source = self.archive = None


class LogWriter:
    """LogWriterService for one stream: one header record per period, a head
    pause, then records up to the period's target size, or at the configured
    rate until the period ends."""

    def __init__(self, config, stream, clock, console, rate_bytes_per_sec=None):
        self.config = config
        self.stream = stream
        self.clock = clock
        self.console = console
        self.periods = Periods(config.period_ms)
        os.makedirs(stream.directory, exist_ok=True)
        self.log_path = os.path.join(stream.directory, ACTIVE_LOG_NAME)
        self.journal_path = os.path.join(stream.directory, JOURNAL_NAME)
        self.targets = parse_target_bytes_sequence(config.target_bytes_sequence, config.target_bytes)
        if rate_bytes_per_sec is None:
            rate_bytes_per_sec = config.rate_bytes_per_sec
        self.rate_bytes_per_sec = rate_bytes_per_sec
        self.payload = "x" * config.payload_bytes
        self.sequence = 0
        self.stats = PeriodStats()
        self.jobs = []
        self.rotated_names = collections.deque()
        self._servicing = False
        # Target-size schedule, like the Java writer.
        self.completed_period = ""
        self.completed_periods = 0
        # Rate schedule.
        self.paced_period = ""
        self.paced_records = 0
        self.paced_fill_start_ms = 0
        self.paced_fill_bytes = 0
        self.paced_target = rate_bytes_per_sec * config.period_ms // 1000
        if config.rotation_mode == "copytruncate":
            self.file = CopyTruncateFile(self, self.log_path, clock, self.periods)
        else:
            self.file = RollingFile(self, self.log_path, clock, self.periods, config.rotation_mode)
        if config.rotation_mode == "rename" and config.period_ms == MINUTE_MS:
            rotation = "log4j2-minute"
        else:
            rotation = f"{config.rotation_mode} period_ms={config.period_ms}"
        schedule = f"rate_bytes_per_sec={rate_bytes_per_sec} " if rate_bytes_per_sec > 0 else ""
        self.status(
            "INFO",
            f"writer_ready run_id={stream.run_id} path={self.log_path} rotation={rotation} {schedule}"
            f"target_bytes={config.target_bytes} target_bytes_sequence=[{', '.join(str(t) for t in self.targets)}] "
            f"head_pause_ms={config.head_pause_ms} writer=python",
            MAIN_THREAD_NAME,
        )

    def status(self, level, message, thread=THREAD_NAME):
        self.console.log(self.clock.now_ms(), level, STATUS_LOGGER, message, thread)

    def tick(self):
        if self.rate_bytes_per_sec > 0:
            self.write_paced()
        else:
            self.write_current_period()

    # The rotator, the compressions, and the sleeps that run them.

    def schedule(self, job):
        self.jobs.append(job)

    def service(self):
        """Runs the copytruncate rotator and the jobs that are due. Between
        two steps of a job, a paced writer writes what it is due, so its rate
        holds while a rotated file is copied or compressed."""
        if self._servicing:
            return
        self._servicing = True
        try:
            rotator = getattr(self.file, "service", None)
            if rotator is not None:
                try:
                    rotator(self.clock.now_ms())
                except Exception:
                    traceback.print_exc()
                    sys.stderr.flush()
            for job in list(self.jobs):
                while job in self.jobs and job.due_ms <= self.clock.now_ms():
                    try:
                        finished = job.step(self.clock.now_ms())
                    except Exception:
                        traceback.print_exc()
                        sys.stderr.flush()
                        job.abort()
                        finished = True
                    if finished:
                        self.jobs.remove(job)
                        break
                    self.catch_up()
        finally:
            self._servicing = False

    def sleep(self, milliseconds):
        end_ms = self.clock.now_ms() + milliseconds
        while True:
            self.service()
            now_ms = self.clock.now_ms()
            if now_ms >= end_ms:
                return
            wake_ms = end_ms
            for job in self.jobs:
                wake_ms = min(wake_ms, job.due_ms)
            rotator_wake = getattr(self.file, "wake_ms", None)
            if rotator_wake is not None and rotator_wake() is not None:
                wake_ms = min(wake_ms, rotator_wake())
            self.clock.sleep_ms(max(wake_ms, now_ms + 1) - now_ms)

    # The journal and the retention.

    def take_stats(self):
        """Ends the active file's period: the next write starts another."""
        stats, self.stats = self.stats, PeriodStats()
        return stats

    def journal_period(self, stats, file_name, disposition, head, archive="", at_risk=None):
        if stats.empty():
            return
        entry = {
            "run_id": self.stream.run_id,
            "period": stats.period,
            "file": file_name,
            "target_bytes": stats.target_bytes,
            "bytes": stats.bytes,
        }
        entry.update(head_checksums(head))
        entry.update(
            {
                "first_sequence": stats.first_sequence,
                "last_sequence": stats.last_sequence,
                "line_count": stats.records,
                "stream": self.stream.name,
                "rotation_mode": self.config.rotation_mode,
                "disposition": disposition,
                "archive": archive,
                "unwritten_sequences": stats.unwritten,
                "at_risk_sequences": at_risk or [],
                "rotated_at": iso_ms(self.clock.now_ms()),
            }
        )
        line = json.dumps(entry, separators=(",", ":")) + "\n"
        try:
            fd = os.open(self.journal_path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o666)
            try:
                write_all(fd, line.encode())
                os.fsync(fd)
            finally:
                os.close(fd)
        except OSError as error:
            print(f"journal {self.journal_path} lost period {stats.period}: {error}", file=sys.stderr, flush=True)
            return
        self.status(
            "INFO",
            f"period_journaled run_id={self.stream.run_id} period={stats.period} file={file_name} "
            f"disposition={disposition} first_sequence={stats.first_sequence} last_sequence={stats.last_sequence} "
            f"records={stats.records} bytes={stats.bytes} unwritten={stats.unwritten} at_risk={at_risk or []}",
        )

    def retain(self, name):
        if self.config.max_rotated_files <= 0:
            return
        self.rotated_names.append(name)
        while len(self.rotated_names) > self.config.max_rotated_files:
            oldest = self.rotated_names.popleft()
            for path in (
                os.path.join(self.stream.directory, oldest),
                os.path.join(self.stream.directory, oldest + ".gz"),
            ):
                try:
                    os.remove(path)
                except FileNotFoundError:
                    pass
                except OSError as error:
                    print(
                        f"delete {path} beyond LOGWRITER_MAX_ROTATED_FILES failed: {error}", file=sys.stderr, flush=True
                    )

    # Records.

    def record_message(self, period, phase, record, period_target):
        return (
            f"run_id={self.stream.run_id} period={period} sequence={self.sequence} record={record} "
            f"phase={phase} target_bytes={period_target} host={self.config.host} payload={self.payload}"
        )

    def write_record(self, period, phase, record, period_target):
        self.sequence += 1
        message = self.record_message(period, phase, record, period_target)
        level = level_of(self.sequence)
        event_ms = self.clock.now_ms()
        # The data logger writes to the console and to the rolling file.
        if self.config.console_records:
            self.console.log(event_ms, level, DATA_LOGGER, message)
        line = format_line(event_ms, level, DATA_LOGGER, self.console.pid, message)
        return self._append(event_ms, line.encode(), self.sequence, self.sequence, period, period_target)

    def _append(self, event_ms, data, first, last, period, period_target):
        try:
            self.file.append(event_ms, data)
        except OSError as error:
            # Log4j2 appenders ignore their exceptions by default: the record
            # is lost and the writer carries on. The journal lists it.
            lost = str(first) if first == last else f"{first}-{last}"
            print(f"AppRollingFile lost sequence {lost}: {error}", file=sys.stderr, flush=True)
            self.stats.failed(period, period_target, first, last)
            return False
        self.stats.written(period, period_target, first, last, len(data))
        return True

    def append_padding(self, count):
        if count <= 0:
            return
        if count > MAX_PADDING_BYTES:
            raise ValueError("padding must not exceed 512 bytes")
        # Files.write(path, bytes, APPEND): its own descriptor, no create.
        fd = os.open(self.log_path, os.O_WRONLY | os.O_APPEND)
        try:
            write_all(fd, b"p" * count)
        finally:
            os.close(fd)
        self.stats.padded(count)

    # The Java writer's schedule: fill each period to its target at once.

    def write_current_period(self):
        period = self.periods.name(self.clock.now_ms())
        if period == self.completed_period:
            return
        if self.sequence == 0 and should_defer_initial_period(
            self.clock.now_ms(), self.periods, self.config.head_pause_ms, self.config.initial_fill_runway_ms
        ):
            return

        period_target = self.targets[self.completed_periods % len(self.targets)]
        self.write_record(period, "head", 1, period_target)
        self.sleep(self.config.head_pause_ms)

        records = 1
        while True:
            size = self.file.active_size()
            if size >= period_target:
                break
            if period != self.periods.name(self.clock.now_ms()):
                self.status(
                    "WARN",
                    f"period_changed_before_target run_id={self.stream.run_id} period={period} "
                    f"records={records} bytes={size}",
                )
                return
            if records >= self.config.max_records_per_period:
                raise RuntimeError("active file did not reach target size before max records: " + self.log_path)
            remaining = period_target - size
            if remaining <= MAX_PADDING_BYTES:
                self.append_padding(remaining)
                break
            records += 1
            if not self.write_record(period, "fill", records, period_target):
                # The file cannot take records right now, for example while a
                # deleted file is still open elsewhere: retry on the next tick.
                self.sleep(self.config.interval_ms)

        self.completed_period = period
        self.completed_periods += 1
        self.status(
            "INFO",
            f"period_complete run_id={self.stream.run_id} period={period} target_bytes={period_target} "
            f"records={records} bytes={self.file.active_size()} "
            f"first_sequence={self.sequence - records + 1} last_sequence={self.sequence}",
        )

    # The rate schedule: a head record, the head pause, then records at the
    # rate until the period ends.

    def write_paced(self):
        now_ms = self.clock.now_ms()
        period = self.periods.name(now_ms)
        if period != self.paced_period:
            if self.sequence == 0 and should_defer_initial_period(
                now_ms, self.periods, self.config.head_pause_ms, self.config.initial_fill_runway_ms
            ):
                return
            self.paced_period = period
            self.paced_records = 1
            self.paced_fill_start_ms = now_ms + self.config.head_pause_ms
            self.paced_fill_bytes = 0
            self.write_record(period, "head", 1, self.paced_target)
            return
        self.catch_up()

    def catch_up(self):
        if self.rate_bytes_per_sec <= 0 or not self.paced_period:
            return
        now_ms = self.clock.now_ms()
        if now_ms < self.paced_fill_start_ms or self.periods.name(now_ms) != self.paced_period:
            return
        due = (now_ms - self.paced_fill_start_ms) * self.rate_bytes_per_sec // 1000 - self.paced_fill_bytes
        limit = MAX_CATCH_UP_MS * self.rate_bytes_per_sec // 1000
        if due > limit:
            self.paced_fill_bytes += due - limit
            due = limit
        if due > 0:
            self.write_fill_records(self.paced_period, due, now_ms)

    def write_fill_records(self, period, budget, event_ms):
        """Writes records worth budget bytes, in writes of up to
        LOGWRITER_BUFFER_BYTES, or one per record without it."""
        limit = self.config.buffer_bytes
        batch = []
        batch_bytes = 0
        batch_first = 0
        while budget > 0:
            self.sequence += 1
            self.paced_records += 1
            message = self.record_message(period, "fill", self.paced_records, self.paced_target)
            level = level_of(self.sequence)
            if self.config.console_records:
                self.console.log(event_ms, level, DATA_LOGGER, message)
            data = format_line(event_ms, level, DATA_LOGGER, self.console.pid, message).encode()
            if batch and (limit == 0 or batch_bytes + len(data) > limit):
                self._append(event_ms, b"".join(batch), batch_first, self.sequence - 1, period, self.paced_target)
                batch, batch_bytes = [], 0
            if not batch:
                batch_first = self.sequence
            batch.append(data)
            batch_bytes += len(data)
            budget -= len(data)
            self.paced_fill_bytes += len(data)
        if batch:
            self._append(event_ms, b"".join(batch), batch_first, self.sequence, period, self.paced_target)


def run(writer, config, done=None):
    """Spring's @Scheduled(fixedDelay, initialDelay): a failed tick is logged
    and the next one still runs."""
    writer.sleep(config.initial_delay_ms)
    while done is None or not done():
        try:
            writer.service()
            writer.tick()
        except Exception:
            writer.console.log(
                writer.clock.now_ms(), "ERROR", SCHEDULER_LOGGER, "Unexpected error occurred in scheduled task"
            )
            traceback.print_exc()
            sys.stderr.flush()
        writer.sleep(config.interval_ms)


def stream_rate(config, streams):
    return config.rate_bytes_per_sec // len(streams)


def crc64_table():
    table = []
    for index in range(256):
        crc = index
        for _ in range(8):
            crc = (crc >> 1) ^ CRC64_ISO_POLYNOMIAL if crc & 1 else crc >> 1
        table.append(crc)
    return table


CRC64_TABLE = crc64_table()


def crc64_iso(data):
    crc = CRC64_MASK
    for value in data:
        crc = CRC64_TABLE[(crc ^ value) & 0xFF] ^ (crc >> 8)
    return crc ^ CRC64_MASK


def first_bytes(path, count):
    chunks = []
    with open(path, "rb") as source:
        while count > 0:
            chunk = source.read(count)
            if not chunk:
                break
            chunks.append(chunk)
            count -= len(chunk)
    return b"".join(chunks)


def first_line(path):
    with open(path, "rb") as source:
        line = source.readline()
    if line.endswith(b"\n"):
        line = line[:-1]
    if line.endswith(b"\r"):
        line = line[:-1]
    return line


def crc64_main(args):
    if len(args) != 2 or args[0] not in ("bytes", "line"):
        print("usage: logwriter.py crc64 <bytes|line> <path>", file=sys.stderr)
        return 2
    content = first_bytes(args[1], CRC64_HEAD_BYTES) if args[0] == "bytes" else first_line(args[1])
    print(f"0x{crc64_iso(content):x}")
    return 0


def rotated_and_compressed(writer, rotations):
    return lambda: writer.file.rotations >= rotations and not writer.jobs


def selftest_main(args):
    parser = argparse.ArgumentParser(prog="logwriter.py selftest")
    parser.add_argument("--start", required=True, help="UTC start time, as 2026-08-13T12:00:10Z")
    parser.add_argument("--rotations", type=int, required=True, help="stop once this many files are rotated")
    options = parser.parse_args(args)
    start = datetime.datetime.strptime(options.start, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=datetime.timezone.utc)
    config = Config(os.environ)
    streams = streams_of(config)
    # The streams are independent, so each one runs on a clock of its own from
    # the same start, one after the other.
    for stream in streams:
        clock = SimulatedClock(int(start.timestamp()) * 1000)
        # The writer is PID 1 in its container.
        writer = LogWriter(config, stream, clock, Console(sys.stdout, 1), stream_rate(config, streams))
        run(writer, config, done=rotated_and_compressed(writer, options.rotations))
    return 0


def main(argv):
    if len(argv) > 1 and argv[1] == "crc64":
        return crc64_main(argv[2:])
    if len(argv) > 1 and argv[1] == "selftest":
        return selftest_main(argv[2:])
    if len(argv) > 1:
        print(__doc__, file=sys.stderr)
        return 2

    # As PID 1 the writer would otherwise ignore the SIGTERM that stops its
    # pod; the JVM exits on it.
    signal.signal(signal.SIGTERM, lambda signum, _frame: sys.exit(128 + signum))
    clock = SystemClock()
    config = Config(os.environ)
    streams = streams_of(config)
    console = Console(sys.stdout, os.getpid())
    writers = [LogWriter(config, stream, clock, console, stream_rate(config, streams)) for stream in streams]
    if len(writers) == 1:
        run(writers[0], config)
        return 0
    # One thread per stream; each one only touches its own files.
    threads = [threading.Thread(target=run, args=(writer, config), daemon=True) for writer in writers]
    for thread in threads:
        thread.start()
    while any(thread.is_alive() for thread in threads):
        for thread in threads:
            thread.join(1.0)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
