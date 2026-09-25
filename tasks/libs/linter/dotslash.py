"""Validate repository DotSlash manifests without depending on Invoke or Bazel."""

from __future__ import annotations

import hashlib
import json
import os
import platform
import re
import shutil
import subprocess
import tempfile
import tomllib
from contextlib import contextmanager
from pathlib import Path
from urllib.parse import urlsplit

SHEBANG = b"#!/usr/bin/env dotslash"
PLATFORMS = {
    "linux-x86_64": ("linux", "amd64"),
    "linux-aarch64": ("linux", "arm64"),
    "macos-x86_64": ("darwin", "amd64"),
    "macos-aarch64": ("darwin", "arm64"),
    "windows-x86_64": ("windows", "amd64"),
}
MIRROR = "https://depot-read-api-github-releases.us1.ddbuild.io/internal/mirror/github-releases/"
# The generic Windows launcher has no tool-specific artifact pin of its own.
WINDOWS_SHIM_SHA256 = "b6b3e378ae463ad09563472cdc2080bbaef227a277ac9e17e7c394f155646860"
VALIDATOR_INPUTS = {
    "mise.toml",
    "tasks/linter.py",
    "tasks/libs/linter/dotslash.py",
    "tasks/unit_tests/dotslash_tests.py",
    "tasks/unit_tests/dotslash_task_tests.py",
    ".gitlab-ci.yml",
    ".gitlab/build/dotslash.yml",
    ".gitlab/deploy/conditions.yml",
    ".gitlab/.pre/common/macos.yml",
}


def run(argv, *, cwd=None, env=None, combined=False):
    """Preserve argument boundaries on every host and bound external operations."""
    result = subprocess.run(
        argv,
        cwd=cwd,
        env=env,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT if combined else subprocess.PIPE,
        encoding="utf-8",
        errors="replace",
        timeout=300,
        check=False,
    )
    if result.returncode:
        raise ValueError(f"{argv[0]} exited with {result.returncode}:\n{result.stdout}{result.stderr or ''}")
    return result.stdout


def discover(root):
    """Recognize manifests by their official header, allowing unrelated launchers."""
    manifests = []
    for path in sorted((root / "tools/bin").rglob("*")):
        if path.is_file():
            with path.open("rb") as stream:
                if stream.readline(len(SHEBANG) + 3) in (SHEBANG + b"\n", SHEBANG + b"\r\n"):
                    manifests.append(path)
    return manifests


def native_check(document):
    """Validate optional native-check configuration without inventing a command."""
    metadata = document.get("metadata", {})
    if not isinstance(metadata, dict):
        raise ValueError("metadata must be a mapping.")
    if "native_check" not in metadata:
        return None
    check = metadata["native_check"]
    if not isinstance(check, dict) or set(check) - {"command", "pattern"}:
        raise ValueError("metadata.native_check must be a mapping containing only command and pattern.")
    pattern = check.get("pattern")
    if "pattern" in check:
        if not isinstance(pattern, str) or not pattern:
            raise ValueError("The native-check pattern must be a nonempty regular expression.")
        try:
            pattern = re.compile(pattern)
        except re.error as error:
            raise ValueError(f"Invalid native-check pattern: {error}") from error
    if "command" not in check:
        return None
    command = check["command"]
    if (
        not isinstance(command, list)
        or not command
        or any(not isinstance(arg, str) or "\0" in arg for arg in command)
        or command[0] != document["name"]
    ):
        raise ValueError("The native-check command must be an argument array beginning with the manifest's name.")
    if pattern is None:
        raise ValueError("A native-check command requires a pattern.")
    return {"command": command, "pattern": pattern}


def validate_document(document, name):
    """Check repository policy after DotSlash parsing and prepare the native check."""
    if document["name"] != name:
        raise ValueError("The manifest name must match its filename.")
    entries = document["platforms"]
    if set(entries) != set(PLATFORMS):
        raise ValueError(f"The manifest must cover exactly {', '.join(sorted(PLATFORMS))}.")
    versions = set()
    for key, entry in entries.items():
        if entry["size"] <= 0:
            raise ValueError(f"{key}: size must be a positive integer.")
        if entry["hash"] != "sha256":
            raise ValueError(f"{key}: SHA-256 is required.")
        path = entry["path"]
        # DotSlash's Unix parser permits colons, including Windows drive prefixes.
        if ":" in path:
            raise ValueError(f"{key}: executable paths must not contain colons.")
        if "providers_order" in entry:
            raise ValueError(f"{key}: providers must be attempted in their declared list order without an override.")
        providers = entry["providers"]
        if not providers:
            raise ValueError(f"{key}: at least one provider is required.")
        urls = []
        for provider in providers:
            if not isinstance(provider, dict) or provider.get("type", "http") != "http":
                raise ValueError(f"{key}: only HTTP providers are supported in this phase.")
            url = provider.get("url")
            if not isinstance(url, str):
                raise ValueError(f"{key}: providers require a URL.")
            parsed = urlsplit(url)
            if (
                parsed.scheme != "https"
                or not parsed.hostname
                or parsed.username is not None
                or parsed.password is not None
                or parsed.fragment
            ):
                raise ValueError(f"{key}: provider URLs must use HTTPS without credentials or fragments.")
            urls.append(url)
        system, arch = PLATFORMS[key]
        executable = name + (".exe" if system == "windows" else "")
        if name == "buildifier":
            asset = f"buildifier-{system}-{arch}" + (".exe" if system == "windows" else "")
            match = re.fullmatch(
                rf"https://github\.com/bazelbuild/buildtools/releases/download/v([^/]+)/{re.escape(asset)}", urls[-1]
            )
            if not match or urls != [MIRROR + urls[-1].removeprefix("https://github.com/"), urls[-1]]:
                raise ValueError(
                    f"{key}: Buildifier requires the matching mirror and upstream release assets, in order."
                )
            if "format" in entry or path != executable:
                raise ValueError(f"{key}: Buildifier must be a standalone executable named {executable}.")
            versions.add(match[1])
        elif name == "vault":
            match = re.fullmatch(
                rf"https://releases\.hashicorp\.com/vault/([^/]+)/vault_\1_{system}_{arch}\.zip", urls[0]
            )
            if not match or len(urls) != 1 or entry.get("format") != "zip" or path != executable:
                raise ValueError(f"{key}: Vault requires its matching HashiCorp zip and executable path.")
            versions.add(match[1])
    if len(versions) > 1:
        raise ValueError("All platform artifacts must refer to the same release version.")
    return native_check(document)


def selected_manifests(root, manifests, changed_since):
    """Include working-tree changes and select every tool for shared-input changes."""
    if not changed_since:
        return manifests
    base = run(["git", "merge-base", "HEAD", changed_since], cwd=root).strip()
    changed = set(run(["git", "diff", "--name-only", "--no-renames", "-z", base, "--"], cwd=root).split("\0"))
    changed.update(run(["git", "ls-files", "--others", "--exclude-standard", "-z"], cwd=root).split("\0"))
    if changed & VALIDATOR_INPUTS or any(path.startswith("tools/bin/") and path.endswith(".exe") for path in changed):
        return manifests
    return [path for path in manifests if path.relative_to(root).as_posix() in changed]


def host_platform():
    system = {"Darwin": "macos", "Linux": "linux", "Windows": "windows"}.get(platform.system())
    arch = {"AMD64": "x86_64", "x86_64": "x86_64", "arm64": "aarch64", "aarch64": "aarch64"}.get(platform.machine())
    key = f"{system}-{arch}"
    if key not in PLATFORMS:
        raise ValueError(f"Unsupported native platform: {platform.system()} {platform.machine()}.")
    return key


def write_manifest(path, document):
    path.write_bytes(SHEBANG + b"\n" + json.dumps(document).encode("utf-8"))


def fetch(interpreter, manifest, cache):
    output = run([interpreter, "--", "fetch", str(manifest)], env={**os.environ, "DOTSLASH_CACHE": str(cache)})
    executable = Path(output.strip()).resolve()
    if not executable.is_relative_to(cache.resolve()) or not executable.is_file():
        raise ValueError("Fetching did not produce a regular executable file inside the temporary cache.")
    return executable


def verify_download(interpreter, name, entry, provider, host):
    with tempfile.TemporaryDirectory(prefix="dotslash-download-") as temporary:
        root = Path(temporary)
        artifact = {**entry, "providers": [provider]}
        document = {"name": name, "platforms": {host: artifact}}
        manifest = root / name
        write_manifest(manifest, document)
        fetch(interpreter, manifest, root / "cache")


def execute_check(manifest, check, host, cache, root):
    executable = manifest.with_name(manifest.name + ".exe") if host.startswith("windows-") else manifest
    output = run(
        [str(executable), *check["command"][1:]],
        cwd=root,
        env={**os.environ, "DOTSLASH_CACHE": str(cache)},
        combined=True,
    )
    if not check["pattern"].search(output):
        raise ValueError(f"Native output did not match {check['pattern'].pattern!r}:\n{output}")


@contextmanager
def record(results, stage, **details):
    """Record an explicit outcome and allow independent checks to continue after failure."""
    result = {"stage": stage, "status": "passed", **details}
    try:
        yield result
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        result.update(status="failed", message=str(error))
    results.append(result)
    label = " / ".join(result[key] for key in ("manifest", "platform", "provider", "stage") if result.get(key))
    print(f"{result['status']}: {label}")
    if "message" in result:
        print(result["message"])


def prepare(root):
    interpreter = shutil.which("dotslash")
    if not interpreter:
        raise ValueError("DotSlash is unavailable. Run through mise exec or activate mise for your shell.")
    pin = tomllib.loads((root / "mise.toml").read_text(encoding="utf-8"))["tools"]["dotslash"]["version"]
    if run([interpreter, "--version"]).strip() != f"DotSlash {pin}":
        raise ValueError("The active DotSlash does not match mise.toml. Run through mise exec.")
    modes = {}
    for item in run(["git", "ls-files", "--stage", "-z", "--", "tools/bin"], cwd=root).split("\0"):
        if item:
            attributes, filename = item.split("\t", 1)
            modes[filename] = attributes.split()[0]
    host = host_platform()
    expected = os.environ.get("DOTSLASH_EXPECTED_PLATFORM", host)
    if host != expected:
        raise ValueError(f"The runner must execute natively on {expected}, but Python reports {host}.")
    return interpreter, modes, host


def check_manifests(root, paths, interpreter, modes, results):
    manifests = {}
    for path in paths:
        relative = path.relative_to(root).as_posix()
        with record(results, "policy", manifest=relative):
            document = json.loads(run([interpreter, "--", "parse", str(path)]))
            check = validate_document(document, path.name)
            if path.is_symlink() or modes.get(relative) != "100755":
                raise ValueError("Stage the manifest as a regular Git file with executable mode 100755.")
            if os.name != "nt" and not os.access(path, os.X_OK):
                raise ValueError("The manifest must also be executable in the working tree.")
            shim = path.with_name(path.name + ".exe")
            if shim.is_symlink() or hashlib.sha256(shim.read_bytes()).hexdigest() != WINDOWS_SHIM_SHA256:
                raise ValueError("The Windows companion must be the approved generic DotSlash x86-64 shim.")
            manifests[path] = (document, check)
    return manifests


def check_downloads(root, manifests, interpreter, host, results):
    for path, (document, _check) in manifests.items():
        for key, entry in document["platforms"].items():
            for provider in entry["providers"]:
                with record(
                    results,
                    "download",
                    manifest=path.relative_to(root).as_posix(),
                    platform=key,
                    provider=provider["url"],
                ):
                    verify_download(interpreter, path.name, entry, provider, host)


def check_native(root, manifests, host, results):
    for path, (_document, check) in manifests.items():
        relative = path.relative_to(root).as_posix()
        if check is None:
            with record(results, "native", manifest=relative, platform=host) as result:
                result.update(status="skipped", message="No native command is configured.")
            continue
        with tempfile.TemporaryDirectory(prefix="dotslash-native-") as temporary:
            cache = Path(temporary) / "cache"
            for stage in ("native-cold", "native-warm"):
                with record(results, stage, manifest=relative, platform=host) as result:
                    execute_check(path, check, host, cache, root)
                if result["status"] == "failed":
                    break


def validate(root, *, download=False, smoke=False, changed_since=None, report=None):
    """Return success after recording every independent validation result."""
    root = Path(root).resolve()
    results = []
    with record(results, "setup") as setup:
        interpreter, modes, host = prepare(root)
        paths = discover(root)
        selected = selected_manifests(root, paths, changed_since)
    if setup["status"] == "passed":
        manifests = check_manifests(root, paths, interpreter, modes, results)
        selected = {path: manifests[path] for path in selected if path in manifests}
        if download:
            check_downloads(root, selected, interpreter, host, results)
        if smoke and not any(result["status"] == "failed" for result in results):
            check_native(root, selected, host, results)
    success = not any(result["status"] == "failed" for result in results)
    if report:
        destination = Path(report)
        if not destination.is_absolute():
            destination = root / destination
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text(json.dumps({"success": success, "results": results}, indent=2) + "\n", encoding="utf-8")
    return success
