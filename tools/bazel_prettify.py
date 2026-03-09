#!/usr/bin/env python3
"""Bazel output prettifier with optional Claude error analysis.

Invoked by tools/bazel wrapper. Pipes bazel's stderr through a colorizer
and, on failure, optionally sends error context to Claude for diagnosis
when ANTHROPIC_API_KEY is set.

Disable entirely with BAZEL_NO_PRETTIFY=1.
"""

import json
import os
import re
import shutil
import subprocess
import sys
import textwrap
from urllib.error import URLError
from urllib.request import Request, urlopen

# ── ANSI codes ──────────────────────────────────────────────────────────

RESET = "\033[0m"
BOLD = "\033[1m"
DIM = "\033[2m"
ITALIC = "\033[3m"
UNDERLINE = "\033[4m"
RED = "\033[31m"
GREEN = "\033[32m"
YELLOW = "\033[33m"
BLUE = "\033[34m"
MAGENTA = "\033[35m"
CYAN = "\033[36m"
BOLD_RED = "\033[1;31m"
BOLD_GREEN = "\033[1;32m"
BOLD_YELLOW = "\033[1;33m"
BOLD_BLUE = "\033[1;34m"
BOLD_MAGENTA = "\033[1;35m"
BOLD_CYAN = "\033[1;36m"

ANSI_RE = re.compile(r"\033\[[0-9;]*[a-zA-Z]")

try:
    TERM_WIDTH = shutil.get_terminal_size().columns
except (ValueError, OSError):
    TERM_WIDTH = 120


def strip_ansi(s):
    return ANSI_RE.sub("", s)


# ── Regex patterns ──────────────────────────────────────────────────────

# Full target label: @repo//pkg:target, @@canonical+name//:target, //pkg:target
TARGET_RE = re.compile(r"(@@?[\w.+-]+(?:\+[\w.+-]+)*)?//[\w/._-]*(?::[\w/._+-]+)")
# Standalone external repo ref: @repo or @@canonical+ext (not followed by //)
STANDALONE_REPO_RE = re.compile(r"(?<![/\w])@@?[\w.+-]+(?:\+[\w.+-]+)*(?![\w.+-]*//)")
PROGRESS_RE = re.compile(r"\[[\d,]+ / [\d,]+\]")
STRATEGY_RE = re.compile(
    r"\b(local|remote|worker|remote-cache|linux-sandbox" r"|processwrapper-sandbox|docker-sandbox)\b"
)
FROM_RE = re.compile(r"^INFO: From (\S+)(.*)$")
BUILD_OK_RE = re.compile(r"Build completed successfully")
BUILD_FAIL_RE = re.compile(r"Build did NOT complete successfully")

PREFIXES = [
    ("ERROR:", BOLD_RED),
    ("FATAL:", BOLD_RED),
    ("FAILED:", BOLD_RED),
    ("WARNING:", MAGENTA),
    ("INFO:", GREEN),
    ("DEBUG:", YELLOW),
]

_FILE_REF = re.compile(r"([\w/.+-]{2,}):(\d+)")


# ── Line wrapping ──────────────────────────────────────────────────────


def _wrap(text, width=None, indent="    "):
    """Wrap a long line, indenting continuations."""
    if width is None:
        width = TERM_WIDTH
    if len(text) <= width:
        return text
    return textwrap.fill(
        text,
        width=width,
        subsequent_indent=indent,
        break_long_words=False,
        break_on_hyphens=False,
    )


# ── Colorization ────────────────────────────────────────────────────────


def _color_target(m):
    """Color a target label — repo prefix in magenta, path:name in cyan."""
    repo = m.group(1) or ""
    rest = m.group()[len(repo) :]
    if repo:
        return BOLD_MAGENTA + repo + BOLD_CYAN + rest + RESET
    return BOLD_CYAN + m.group() + RESET


def _highlight(text):
    """Color target labels, external repos, and execution strategies."""
    out = TARGET_RE.sub(_color_target, text)
    out = STANDALONE_REPO_RE.sub(lambda m: BOLD_MAGENTA + m.group() + RESET, out)
    out = STRATEGY_RE.sub(lambda m: BLUE + m.group() + RESET, out)
    return out


def colorize(line):
    """Apply color coding and wrapping to a single line of Bazel stderr."""
    raw = strip_ansi(line).rstrip("\n")
    if not raw:
        return "\n"

    if BUILD_OK_RE.search(raw):
        return BOLD_GREEN + raw + RESET + "\n"
    if BUILD_FAIL_RE.search(raw):
        return BOLD_RED + raw + RESET + "\n"

    # "INFO: From <mnemonic> <rest>"
    m = FROM_RE.match(raw)
    if m:
        rest = _wrap(m.group(2), indent="        ")
        return f"{GREEN}INFO:{RESET} From {BOLD_YELLOW}{m.group(1)}{RESET}{_highlight(rest)}\n"

    for pfx, style in PREFIXES:
        if raw.startswith(pfx):
            wrapped = _wrap(raw, indent="    ")
            parts = wrapped.split("\n")
            colored = style + pfx + RESET + _highlight(parts[0][len(pfx) :])
            for cont in parts[1:]:
                colored += "\n" + _highlight(cont)
            return colored + "\n"

    pm = PROGRESS_RE.search(raw)
    if pm:
        return raw[: pm.start()] + BOLD + pm.group() + RESET + _highlight(raw[pm.end() :]) + "\n"

    return _highlight(raw) + "\n"


# ── Error collector ─────────────────────────────────────────────────────

_ERROR_STARTS = ("ERROR:", "FAILED:", "FATAL:")
_OTHER_STARTS = ("INFO:", "WARNING:", "DEBUG:")


class ErrorCollector:
    """Accumulates error blocks (ERROR/FAILED line + subsequent output)."""

    def __init__(self):
        self.blocks = []
        self._cur = None

    def feed(self, raw):
        line = strip_ansi(raw).rstrip()
        if line.startswith(_ERROR_STARTS):
            self._save()
            self._cur = [line]
        elif self._cur is not None:
            if line.startswith(_OTHER_STARTS) or PROGRESS_RE.match(line):
                self._save()
            else:
                self._cur.append(line)

    def _save(self):
        if self._cur:
            self.blocks.append("\n".join(self._cur))
        self._cur = None

    def finalize(self):
        self._save()

    @property
    def text(self):
        return "\n\n".join(self.blocks)

    @property
    def has_errors(self):
        return bool(self.blocks)


# ── Source file detection ───────────────────────────────────────────────


def _detect_files(error_text, limit=5):
    """Find source/BUILD/bzl files referenced in error output."""
    seen, out = set(), []

    for m in _FILE_REF.finditer(error_text):
        p = m.group(1)
        if p not in seen and os.path.isfile(p):
            seen.add(p)
            out.append(p)
        if len(out) >= limit:
            return out

    # BUILD files inferred from target labels (//pkg:target -> pkg/BUILD.bazel)
    for m in TARGET_RE.finditer(error_text):
        target = m.group()
        if target.startswith("@"):
            continue
        pkg = target.lstrip("/").split(":")[0]
        for name in ("BUILD.bazel", "BUILD"):
            bp = os.path.join(pkg, name)
            if bp not in seen and os.path.isfile(bp):
                seen.add(bp)
                out.append(bp)
                break
        if len(out) >= limit:
            return out

    return out


def _read_snippet(path, max_lines=150):
    try:
        with open(path) as f:
            lines = []
            for i, ln in enumerate(f, 1):
                if i > max_lines:
                    lines.append(f"... truncated at {max_lines} lines ...")
                    break
                lines.append(f"{i:4d} | {ln.rstrip()}")
            return "\n".join(lines)
    except OSError:
        return f"(could not read {path})"


# ── Markdown → ANSI renderer ───────────────────────────────────────────

_MD_BOLD = re.compile(r"\*\*(.+?)\*\*")
_MD_ITALIC = re.compile(r"(?<!\*)\*([^*]+)\*(?!\*)")
_MD_CODE = re.compile(r"`([^`]+)`")


def _render_markdown(text):
    """Convert Claude's markdown response to ANSI-formatted terminal text."""
    lines = text.split("\n")
    result = []
    in_code = False

    for line in lines:
        # Code fence
        if line.startswith("```"):
            in_code = not in_code
            if in_code:
                lang = line[3:].strip()
                label = f" {lang} " if lang else ""
                border = "\u2500" * max(1, 38 - len(label))
                result.append(f"{DIM}  \u250c\u2500{label}{border}{RESET}")
            else:
                result.append(f"{DIM}  \u2514{'\u2500' * 40}{RESET}")
            continue

        if in_code:
            result.append(f"{DIM}  \u2502 {line}{RESET}")
            continue

        # Headers
        if line.startswith("### "):
            result.append(BOLD_YELLOW + "  " + line[4:] + RESET)
            continue
        if line.startswith("## "):
            result.append(BOLD_YELLOW + line[3:] + RESET)
            continue
        if line.startswith("# "):
            result.append(BOLD + UNDERLINE + line[2:] + RESET)
            continue

        # Inline formatting
        line = _MD_BOLD.sub(BOLD + r"\1" + RESET, line)
        line = _MD_ITALIC.sub(ITALIC + r"\1" + RESET, line)
        line = _MD_CODE.sub(CYAN + r"\1" + RESET, line)

        # Bullet lists
        stripped = line.lstrip()
        indent = len(line) - len(stripped)
        if stripped.startswith("- "):
            line = f"{' ' * indent}\u2022 {stripped[2:]}"

        # Wrap long prose lines
        visible = strip_ansi(line)
        if len(visible) > TERM_WIDTH:
            line = _wrap(visible, indent="  ")
            line = _MD_BOLD.sub(BOLD + r"\1" + RESET, line)
            line = _MD_ITALIC.sub(ITALIC + r"\1" + RESET, line)
            line = _MD_CODE.sub(CYAN + r"\1" + RESET, line)

        result.append(line)

    return "\n".join(result)


# ── Claude analysis ────────────────────────────────────────────────────

_MAX_ERROR_CHARS = 4000
_MAX_FILE_CHARS = 8000


def _call_claude(error_text, source_files, bazel_args):
    api_key = os.environ.get("ANTHROPIC_API_KEY", "")
    if not api_key:
        return

    model = os.environ.get("ANTHROPIC_MODEL", "claude-sonnet-4-20250514")

    file_sections = []
    chars = 0
    for p in source_files:
        snippet = _read_snippet(p)
        if chars + len(snippet) > _MAX_FILE_CHARS:
            break
        file_sections.append(f"### {p}\n```\n{snippet}\n```")
        chars += len(snippet)

    cmd = " ".join(["bazel"] + bazel_args)
    prompt = (
        f"Analyze this Bazel build error from `{cmd}`. "
        "Diagnose the root cause and suggest a concise fix.\n\n"
        "## Error output\n```\n" + error_text[:_MAX_ERROR_CHARS] + "\n```\n\n"
        "## Referenced files\n"
        + ("\n\n".join(file_sections) if file_sections else "(none detected)")
        + "\n\nProvide:\n1. Root cause\n2. Suggested fix\n"
    )

    body = json.dumps(
        {
            "model": model,
            "max_tokens": 1024,
            "messages": [{"role": "user", "content": prompt}],
        }
    ).encode()

    req = Request(
        "https://api.anthropic.com/v1/messages",
        data=body,
        headers={
            "Content-Type": "application/json",
            "x-api-key": api_key,
            "anthropic-version": "2023-06-01",
        },
    )

    try:
        sys.stderr.write(f"\n{DIM}Analyzing build errors with Claude...{RESET}\n")
        sys.stderr.flush()
        with urlopen(req, timeout=60) as resp:
            data = json.loads(resp.read())
            analysis = data["content"][0]["text"]

        bar = "\u2500" * min(72, TERM_WIDTH - 2)
        sys.stderr.write(f"\n{BOLD}{bar}{RESET}\n")
        sys.stderr.write(f"{BOLD_CYAN}\U0001f50d Claude Build Error Analysis{RESET}\n")
        sys.stderr.write(f"{BOLD}{bar}{RESET}\n\n")
        sys.stderr.write(_render_markdown(analysis) + "\n\n")
        sys.stderr.write(f"{BOLD}{bar}{RESET}\n")
        sys.stderr.flush()
    except (URLError, OSError, KeyError, json.JSONDecodeError) as exc:
        sys.stderr.write(f"\n{DIM}(Claude analysis unavailable: {exc}){RESET}\n")
        sys.stderr.flush()


# ── Main ────────────────────────────────────────────────────────────────


def main():
    if len(sys.argv) < 2:
        print("Usage: bazel_prettify.py BAZEL_REAL [args...]", file=sys.stderr)
        sys.exit(2)

    bazel = sys.argv[1]
    args = sys.argv[2:]
    tty = sys.stderr.isatty()

    proc = subprocess.Popen(
        [bazel] + args,
        stdin=sys.stdin,
        stdout=sys.stdout,
        stderr=subprocess.PIPE,
    )

    collector = ErrorCollector()

    try:
        while True:
            line = proc.stderr.readline()
            if not line:
                break
            decoded = line.decode("utf-8", errors="replace")
            collector.feed(decoded)
            sys.stderr.write(colorize(decoded) if tty else decoded)
            sys.stderr.flush()
    except KeyboardInterrupt:
        pass  # bazel receives SIGINT from the same process group

    rc = proc.wait()
    collector.finalize()

    if rc != 0 and collector.has_errors and os.environ.get("ANTHROPIC_API_KEY"):
        files = _detect_files(collector.text)
        _call_claude(collector.text, files, args)

    sys.exit(rc)


if __name__ == "__main__":
    main()
