#!/usr/bin/env python3
"""The ledger and appender sidecars of the Azure Files E2E writer, in Python.

A writer pod runs ledger.sh and appender.sh next to logwriter.py. The Windows
file server of the smb-windows cell has no POSIX shell to run them, so it runs
this port of both, on the same Python as its writer. Each command reads the
LOGWRITER_* variables of its shell script, prints the same status lines and
writes the same JSON lines:

- ledger: ledger.sh with LOGWRITER_LEDGER_SOURCE=journal. Every line the writer
  appended to its journal (periods.jsonl) before it renamed, copied, truncated,
  compressed or deleted a file is recorded once in ledger.jsonl, with what the
  directory holds for that file when the ledger looks: "file", "archive" (only
  its .gz is left), "deleted" (by design) or "missing", and its size. The scan
  source of the Java image is not ported: it needs the image's Crc64 class;
- appender: appender.sh. Each newly rotated file gets one post-rename marker
  line per age of LOGWRITER_APPEND_DELAYS_MS, and each attempt is journalled in
  markers.jsonl. Rotated files that predate the appender, compressed files and
  rotated files that are gone get no marker, and an append never creates a
  file. LOGWRITER_APPEND_DELAYS_MS=none idles.

With LOGWRITER_STREAMS=N, both work on each stream directory, svc-1 to svc-<N>,
on its own, as the shell scripts do.

Windows refuses to open a file for an append while another process holds it
open without sharing write access, so every append is retried briefly.

Commands:

    sidecars.py ledger
    sidecars.py appender
"""

import datetime
import json
import os
import re
import signal
import sys
import threading
import time

DEFAULT_LOG_DIR = "/mnt/azure-files"
ACTIVE_LOG_NAME = "app.log"
LEDGER_NAME = "ledger.jsonl"
JOURNAL_NAME = "periods.jsonl"
MARKER_JOURNAL_NAME = "markers.jsonl"
STREAM_DIR_PREFIX = "svc-"
APPEND_ATTEMPTS = 5
APPEND_RETRY_SECONDS = 0.02
NUMBER_PATTERN = re.compile(r"[0-9]+")
# appender.sh splits its delays on commas and blanks.
DELAY_SEPARATOR = re.compile(r"[,\s]+")
# Text mode would write every newline as CRLF on Windows.
O_BINARY = getattr(os, "O_BINARY", 0)


def utc_now():
    """The time as `date -u '+%Y-%m-%dT%H:%M:%SZ'` prints it."""
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def say(message, stream=None):
    print(message, file=stream or sys.stdout, flush=True)


def fatal(component, reason, **fields):
    details = "".join(f" {key}={value}" for key, value in fields.items())
    say(f"{component}_fatal reason={reason}{details}", sys.stderr)
    sys.exit(1)


def read_streams(environ, component):
    streams = environ.get("LOGWRITER_STREAMS", "") or "0"
    if not NUMBER_PATTERN.fullmatch(streams):
        fatal(component, "invalid_streams", streams=streams)
    return int(streams)


def stream_dirs(log_dir, streams):
    """The stream directories: the log directory itself, or svc-1 to svc-<N>."""
    if streams == 0:
        return [log_dir]
    return [os.path.join(log_dir, STREAM_DIR_PREFIX + str(index)) for index in range(1, streams + 1)]


def append_line(path, line, create=True):
    """Appends one line in a single write, retrying while another process holds
    the file. Without create, a file that is gone stays gone."""
    flags = os.O_WRONLY | os.O_APPEND | O_BINARY
    if create:
        flags |= os.O_CREAT
    data = line.encode()
    for attempt in range(APPEND_ATTEMPTS):
        try:
            fd = os.open(path, flags, 0o666)
        except PermissionError:
            if attempt == APPEND_ATTEMPTS - 1:
                raise
            time.sleep(APPEND_RETRY_SECONDS)
            continue
        try:
            view = memoryview(data)
            while view:
                view = view[os.write(fd, view) :]
        finally:
            os.close(fd)
        return


def file_bytes(path):
    try:
        return os.stat(path).st_size
    except OSError:
        return -1


class StreamLedger:
    """The ledger of one stream directory. Each journal line is recorded once,
    keyed by its period, like the seen files of ledger.sh; the periods already
    in the ledger count as seen, so a restarted ledger records nothing twice."""

    def __init__(self, directory, ledger_path, journal_name):
        self.directory = directory
        self.ledger_path = ledger_path
        self.journal_path = os.path.join(directory, journal_name)
        self.seen = set()
        try:
            with open(ledger_path, "rb") as ledger:
                for raw in ledger:
                    try:
                        entry = json.loads(raw)
                    except ValueError:
                        continue
                    if isinstance(entry, dict) and entry.get("period"):
                        self.seen.add(entry["period"])
        except FileNotFoundError:
            pass

    def record_journal(self):
        """Records the journal lines not recorded yet, and returns whether the
        journal exists."""
        try:
            with open(self.journal_path, "rb") as journal:
                content = journal.read()
        except FileNotFoundError:
            return False
        # A last line without its newline is still being written: it is
        # recorded on the next poll.
        for raw in content.split(b"\n")[:-1]:
            line = raw.decode("utf-8", "replace").rstrip("\r")
            if not line:
                continue
            if not (line.startswith("{") and line.endswith("}")):
                say(f"ledger_skip journal={self.journal_path} reason=not_an_object", sys.stderr)
                continue
            self.record_journal_line(line)
        return True

    def record_journal_line(self, line):
        try:
            entry = json.loads(line)
        except ValueError:
            entry = None
        if not isinstance(entry, dict):
            say(f"ledger_skip journal={self.journal_path} reason=not_an_object", sys.stderr)
            return
        period = entry.get("period")
        if not isinstance(period, str) or not period:
            say(f"ledger_skip journal={self.journal_path} reason=no_period", sys.stderr)
            return
        if period in self.seen:
            return

        discovered_at = utc_now()
        file_name = entry.get("file") or ""
        archive = entry.get("archive") or ""
        disposition = entry.get("disposition") or ""
        observed, observed_bytes = "missing", -1
        if disposition == "deleted":
            observed = "deleted"
        elif file_name and os.path.isfile(os.path.join(self.directory, file_name)):
            observed, observed_bytes = "file", file_bytes(os.path.join(self.directory, file_name))
        elif archive and os.path.isfile(os.path.join(self.directory, archive)):
            observed, observed_bytes = "archive", file_bytes(os.path.join(self.directory, archive))
        observed_at = utc_now()

        # The journal line is one JSON object: append the observation to it,
        # as ledger.sh does.
        recorded = (
            f'{line[:-1]},"discovered_at":"{discovered_at}","observed_at":"{observed_at}",'
            f'"observed":"{observed}","observed_bytes":{observed_bytes}}}\n'
        )
        try:
            append_line(self.ledger_path, recorded)
        except OSError as error:
            say(f"ledger_write_failed path={self.ledger_path} period={period} error={error}", sys.stderr)
            return
        self.seen.add(period)
        say(f"ledger_recorded file={file_name} period={period} disposition={disposition} observed={observed}")


def ledger_main(environ):
    log_dir = environ.get("LOGWRITER_LOG_DIR", "") or DEFAULT_LOG_DIR
    source = environ.get("LOGWRITER_LEDGER_SOURCE", "") or "journal"
    if source != "journal":
        fatal("ledger", "unknown_source", source=source)
    streams = read_streams(environ, "ledger")
    journal_name = environ.get("LOGWRITER_JOURNAL_NAME", "") or JOURNAL_NAME
    poll_seconds = float(environ.get("LOGWRITER_LEDGER_POLL_SECONDS", "") or "1")

    ledgers = []
    for directory in stream_dirs(log_dir, streams):
        ledger_path = os.path.join(directory, LEDGER_NAME)
        # The override only applies to the single stream at the root.
        if streams == 0:
            ledger_path = environ.get("LOGWRITER_LEDGER_PATH", "") or ledger_path
        ledgers.append(StreamLedger(directory, ledger_path, journal_name))

    while True:
        found = False
        for ledger in ledgers:
            if ledger.record_journal():
                found = True
        if not found:
            say(f"ledger_waiting directory={log_dir} source={source}")
        time.sleep(poll_seconds)


class MarkerWatcher:
    """Watches one stream directory for rotated files and appends their
    markers, like watch_dir in appender.sh."""

    def __init__(self, directory, run_id, journal_path, delays_ms, delays_text, poll_ms):
        self.directory = directory
        self.run_id = run_id
        self.journal_path = journal_path
        self.delays_ms = delays_ms
        self.delays_text = delays_text
        self.poll_ms = poll_ms
        self.journal_lock = threading.Lock()
        self.seen = set()
        self.rotation = 0

    def rotated_names(self):
        try:
            names = sorted(os.listdir(self.directory))
        except FileNotFoundError:
            return []
        rotated = []
        for name in names:
            # A compressed rotated file is not one the writer rotated.
            if not name.startswith(ACTIVE_LOG_NAME + ".") or name.endswith(".gz"):
                continue
            if os.path.isfile(os.path.join(self.directory, name)):
                rotated.append(name)
        return rotated

    def run(self):
        say(
            f"appender_ready run_id={self.run_id} directory={self.directory} delays_ms={self.delays_text} "
            f"poll_ms={self.poll_ms} nap_mode=python"
        )
        # Rotated files that predate the appender are recorded as seen without
        # markers: appending to them would produce markers whose real age is
        # minutes, not milliseconds.
        primed = False
        while True:
            for name in self.rotated_names():
                if name in self.seen:
                    continue
                self.seen.add(name)
                if not primed:
                    say(f"appender_primed file={name} reason=predates_container")
                    continue
                self.rotation += 1
                say(f"appender_rotation_detected file={name} rotation={self.rotation}")
                # One thread per rotation, so markers keep their ages when
                # rotations come faster than the oldest marker age.
                threading.Thread(
                    target=self.append_markers,
                    args=(os.path.join(self.directory, name), name, self.rotation, time.monotonic()),
                    daemon=True,
                ).start()
            primed = True
            time.sleep(self.poll_ms / 1000)

    def append_markers(self, path, name, rotation, detected_at):
        for delay_ms in self.delays_ms:
            remaining = detected_at + delay_ms / 1000 - time.monotonic()
            if remaining > 0:
                time.sleep(remaining)
            marker_id = f"{self.run_id}-r{rotation}-m{delay_ms}"
            fields = f"file={name} rotation={rotation} marker_age_ms={delay_ms} marker_id={marker_id}"
            # The append targets the rotated path, so it lands on the file the
            # draining reader is still finishing, not on the new active file.
            marker = (
                f"post_rotation_marker run_id={self.run_id} rotation={rotation} marker_age_ms={delay_ms} "
                f"marker_id={marker_id} rotated_file={name}\n"
            )
            try:
                append_line(path, marker, create=False)
            except FileNotFoundError:
                # A rotated file that was compressed or deleted since gets no
                # marker, and no file of the marker's own.
                self.journal("skipped", rotation, delay_ms, marker_id, name)
                say(f"appender_marker_skipped {fields} reason=file_gone")
                continue
            except OSError as error:
                self.journal("failed", rotation, delay_ms, marker_id, name)
                say(f"appender_marker_failed {fields} error={error}", sys.stderr)
                continue
            self.journal("appended", rotation, delay_ms, marker_id, name)
            say(f"appender_marker_appended {fields}")

    def journal(self, status, rotation, age_ms, marker_id, name):
        line = (
            f'{{"run_id":"{self.run_id}","rotation":{rotation},"marker_age_ms":{age_ms},"marker_id":"{marker_id}",'
            f'"rotated_file":"{name}","appended_at":"{utc_now()}","status":"{status}"}}\n'
        )
        with self.journal_lock:
            try:
                append_line(self.journal_path, line)
            except OSError as error:
                say(f"appender_journal_failed path={self.journal_path} marker_id={marker_id} error={error}", sys.stderr)


def appender_main(environ):
    log_dir = environ.get("LOGWRITER_LOG_DIR", "") or DEFAULT_LOG_DIR
    run_id = environ.get("LOGWRITER_RUN_ID", "") or "unknown"
    streams = read_streams(environ, "appender")
    delays_text = environ.get("LOGWRITER_APPEND_DELAYS_MS", "") or "1500,45000"
    poll_text = environ.get("LOGWRITER_APPEND_POLL_MS", "") or "200"

    if delays_text == "none":
        say(f"appender_idle run_id={run_id} reason=no_markers")
        while True:
            time.sleep(3600)
    delays_ms = []
    for delay in DELAY_SEPARATOR.split(delays_text.strip()):
        if not NUMBER_PATTERN.fullmatch(delay):
            fatal("appender", "invalid_delay", delays_ms=delays_text)
        delays_ms.append(int(delay))
    if not NUMBER_PATTERN.fullmatch(poll_text):
        fatal("appender", "invalid_poll", poll_ms=poll_text)
    poll_ms = int(poll_text)

    if streams == 0:
        journal_path = environ.get("LOGWRITER_MARKER_JOURNAL_PATH", "") or os.path.join(log_dir, MARKER_JOURNAL_NAME)
        MarkerWatcher(log_dir, run_id, journal_path, delays_ms, delays_text, poll_ms).run()
        return
    for directory in stream_dirs(log_dir, streams):
        stream_run_id = run_id + "-" + os.path.basename(directory)
        watcher = MarkerWatcher(
            directory, stream_run_id, os.path.join(directory, MARKER_JOURNAL_NAME), delays_ms, delays_text, poll_ms
        )
        threading.Thread(target=watcher.run, daemon=True).start()
    while True:
        time.sleep(3600)


def main(argv):
    signal.signal(signal.SIGTERM, lambda signum, _frame: sys.exit(128 + signum))
    if len(argv) != 2 or argv[1] not in ("ledger", "appender"):
        say("usage: sidecars.py ledger|appender", sys.stderr)
        return 2
    if argv[1] == "ledger":
        ledger_main(os.environ)
    else:
        appender_main(os.environ)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
