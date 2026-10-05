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

Commands:

    logwriter.py
        Runs the writer, configured through the LOGWRITER_* variables of the
        Java writer (application.properties).
    logwriter.py crc64 bytes|line PATH
        Prints Go's hash/crc64 ISO checksum of the first 2048 bytes or the
        first line of PATH, like Crc64.java.
    logwriter.py selftest --start 2026-08-13T12:00:10Z --rotations 4
        Runs the same writer on a simulated clock, whose sleeps return at once,
        until it has rotated the given number of files.
"""

import argparse
import datetime
import os
import re
import shutil
import signal
import sys
import time
import traceback
import uuid

ACTIVE_LOG_NAME = "app.log"
DEFAULT_LOG_DIR = "/mnt/azure-files"
MINUTE_MS = 60 * 1000
PAYLOAD = "x" * 128
MAX_PADDING_BYTES = 512
RUN_ID_PATTERN = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,62}")
TARGET_BYTES_PATTERN = re.compile(r"[+-]?[0-9]+")
MIN_SEQUENCE_TARGET_BYTES = 511
MAX_SEQUENCE_TARGET_BYTES = 1024 * 1024

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

CRC64_ISO_POLYNOMIAL = 0xD800000000000000
CRC64_MASK = (1 << 64) - 1
CRC64_HEAD_BYTES = 2048


def utc(seconds):
    return datetime.datetime.fromtimestamp(seconds, datetime.timezone.utc)


def minute_floor(milliseconds):
    return milliseconds - milliseconds % MINUTE_MS


def period_of(milliseconds):
    return utc(milliseconds // 1000).strftime(PERIOD_FORMAT)


def format_line(event_ms, level, logger, pid, message, thread=THREAD_NAME):
    seconds, millis = divmod(event_ms, 1000)
    timestamp = utc(seconds).strftime("%Y-%m-%d %H:%M:%S")
    return f"{timestamp}.{millis:03d}  {level:<5} {pid} --- [{thread:>20}] {logger:<40} : {message}\n"


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
    """The Console appender of log4j2.xml."""

    def __init__(self, stream, pid):
        self.stream = stream
        self.pid = pid

    def log(self, event_ms, level, logger, message, thread=THREAD_NAME):
        self.stream.write(format_line(event_ms, level, logger, self.pid, message, thread))
        self.stream.flush()


class MinuteRollingFile:
    """The AppRollingFile appender of log4j2.xml.

    Like RollingFileManager.checkRollover with a TimeBasedTriggeringPolicy of
    interval 1 and modulate true, every event first checks the rollover. An
    event at or past the next minute boundary closes the active file, renames
    it after the minute in which the policy last rolled over or started (the
    pattern's prevFileTime), and only then opens the active path again to
    write the event. Only records go through here: the writer's padding opens
    the path on its own, like Files.write in Java, and never rolls over.
    """

    def __init__(self, path, clock):
        self.path = path
        # RollingFileManager starts from the existing file's creation time,
        # which Java 8 reports as the modification time on Linux, or from now.
        try:
            initial_ms = os.stat(path).st_mtime_ns // 1_000_000
        except FileNotFoundError:
            initial_ms = 0
        if initial_ms <= 0:
            initial_ms = clock.now_ms()
        self.file_time_ms = minute_floor(initial_ms)
        self.next_rollover_ms = self.file_time_ms + MINUTE_MS
        self.fd = None
        self.rotations = 0

    def append(self, event_ms, data):
        if event_ms >= self.next_rollover_ms:
            rotated_file_time_ms = self.file_time_ms
            self.file_time_ms = minute_floor(event_ms)
            self.next_rollover_ms = self.file_time_ms + MINUTE_MS
            self._rollover(rotated_file_time_ms)
        if self.fd is None:
            # FileOutputStream(path, true): O_WRONLY | O_CREAT | O_APPEND.
            self.fd = os.open(self.path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o666)
        # immediateFlush: one write per event.
        write_all(self.fd, data)

    def _rollover(self, rotated_file_time_ms):
        if self.fd is not None:
            os.close(self.fd)
            self.fd = None
        suffix = utc(rotated_file_time_ms // 1000).strftime(ROTATED_SUFFIX_FORMAT)
        if self._rename(self.path + "." + suffix):
            self.rotations += 1

    def _rename(self, target):
        """FileRenameAction.execute without renameEmptyFiles."""
        try:
            size = os.stat(self.path).st_size
        except FileNotFoundError:
            return False
        if size == 0:
            # An empty active file is deleted rather than rotated.
            os.remove(self.path)
            return False
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


class Config:
    """application.properties, read from the same environment variables."""

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


def should_defer_initial_period(now_ms, head_pause_ms, fill_runway_ms):
    remaining_ms = minute_floor(now_ms) + MINUTE_MS - now_ms
    return remaining_ms <= head_pause_ms + fill_runway_ms


class LogWriter:
    """LogWriterService: one header record per UTC minute, a head pause, then
    records up to the period's target size."""

    def __init__(self, config, clock, console):
        self.config = config
        self.clock = clock
        self.console = console
        self.log_path = os.path.join(config.log_dir, ACTIVE_LOG_NAME)
        self.targets = parse_target_bytes_sequence(config.target_bytes_sequence, config.target_bytes)
        self.log = MinuteRollingFile(self.log_path, clock)
        self.sequence = 0
        self.completed_period = ""
        self.completed_periods = 0
        self.status(
            "INFO",
            f"writer_ready run_id={config.run_id} path={self.log_path} rotation=log4j2-minute "
            f"target_bytes={config.target_bytes} target_bytes_sequence=[{', '.join(str(t) for t in self.targets)}] "
            f"head_pause_ms={config.head_pause_ms} writer=python",
            MAIN_THREAD_NAME,
        )

    def status(self, level, message, thread=THREAD_NAME):
        self.console.log(self.clock.now_ms(), level, STATUS_LOGGER, message, thread)

    def write_current_period(self):
        period = period_of(self.clock.now_ms())
        if period == self.completed_period:
            return
        if self.sequence == 0 and should_defer_initial_period(
            self.clock.now_ms(), self.config.head_pause_ms, self.config.initial_fill_runway_ms
        ):
            return

        period_target = self.targets[self.completed_periods % len(self.targets)]
        self.write_record(period, "head", 1, period_target)
        self.clock.sleep_ms(self.config.head_pause_ms)

        records = 1
        while True:
            size = os.stat(self.log_path).st_size
            if size >= period_target:
                break
            if period != period_of(self.clock.now_ms()):
                self.status(
                    "WARN",
                    f"period_changed_before_target run_id={self.config.run_id} period={period} "
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
            self.write_record(period, "fill", records, period_target)

        self.completed_period = period
        self.completed_periods += 1
        self.status(
            "INFO",
            f"period_complete run_id={self.config.run_id} period={period} target_bytes={period_target} "
            f"records={records} bytes={os.stat(self.log_path).st_size} "
            f"first_sequence={self.sequence - records + 1} last_sequence={self.sequence}",
        )

    def write_record(self, period, phase, record, period_target):
        self.sequence += 1
        message = (
            f"run_id={self.config.run_id} period={period} sequence={self.sequence} record={record} "
            f"phase={phase} target_bytes={period_target} host={self.config.host} payload={PAYLOAD}"
        )
        if self.sequence % 20 == 0:
            level = "ERROR"
        elif self.sequence % 10 == 0:
            level = "WARN"
        else:
            level = "INFO"
        event_ms = self.clock.now_ms()
        # The data logger writes to the console and to the rolling file.
        self.console.log(event_ms, level, DATA_LOGGER, message)
        line = format_line(event_ms, level, DATA_LOGGER, self.console.pid, message)
        try:
            self.log.append(event_ms, line.encode())
        except OSError as error:
            # Log4j2 appenders ignore their exceptions by default: the record
            # is lost and the writer carries on.
            print(f"AppRollingFile lost sequence {self.sequence}: {error}", file=sys.stderr, flush=True)

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


def run(writer, clock, config, done=None):
    """Spring's @Scheduled(fixedDelay, initialDelay): a failed tick is logged
    and the next one still runs."""
    clock.sleep_ms(config.initial_delay_ms)
    while done is None or not done():
        try:
            writer.write_current_period()
        except Exception:
            writer.console.log(clock.now_ms(), "ERROR", SCHEDULER_LOGGER, "Unexpected error occurred in scheduled task")
            traceback.print_exc()
            sys.stderr.flush()
        clock.sleep_ms(config.interval_ms)


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


def selftest_main(args):
    parser = argparse.ArgumentParser(prog="logwriter.py selftest")
    parser.add_argument("--start", required=True, help="UTC start time, as 2026-08-13T12:00:10Z")
    parser.add_argument("--rotations", type=int, required=True, help="stop once this many files are rotated")
    options = parser.parse_args(args)
    start = datetime.datetime.strptime(options.start, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=datetime.timezone.utc)
    clock = SimulatedClock(int(start.timestamp()) * 1000)
    config = Config(os.environ)
    # The writer is PID 1 in its container.
    writer = LogWriter(config, clock, Console(sys.stdout, 1))
    run(writer, clock, config, done=lambda: writer.log.rotations >= options.rotations)
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
    run(LogWriter(config, clock, Console(sys.stdout, os.getpid())), clock, config)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
