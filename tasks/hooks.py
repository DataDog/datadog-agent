"""Fast, read-only validators shared by pre-commit and CI.

Run this file directly to avoid importing the Invoke task registry. During a
commit, pre-commit supplies the staged working tree by hiding unstaged changes.
Outside pre-commit, use --all or explicit paths to validate the working tree.
Tool installation and downloads belong in the explicit setup commands only.
"""

import argparse
import os
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ACTIONLINT_VERSION = "1.7.12"
SET_X = re.compile(r"set( +-[^ ])* +-[^ ]*(x|( +xtrace))")
CHECKS = (
    "go-fmt",
    "shellcheck",
    "actionlint",
    "shell-check-no-set-x",
    "release-json-newline",
)


def git_paths(*args):
    return subprocess.check_output(["git", *args, "-z"], cwd=ROOT).decode("utf-8").split("\0")[:-1]


def settings_changed(check, paths):
    settings = {
        "go-fmt": {".go-version", ".golangci.yml"},
        "shellcheck": {".shellcheckrc"},
        "actionlint": {".github/actionlint.yaml", ".github/actionlint.yml"},
    }
    return any(
        path in settings.get(check, set())
        or (check == "actionlint" and path.startswith(".github/actions/"))
        or (check == "shellcheck" and path.endswith("/.shellcheckrc"))
        for path in paths
    )


def eligible(check, path):
    if check == "go-fmt":
        return path.endswith(".go")
    if check == "shellcheck":
        if path.endswith((".sh", ".bash", ".bats", ".ksh")):
            return True
        # Extensionless packaging scripts and shell templates need shebang
        # detection. Known non-shell extensions do not need to be opened.
        if Path(path).suffix not in {"", ".in"} or not (ROOT / path).is_file():
            return False
        with (ROOT / path).open("rb") as source:
            return bool(re.match(rb"#![^\r\n]*\b(?:ba|da|k)?sh\b", source.readline(256)))
    if check == "actionlint":
        return path.startswith(".github/workflows/") and path.endswith((".yml", ".yaml"))
    if check == "shell-check-no-set-x":
        return path.endswith((".sh", ".yaml", ".yml", "Dockerfile")) and ".pre-commit-config" not in path
    if check == "release-json-newline":
        return path == "release.json"
    return False


def selected_files(check, paths, all_files=False):
    if all_files or settings_changed(check, paths):
        # Git scans file metadata natively, avoiding thousands of Python stat
        # calls on Windows. Exclude symlinks, missing files, and type changes.
        unavailable = set(git_paths("diff-files", "--name-only", "--diff-filter=DT"))
        candidates = []
        for entry in git_paths("ls-files", "--stage"):
            metadata, path = entry.split("\t", 1)
            mode, _, stage = metadata.split()
            if stage != "0":
                raise ValueError(f"Resolve the index conflict in {path} before validating files.")
            if mode in {"100644", "100755"} and path not in unavailable:
                candidates.append(path)
        return sorted(path for path in set(candidates) if eligible(check, path))
    candidates = paths
    return sorted(
        path
        for path in set(candidates)
        if eligible(check, path) and (ROOT / path).is_file() and not (ROOT / path).is_symlink()
    )


def run_files(command, files):
    # Leave headroom below Windows' command-line limit, including quoting.
    result = 0
    batch = []
    size = 0
    for path in files:
        if batch and size + len(path) > 20000:
            result |= subprocess.run([*command, *batch], cwd=ROOT).returncode
            batch, size = [], 0
        batch.append("./" + path)
        size += len(path) + 4
    if batch:
        result |= subprocess.run([*command, *batch], cwd=ROOT).returncode
    return int(bool(result))


def parallel_files(command, files, output_is_failure=False, env=None, max_files=16, max_workers=8):
    from concurrent.futures import ThreadPoolExecutor

    def run(batch):
        return subprocess.run([*command, *batch], cwd=ROOT, capture_output=True, env=env)

    # Full-scope configuration changes otherwise serialize tens of seconds of
    # formatting and shell analysis. Keep output ordered and bound concurrency.
    batches = []
    batch = []
    size = 0
    for path in files:
        if batch and (size + len(path) > 20000 or len(batch) == max_files):
            batches.append(batch)
            batch, size = [], 0
        batch.append("./" + path)
        size += len(path) + 4
    if batch:
        batches.append(batch)
    result = 0
    with ThreadPoolExecutor(max_workers=min(max_workers, os.cpu_count() or 1)) as pool:
        for completed in pool.map(run, batches):
            if completed.stdout or completed.stderr:
                print((completed.stdout + completed.stderr).decode("utf-8"), end="")
            result |= bool(completed.returncode or (output_is_failure and completed.stdout))
    return int(result)


def gofmt_command():
    expected = "go" + (ROOT / ".go-version").read_text("utf-8").strip()
    version, goroot = subprocess.check_output(
        ["go", "env", "GOVERSION", "GOROOT"], env={**os.environ, "GOTOOLCHAIN": "local"}, text=True
    ).splitlines()
    if version != expected:
        raise ValueError(f"Install Go {expected.removeprefix('go')} from .go-version (found {version}).")
    return str(Path(goroot) / "bin" / ("gofmt.exe" if os.name == "nt" else "gofmt"))


def gofmt_files(files):
    import yaml

    exclusions = yaml.safe_load((ROOT / ".golangci.yml").read_text("utf-8"))["formatters"]["exclusions"]
    paths = exclusions["paths"]
    candidates = [
        path
        for path in files
        if not re.search(r"(?:\.pb|_gen)\.go$", path) and not any(re.search(pattern, path) for pattern in paths)
    ]
    pattern = r"^//.*(code generated|do not edit|autogenerated)"
    if len(candidates) > 64:
        # Native Git scans many small files substantially faster than opening
        # each file in Python on Windows. It reads the same staged working tree.
        result = subprocess.run(
            ["git", "grep", "-I", "-l", "-z", "-i", "-E", pattern, "--", "*.go"], cwd=ROOT, capture_output=True
        )
        if result.returncode not in {0, 1}:
            raise subprocess.CalledProcessError(result.returncode, result.args, result.stdout, result.stderr)
        generated = set(result.stdout.decode("utf-8").split("\0"))
        tracked = set(git_paths("ls-files"))
        generated.update(
            path
            for path in candidates
            if path not in tracked and re.search(pattern, (ROOT / path).read_text("utf-8"), re.I | re.M)
        )
        return [path for path in candidates if path not in generated]
    return [path for path in candidates if not re.search(pattern, (ROOT / path).read_text("utf-8"), re.I | re.M)]


def check_files(check, files):
    if not files:
        return 0
    if check == "go-fmt":
        files = gofmt_files(files)
        # gofmt -l reports formatting differences with a successful exit code.
        # Compare stdout as well as the exit status, without ever using -w.
        executable = gofmt_command()
        return parallel_files(
            [executable, "-l", "-s"],
            files,
            output_is_failure=True,
            env={**os.environ, "GOMAXPROCS": "1"},
            max_files=256,
            max_workers=16,
        )
    if check == "shellcheck":
        return parallel_files(["shellcheck", "--severity=info", "-e", "SC2059", "-e", "SC2028"], files)
    if check == "actionlint":
        return run_files(["actionlint", "-shellcheck=", "-pyflakes="], files)
    if check == "shell-check-no-set-x":
        errors = [
            f'{path}:{number}: {line.strip()}'
            for path in files
            for number, line in enumerate((ROOT / path).read_text("utf-8").splitlines(), 1)
            if SET_X.search(line)
        ]
        if errors:
            print('\n'.join(errors) + '\nNo shell script should use "set -x".')
        return int(bool(errors))
    if check == "release-json-newline":
        if not (ROOT / "release.json").read_bytes().endswith(b"\n"):
            print("release.json: add a final newline.")
            return 1
        return 0
    raise ValueError(f"Unknown check: {check}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("check", choices=(*CHECKS, "setup-actionlint"))
    scope = parser.add_mutually_exclusive_group()
    scope.add_argument("--all", action="store_true")
    scope.add_argument("--base", help="Check the committed diff from this revision (CI).")
    parser.add_argument("paths", nargs="*")
    args = parser.parse_args()
    os.chdir(ROOT)
    if args.check == "setup-actionlint":
        return subprocess.run(
            ["go", "install", f"github.com/rhysd/actionlint/cmd/actionlint@v{ACTIONLINT_VERSION}"]
        ).returncode
    paths = args.paths
    if args.base:
        paths = git_paths("diff", args.base, "HEAD", "--no-renames", "--name-only", "--diff-filter=ACDMRT")
    if not args.all and not args.base and not paths:
        parser.error("Choose --all, --base, or explicit paths.")
    return check_files(args.check, selected_files(args.check, paths, args.all))


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(error, file=sys.stderr)
        sys.exit(1)
