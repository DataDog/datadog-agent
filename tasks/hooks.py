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
from contextlib import contextmanager
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ACTIONLINT_VERSION = "1.7.12"
SET_X = re.compile(r"set( +-[^ ])* +-[^ ]*(x|( +xtrace))")
CHECKS = (
    "protected-branches",
    "go-fmt",
    "shellcheck",
    "actionlint",
    "copyright",
    "shell-check-no-set-x",
    "python-ruff",
    "buildifier",
    "rustfmt",
    "release-json-newline",
)


def git_paths(*args):
    return (
        subprocess.check_output(["git", args[0], "-z", *args[1:]], cwd=ROOT, stderr=subprocess.PIPE)
        .decode("utf-8")
        .split("\0")[:-1]
    )


def settings_changed(check, paths):
    settings = {
        "go-fmt": {".go-version", ".golangci.yml"},
        "shellcheck": {".shellcheckrc"},
        "actionlint": {".github/actionlint.yaml", ".github/actionlint.yml"},
        "copyright": {"tasks/libs/types/copyright.py"},
        "python-ruff": {"pyproject.toml", "ruff.toml", ".ruff.toml", ".python-version"},
        "buildifier": {".buildifier.json", "bazel/prebuilt_buildtools.json", "bazel/buildifier/BUILD.bazel"},
        "rustfmt": {"rust-toolchain.toml", "MODULE.bazel", ".bazelrc"},
    }
    return any(
        path in settings.get(check, set())
        or (check == "actionlint" and path.startswith(".github/actions/"))
        or (check == "shellcheck" and path.endswith("/.shellcheckrc"))
        or (check == "python-ruff" and Path(path).name in {"pyproject.toml", "ruff.toml", ".ruff.toml"})
        or (check == "rustfmt" and Path(path).name in {"Cargo.toml", "rustfmt.toml", ".rustfmt.toml"})
        for path in paths
    )


def eligible(check, path):
    if check in {"go-fmt", "copyright"}:
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
    if check == "python-ruff":
        return path.endswith((".py", ".pyi"))
    if check == "buildifier":
        from fnmatch import fnmatchcase

        name = Path(path).name
        # Match the pinned buildtools buildifier/runner.bash.template.
        return any(
            fnmatchcase(name, pattern)
            for pattern in (
                "*.bzl",
                "*.sky",
                "*.star",
                "*.bazel",
                "BUILD",
                "*.BUILD",
                "BUILD.oss",
                "BUILD.*.oss",
                "WORKSPACE",
                "WORKSPACE.oss",
                "WORKSPACE.bzlmod",
                "WORKSPACE.*.oss",
            )
        )
    if check == "rustfmt":
        return path.endswith(".rs")
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


def file_batches(files, max_files=0):
    batch, size = [], 0
    for path in files:
        cost = len(path.encode("utf-16-le")) // 2 + 4 if os.name == "nt" else len(path.encode("utf-8")) + 4
        if batch and (size + cost > 20000 or (max_files and len(batch) == max_files)):
            yield batch
            batch, size = [], 0
        batch.append(path)
        size += cost
    if batch:
        yield batch


def run_files(command, files):
    result = 0
    for batch in file_batches(files):
        result |= subprocess.run([*command, *("./" + path for path in batch)], cwd=ROOT).returncode
    return int(bool(result))


def parallel_files(command, files, output_is_failure=False, env=None, max_files=16, max_workers=8):
    from concurrent.futures import ThreadPoolExecutor

    def run(batch):
        return subprocess.run([*command, *("./" + path for path in batch)], cwd=ROOT, capture_output=True, env=env)

    result = 0
    with ThreadPoolExecutor(max_workers=min(max_workers, os.cpu_count() or 1)) as pool:
        for completed in pool.map(run, file_batches(files, max_files)):
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


def git_normalizes_crlf(path, contents):
    indexed = git_paths("ls-files", "--eol", "--", f":(literal){path}")
    if indexed and indexed[0].split()[0] in {"i/crlf", "i/mixed"}:
        return False
    cleaned = subprocess.check_output(["git", "hash-object", f"--path={path}", "--stdin"], cwd=ROOT, input=contents)
    normalized = subprocess.check_output(
        ["git", "hash-object", "--stdin"], cwd=ROOT, input=contents.replace(b"\r\n", b"\n")
    )
    return cleaned == normalized


def canonical_bytes(path):
    contents = (ROOT / path).read_bytes()
    if b"\r\n" in contents and git_normalizes_crlf(path, contents):
        return contents.replace(b"\r\n", b"\n")
    return contents


def split_line_endings(files):
    # Only normalize inputs whose Git representation actually uses LF.
    regular, crlf = [], []
    for path in files:
        try:
            contents = (ROOT / path).read_bytes()
        except OSError:
            # Native tools retain authority over exclusions and read errors.
            regular.append(path)
            continue
        normalizable = b"\r\n" in contents and git_normalizes_crlf(path, contents)
        (crlf if normalizable else regular).append(path)
    return regular, crlf


def gofmt_normalized_files(executable, files):
    import tempfile
    from concurrent.futures import ThreadPoolExecutor

    def run(path):
        # File mode requires a complete Go source file; stdin accepts fragments.
        with tempfile.TemporaryDirectory(prefix="agent-hooks-gofmt-") as directory:
            target = Path(directory) / Path(path).name
            target.write_bytes(canonical_bytes(path))
            result = subprocess.run(
                [executable, "-l", "-s", str(target)],
                cwd=ROOT,
                env={**os.environ, "GOMAXPROCS": "1"},
                capture_output=True,
            )
            if result.returncode or result.stdout:
                output = (result.stdout + result.stderr).decode("utf-8")
                return f"{path}: gofmt validation failed (exit {result.returncode})\n" + output.replace(
                    str(target), path
                ).replace(target.as_posix(), path)
        return ""

    with ThreadPoolExecutor(max_workers=min(8, os.cpu_count() or 1)) as pool:
        errors = list(filter(None, pool.map(run, files)))
    if errors:
        print("\n".join(errors))
    return int(bool(errors))


def shellcheck_link(source, target):
    import shutil
    import stat

    source = source.resolve(strict=True)
    if not stat.S_ISREG(source.stat().st_mode):
        raise ValueError(f"ShellCheck context must be a regular file: {source}")
    # Removing a Windows hard link to a read-only file can require changing
    # the original file's attributes. Copies avoid that cleanup side effect.
    if os.name == "nt" and not source.stat().st_mode & stat.S_IWRITE:
        shutil.copyfile(source, target)
    else:
        try:
            os.link(source, target)
        except OSError:
            shutil.copyfile(source, target)


@contextmanager
def shellcheck_view():
    import tempfile

    try:
        temporary = tempfile.TemporaryDirectory(prefix=".agent-hooks-shellcheck-", dir=ROOT.parent)
        view = Path(temporary.name)
    except PermissionError:
        cache = ROOT / ".cache/hooks"
        cache.mkdir(parents=True, exist_ok=True)
        temporary = tempfile.TemporaryDirectory(prefix=".agent-hooks-shellcheck-", dir=cache)
        view = Path(temporary.name) / "filesystem" / ROOT.relative_to(ROOT.anchor)
    with temporary:
        if view != Path(temporary.name):
            # Project ancestor siblings without reading their contents.
            # Relative source paths and ancestor configuration then keep
            # their native meaning, even when the parent is read-only.
            child = ROOT.name
            for parent in ROOT.parents:
                projected = Path(temporary.name) / "filesystem" / parent.relative_to(ROOT.anchor)
                projected.mkdir(parents=True, exist_ok=True)
                try:
                    entries = list(parent.iterdir())
                except PermissionError:
                    # Traversal can allow known configuration paths even
                    # when listing unrelated ancestor entries is forbidden.
                    entries = [parent / name for name in (".shellcheckrc", "shellcheckrc") if (parent / name).is_file()]
                for entry in entries:
                    if entry.name == child:
                        continue
                    target = projected / entry.name
                    if entry.is_dir():
                        if os.name == "nt":
                            from _winapi import CreateJunction

                            CreateJunction(str(entry.resolve()), str(target))
                        else:
                            target.symlink_to(entry, target_is_directory=True)
                    elif entry.is_file():
                        try:
                            shellcheck_link(entry, target)
                        except OSError:
                            # An inaccessible external source remains
                            # unavailable to ShellCheck in the view.
                            if entry.name in {".shellcheckrc", "shellcheckrc"}:
                                raise
                child = parent.name
            view.mkdir(parents=True, exist_ok=True)
        yield view


def shellcheck_files(files):
    command = ["shellcheck", "--severity=info", "-e", "SC2059", "-e", "SC2028"]
    _, crlf = split_line_endings(files)
    if not crlf:
        return run_files(command, files)
    # A separate file view preserves filenames, source inputs, and directory
    # configuration while letting ShellCheck parse Git-normalized bytes.
    endings = {}
    for entry in git_paths("ls-files", "--cached", "--others", "--exclude-standard", "--eol"):
        metadata, path = entry.split("\t", 1)
        endings[path] = metadata.split()[1]
    paths = set(endings) | set(files)
    for path in files:
        for parent in [(ROOT / path).parent, *(ROOT / path).parent.parents]:
            if not parent.is_relative_to(ROOT):
                break
            for name in (".shellcheckrc", "shellcheckrc"):
                config = parent / name
                if config.is_file():
                    paths.add(config.relative_to(ROOT).as_posix())
    # A sibling view preserves relative paths that leave the repository, and
    # shares the original ancestor and user configuration search locations.
    with shellcheck_view() as view:
        for path in sorted(paths):
            relative = Path(path)
            if relative.is_absolute() or ".." in relative.parts:
                raise ValueError("ShellCheck validation inputs must stay inside the repository.")
            source = ROOT / relative
            if not source.is_file():
                continue
            target = view / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            if target.exists():
                raise ValueError(f"The validation view cannot represent distinct paths for {path}.")
            if source.is_symlink():
                resolved = source.resolve()
                contents = (
                    canonical_bytes(resolved.relative_to(ROOT).as_posix())
                    if resolved.is_relative_to(ROOT)
                    else resolved.read_bytes()
                )
                target.write_bytes(contents)
            elif endings.get(path) in {"w/crlf", "w/mixed", "w/-text", None}:
                contents = canonical_bytes(path)
                target.write_bytes(contents)
            else:
                # ShellCheck never writes its inputs. A hard link preserves
                # native access errors without opening unrelated files here.
                try:
                    shellcheck_link(source, target)
                except PermissionError:
                    if path in files or relative.name in {".shellcheckrc", "shellcheckrc"}:
                        raise
                    # ShellCheck reports an unavailable source if it needs it.
                    # An unrelated unreadable file must not block validation.
                    continue
        status = 0
        for batch in file_batches(files):
            result = subprocess.run([*command, *("./" + path for path in batch)], cwd=view)
            status |= bool(result.returncode)
        return int(bool(status))


def buildifier_info():
    import json

    return json.loads((ROOT / "bazel/prebuilt_buildtools.json").read_text("utf-8"))["buildifier"]


def buildifier_path():
    suffix = ".exe" if os.name == "nt" else ""
    return ROOT / ".cache" / "hooks" / buildifier_info()["version"] / ("buildifier" + suffix)


def buildifier_files(files):
    import ast

    # Derive exclusions from the same literal list used by both Bazel targets.
    source = (ROOT / "bazel/buildifier/BUILD.bazel").read_text("utf-8")
    patterns = ast.literal_eval(re.search(r"exclude_patterns = (\[.*?\])", source, re.S)[1])
    from fnmatch import fnmatchcase

    return [path for path in files if not any(fnmatchcase("./" + path, pattern) for pattern in patterns)]


def rust_settings():
    source = (ROOT / "MODULE.bazel").read_text("utf-8")
    settings = re.search(r"^toolchains\.toolchain\((.*?)\n\)", source, re.S | re.M)[1]
    return re.search(r'version = "([^"]+)"', settings)[1], re.search(r'edition = "([^"]+)"', settings)[1]


def rustfmt_files(files):
    import tempfile
    from concurrent.futures import ThreadPoolExecutor

    version, edition = rust_settings()
    executable = subprocess.check_output(
        ["rustup", "which", "--toolchain", version, "rustfmt"], encoding="utf-8"
    ).strip()

    def check(item):
        number, path = item
        target = Path(directory) / f"{number}.rs"
        target.write_bytes(canonical_bytes(path))
        target = target.resolve()
        result = subprocess.run(
            [
                executable,
                "--check",
                "--edition",
                edition,
                "--config-path",
                str(config),
                "--config",
                "skip_children=true",
                str(target),
            ],
            capture_output=True,
            cwd=ROOT,
        )
        if result.returncode:
            output = (result.stdout + result.stderr).decode("utf-8")
            if os.name == "nt":
                native = str(target)
                extended = "\\\\?\\UNC\\" + native[2:] if native.startswith("\\\\") else "\\\\?\\" + native
                output = output.replace(extended, path)
            return f"{path}: rustfmt failed\n" + output.replace(str(target), path).replace(target.as_posix(), path)
        return ""

    # The Bazel rustfmt aspect explicitly selects rules_rust's empty default
    # configuration. It does not discover repository or user rustfmt files.
    with tempfile.TemporaryDirectory(prefix="agent-hooks-rustfmt-") as directory:
        config = Path(directory) / "rustfmt.toml"
        config.write_text("", encoding="utf-8")
        # Match CI's file-mode check and skip_children setting. Unlike stdin
        # comparisons, this also honors skipped BOM and preserved-CRLF files.
        with ThreadPoolExecutor(max_workers=min(8, os.cpu_count() or 1)) as pool:
            errors = list(filter(None, pool.map(check, enumerate(files))))
    if errors:
        message = "\n".join(errors)
        encoding = getattr(sys.stdout, "encoding", None) or "utf-8"
        print(message.encode(encoding, errors="backslashreplace").decode(encoding))
    return int(bool(errors))


def setup_buildifier():
    import hashlib
    import platform
    import urllib.request

    system = {"Darwin": "macos", "Linux": "linux", "Windows": "windows"}[platform.system()]
    cpu = {"AMD64": "x86_64", "x86_64": "x86_64", "aarch64": "arm64", "arm64": "arm64"}[platform.machine()]
    binary = next(item for item in buildifier_info()["binaries"] if item["os"] == system and item["cpu"] == cpu)
    destination = buildifier_path()
    with urllib.request.urlopen(binary["url"], timeout=60) as response:
        data = response.read()
    if hashlib.sha256(data).hexdigest() != binary["sha256"]:
        raise ValueError("Buildifier download does not match bazel/prebuilt_buildtools.json.")
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_bytes(data)
    destination.chmod(0o755)
    print(f"Installed {destination}")


def stdin_format_files(command, files, output_is_failure=True):
    from concurrent.futures import ThreadPoolExecutor

    def run(path):
        arguments = command(path)
        result = subprocess.run(
            arguments,
            cwd=ROOT,
            input=canonical_bytes(path),
            capture_output=True,
        )
        if result.returncode or (output_is_failure and result.stdout):
            name = f"{Path(arguments[0]).name} {arguments[1]}"
            return f"{path}: {name} validation failed\n" + (result.stdout + result.stderr).decode("utf-8")
        return ""

    with ThreadPoolExecutor(max_workers=min(8, os.cpu_count() or 1)) as pool:
        errors = list(filter(None, pool.map(run, files)))
    if errors:
        message = "\n".join(errors)
        encoding = getattr(sys.stdout, "encoding", None) or "utf-8"
        print(message.encode(encoding, errors="backslashreplace").decode(encoding))
    return int(bool(errors))


def check_files(check, files):
    if not files:
        return 0
    if check == "go-fmt":
        files = gofmt_files(files)
        # gofmt -l reports formatting differences with a successful exit code.
        # Compare stdout as well as the exit status, without ever using -w.
        executable = gofmt_command()
        regular, crlf = split_line_endings(files)
        status = parallel_files(
            [executable, "-l", "-s"],
            regular,
            output_is_failure=True,
            env={**os.environ, "GOMAXPROCS": "1"},
            max_files=256,
            max_workers=16,
        )
        return status | gofmt_normalized_files(executable, crlf)
    if check == "shellcheck":
        return shellcheck_files(files)
    if check == "actionlint":
        return run_files(["actionlint", "-shellcheck=", "-pyflakes="], files)
    if check == "copyright":
        from runpy import run_path

        # A staged replacement can share an unstaged module's bytecode stamp.
        # Read the rules from source rather than a timestamp-based .pyc file.
        rules = run_path(str(ROOT / "tasks/libs/types/copyright.py"))
        try:
            rules["CopyrightLinter"]().assert_compliance(files=[ROOT / path for path in files])
        except rules["LintFailure"]:
            return 1
        return 0
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
    if check == "python-ruff":
        regular, crlf = split_line_endings(files)
        status = 0
        for command in (
            ["ruff", "format", "--check", "--force-exclude", "--no-cache"],
            ["ruff", "check", "--no-fix", "--force-exclude", "--no-cache"],
        ):
            status |= run_files(command, regular) | stdin_format_files(
                lambda path, command=command: [*command, "--stdin-filename", str(ROOT / path), "-"],
                crlf,
                output_is_failure=False,
            )
        return status
    if check == "buildifier":
        files = buildifier_files(files)
        if not files:
            return 0
        executable = buildifier_path()
        if not executable.is_file():
            raise ValueError("Run `python tasks/hooks.py setup-buildifier` to install CI's Buildifier version.")
        regular, crlf = split_line_endings(files)
        command = [str(executable), "-mode=check", "-lint=warn"]
        return parallel_files(command, regular, max_files=256) | stdin_format_files(
            lambda path: [*command, f"-path={path}"], crlf, output_is_failure=False
        )
    if check == "rustfmt":
        return rustfmt_files(files)
    if check == "release-json-newline":
        if not (ROOT / "release.json").read_bytes().endswith(b"\n"):
            print("release.json: add a final newline.")
            return 1
        return 0
    raise ValueError(f"Unknown check: {check}")


def protected_branch():
    result = subprocess.run(
        ["git", "symbolic-ref", "--quiet", "--short", "HEAD"], cwd=ROOT, capture_output=True, text=True
    )
    # A detached HEAD has no local branch to protect.
    if result.returncode == 1:
        return 0
    result.check_returncode()
    branch = result.stdout.strip()
    if branch == "main" or re.fullmatch(r"[0-9]+\.[0-9]+\.x", branch):
        print(f"Refusing a commit on protected branch {branch}. Create a development branch first.")
        return 1
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("check", choices=(*CHECKS, "setup-buildifier", "setup-actionlint"))
    scope = parser.add_mutually_exclusive_group()
    scope.add_argument("--staged", action="store_true")
    scope.add_argument(
        "--hook", action="store_true", help="Use pre-commit's requested paths, including deleted settings."
    )
    scope.add_argument("--all", action="store_true")
    scope.add_argument("--base", help="Check the committed diff from this revision (CI).")
    parser.add_argument("paths", nargs="*")
    args = parser.parse_intermixed_args()
    os.chdir(ROOT)
    if args.check == "setup-buildifier":
        setup_buildifier()
        return 0
    if args.check == "setup-actionlint":
        return subprocess.run(
            ["go", "install", f"github.com/rhysd/actionlint/cmd/actionlint@v{ACTIONLINT_VERSION}"]
        ).returncode
    if args.check == "protected-branches":
        return protected_branch()
    paths = [Path(os.path.relpath(ROOT / path, ROOT)).as_posix() for path in args.paths]
    if args.staged:
        paths = git_paths("diff", "--cached", "--no-renames", "--name-only", "--diff-filter=ACMRT")
        deleted = git_paths("diff", "--cached", "--no-renames", "--name-only", "--diff-filter=D")
        paths.extend(path for path in deleted if settings_changed(args.check, [path]))
    elif args.hook:
        from_ref = os.environ.get("PRE_COMMIT_FROM_REF")
        to_ref = os.environ.get("PRE_COMMIT_TO_REF")
        revisions = [f"{from_ref}...{to_ref}"] if from_ref and to_ref else ["--cached"]
        try:
            deleted = git_paths("diff", *revisions, "--no-renames", "--name-only", "--diff-filter=D")
        except subprocess.CalledProcessError:
            if not (from_ref and to_ref):
                raise
            deleted = git_paths("diff", f"{from_ref}..{to_ref}", "--no-renames", "--name-only", "--diff-filter=D")
        paths = [path for path in paths if path not in deleted]
        paths.extend(path for path in deleted if settings_changed(args.check, [path]))
    elif args.base:
        paths = git_paths("diff", args.base, "HEAD", "--no-renames", "--name-only", "--diff-filter=ACDMRT")
    if not args.staged and not args.hook and not args.all and not args.base and not paths:
        parser.error("Choose --staged, --hook, --all, --base, or explicit paths.")
    return check_files(args.check, selected_files(args.check, paths, args.all))


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        if isinstance(error, subprocess.CalledProcessError) and error.stderr:
            print(error.stderr.decode("utf-8"), file=sys.stderr, end="")
        print(error, file=sys.stderr)
        sys.exit(1)
