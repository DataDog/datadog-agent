"""Workspace bootstrap helpers.

One-shot health checks + auto-install for the deps the coordinator harness
needs in a fresh workspace. Designed so `dda inv q.coord-up` can take a
clean container from zero to a running driver.

Each check returns a `Check` describing the dep, whether it's present, a
version or hint string, and (for auto-installable deps) the command we'd
run to install it. `run_all_checks()` is the single entry point.

Auto-install policy (mode="auto"):
  - safe: `pip install` into the active Python — fully reversible
  - linux dda: curl latest tarball from DataDog/datadog-agent-dev, extract,
    move to /usr/local/bin (needs sudo if the dir isn't writable)
  - macOS dda: brew install --cask dda
  - gh: brew on macOS / apt or yum on Linux (tries with sudo if non-root)
  - go: NEVER auto-installed (big toolchain; clear instructions only)

Pure functions where possible so the q.coord-setup task is just a thin
wrapper that prints + decides exit code.
"""

from __future__ import annotations

import os
import platform
import shutil
import subprocess
import sys
from dataclasses import dataclass


@dataclass
class Check:
    name: str
    ok: bool
    version_or_msg: str
    install_hint: str = ""        # human-readable suggestion when ok=False
    auto_installable: bool = False  # True → run_all_checks(install=True) will try


def _run(cmd: list[str], timeout: int = 30) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)


def check_python() -> Check:
    v = sys.version_info
    s = f"{v.major}.{v.minor}.{v.micro}"
    ok = (v.major, v.minor) >= (3, 10)
    return Check("python", ok, s, install_hint="Python 3.10+ required" if not ok else "")


def check_invoke() -> Check:
    try:
        import invoke  # noqa: F401
        return Check("invoke", True, getattr(invoke, "__version__", "?"))
    except ImportError:
        return Check(
            "invoke", False, "missing",
            install_hint=f"{sys.executable} -m pip install invoke",
            auto_installable=True,
        )


def check_claude_sdk() -> Check:
    try:
        import claude_agent_sdk  # noqa: F401
        return Check("claude-agent-sdk", True, getattr(claude_agent_sdk, "__version__", "?"))
    except ImportError:
        return Check(
            "claude-agent-sdk", False, "missing",
            install_hint=f"{sys.executable} -m pip install claude-agent-sdk",
            auto_installable=True,
        )


def check_dda() -> Check:
    path = shutil.which("dda")
    if not path:
        is_mac = platform.system() == "Darwin"
        hint = (
            "brew install --cask dda"
            if is_mac
            else "curl -L https://github.com/DataDog/datadog-agent-dev/releases/latest/download/dda-x86_64-unknown-linux-gnu.tar.gz | tar xz && sudo mv dda /usr/local/bin/"
        )
        return Check("dda", False, "not on PATH", install_hint=hint, auto_installable=True)
    try:
        r = _run([path, "--version"], timeout=10)
        return Check("dda", True, r.stdout.strip() or "installed")
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return Check("dda", False, "broken install", install_hint=f"reinstall: see {path}")


def check_go() -> Check:
    path = shutil.which("go")
    if not path:
        is_mac = platform.system() == "Darwin"
        hint = (
            "brew install go"
            if is_mac
            else "see https://go.dev/dl/ — install Go 1.21+ before continuing"
        )
        # Not auto-installable — go is a substantial toolchain; we don't
        # silently install it. User installs, re-runs.
        return Check("go", False, "not on PATH", install_hint=hint, auto_installable=False)
    try:
        r = _run([path, "version"], timeout=10)
        return Check("go", True, r.stdout.strip())
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return Check("go", False, "broken install", install_hint="reinstall Go")


def check_gh() -> Check:
    path = shutil.which("gh")
    if not path:
        is_mac = platform.system() == "Darwin"
        hint = "brew install gh" if is_mac else "see https://cli.github.com/manual/installation"
        return Check("gh", False, "not on PATH", install_hint=hint, auto_installable=True)
    return Check("gh", True, "installed")


def check_gh_auth() -> Check:
    if not shutil.which("gh"):
        return Check("gh-auth", False, "gh missing", install_hint="install gh first")
    try:
        r = _run(["gh", "auth", "status"], timeout=15)
    except subprocess.TimeoutExpired:
        return Check("gh-auth", False, "timeout", install_hint="check network")
    if r.returncode != 0:
        return Check(
            "gh-auth", False, "not authenticated",
            install_hint="gh auth login",
        )
    # status output goes to stderr; first line typically "Logged in to github.com as <user>"
    summary = ((r.stdout or "") + (r.stderr or "")).splitlines()
    line = next((ln.strip() for ln in summary if "Logged in" in ln), "authenticated")
    return Check("gh-auth", True, line)


def check_anthropic_key() -> Check:
    val = os.environ.get("ANTHROPIC_API_KEY", "")
    if not val:
        return Check(
            "ANTHROPIC_API_KEY", False, "unset",
            install_hint="export ANTHROPIC_API_KEY=sk-ant-...",
        )
    # Don't print the key. Just confirm it exists and looks plausible.
    redacted = f"{val[:8]}…(len={len(val)})"
    return Check("ANTHROPIC_API_KEY", True, redacted)


def check_git_repo(root: str = ".") -> Check:
    try:
        r = _run(["git", "-C", root, "rev-parse", "--is-inside-work-tree"], timeout=5)
        ok = r.returncode == 0 and r.stdout.strip() == "true"
        return Check("git-repo", ok, "ok" if ok else "not a git repo")
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return Check("git-repo", False, "git missing", install_hint="install git")


# Order matters for printing — start with cheap stuff, end with auth/key.
_ALL_CHECKS = (
    check_python,
    check_git_repo,
    check_invoke,
    check_claude_sdk,
    check_go,
    check_dda,
    check_gh,
    check_gh_auth,
    check_anthropic_key,
)


def run_all_checks() -> list[Check]:
    return [fn() for fn in _ALL_CHECKS]


def _try_pip_install(pkg: str) -> tuple[bool, str]:
    try:
        r = _run([sys.executable, "-m", "pip", "install", pkg], timeout=180)
        return r.returncode == 0, (r.stderr or r.stdout)[-400:]
    except subprocess.TimeoutExpired:
        return False, "pip install timed out"


def _try_install_dda_linux() -> tuple[bool, str]:
    """curl-tarball install for dda on Linux. Best-effort.

    Releases: DataDog/datadog-agent-dev — the asset name pattern is
    `dda-<arch>-unknown-linux-gnu.tar.gz`. We pick by uname.
    """
    arch_map = {"x86_64": "x86_64", "aarch64": "aarch64", "arm64": "aarch64"}
    arch = arch_map.get(platform.machine(), "x86_64")
    asset = f"dda-{arch}-unknown-linux-gnu.tar.gz"
    url = f"https://github.com/DataDog/datadog-agent-dev/releases/latest/download/{asset}"
    # Single-shot pipeline: curl -L | tar xz -C /tmp
    cmd = (
        f"curl -fsSL {url} -o /tmp/dda.tar.gz && "
        f"tar xzf /tmp/dda.tar.gz -C /tmp && "
        f"sudo mv /tmp/dda /usr/local/bin/dda && "
        f"sudo chmod +x /usr/local/bin/dda"
    )
    try:
        r = subprocess.run(["bash", "-lc", cmd], capture_output=True, text=True, timeout=180)
        return r.returncode == 0, (r.stderr or r.stdout)[-400:]
    except subprocess.TimeoutExpired:
        return False, "dda install timed out"


def _try_install_dda_mac() -> tuple[bool, str]:
    try:
        r = _run(["brew", "install", "--cask", "dda"], timeout=300)
        return r.returncode == 0, (r.stderr or r.stdout)[-400:]
    except (FileNotFoundError, subprocess.TimeoutExpired) as e:
        return False, f"brew install failed: {e}"


def _try_install_gh() -> tuple[bool, str]:
    if platform.system() == "Darwin":
        try:
            r = _run(["brew", "install", "gh"], timeout=300)
            return r.returncode == 0, (r.stderr or r.stdout)[-400:]
        except (FileNotFoundError, subprocess.TimeoutExpired) as e:
            return False, f"brew install failed: {e}"
    # Linux: try apt then yum.
    for cmd_str in (
        "sudo apt-get update -y && sudo apt-get install -y gh",
        "sudo yum install -y gh",
    ):
        try:
            r = subprocess.run(["bash", "-lc", cmd_str], capture_output=True, text=True, timeout=300)
            if r.returncode == 0:
                return True, "installed"
        except subprocess.TimeoutExpired:
            continue
    return False, "tried apt + yum — both failed; install gh manually"


def auto_install(check: Check) -> tuple[bool, str]:
    """Attempt to install one missing dep. Returns (ok, message)."""
    if not check.auto_installable or check.ok:
        return False, "not auto-installable or already ok"
    name = check.name
    if name in ("invoke", "claude-agent-sdk"):
        return _try_pip_install(name)
    if name == "dda":
        return _try_install_dda_mac() if platform.system() == "Darwin" else _try_install_dda_linux()
    if name == "gh":
        return _try_install_gh()
    return False, f"no installer for {name}"


def format_report(checks: list[Check]) -> str:
    """Pretty-print a check list. ANSI colour."""
    GREEN, RED, YELLOW, RESET = "\033[32m", "\033[31m", "\033[33m", "\033[0m"
    lines: list[str] = []
    for c in checks:
        if c.ok:
            lines.append(f"  {GREEN}✓{RESET} {c.name:24s} {c.version_or_msg}")
        else:
            mark = f"{RED}✗{RESET}" if not c.auto_installable else f"{YELLOW}⚠{RESET}"
            lines.append(f"  {mark} {c.name:24s} {c.version_or_msg}")
            if c.install_hint:
                lines.append(f"      → {c.install_hint}")
    return "\n".join(lines)


def blockers(checks: list[Check]) -> list[Check]:
    """Return checks that must be resolved before the coordinator can run."""
    return [c for c in checks if not c.ok]
