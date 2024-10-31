"""
Miscellaneous functions, no tasks here
"""

from __future__ import annotations

import os
import platform
import re
import sys
import tempfile
import time
import traceback
from collections import Counter
from contextlib import contextmanager
from dataclasses import dataclass
from functools import wraps
from pathlib import Path
from subprocess import CalledProcessError, check_output
from types import SimpleNamespace

import requests
from invoke.context import Context
from invoke.exceptions import Exit

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.constants import ALLOWED_REPO_ALL_BRANCHES, DEFAULT_BRANCH, REPO_PATH
from tasks.libs.common.git import get_commit_sha
from tasks.libs.owners.parsing import search_owners
from tasks.libs.releasing.version import get_version
from tasks.libs.types.arch import Arch

if sys.platform == "darwin":
    RTLOADER_LIB_NAME = "libdatadog-agent-rtloader.dylib"
elif sys.platform == "win32":
    RTLOADER_LIB_NAME = "libdatadog-agent-rtloader.a"
else:
    RTLOADER_LIB_NAME = "libdatadog-agent-rtloader.so"
RTLOADER_HEADER_NAME = "datadog_agent_rtloader.h"


@dataclass
class TimedOperationResult:
    name: str
    # In seconds
    duration: float

    @classmethod
    def run(cls, f, name, description, **f_kwargs):
        time_start = time.perf_counter()

        with gitlab_section(description, collapsed=True):
            result = f(**f_kwargs)

        time_end = time.perf_counter()
        duration = time_end - time_start

        return result, cls(name, duration)

    def __lt__(self, other):
        if isinstance(other, TimedOperationResult):
            return self.name < other.name
        else:
            return True


def get_all_allowed_repo_branches():
    return ALLOWED_REPO_ALL_BRANCHES


def is_allowed_repo_branch(branch):
    return branch in ALLOWED_REPO_ALL_BRANCHES


def running_in_github_actions():
    return os.environ.get("GITHUB_ACTIONS") == "true"


def running_in_gitlab_ci():
    return os.environ.get("GITLAB_CI") == "true"


def running_in_circleci():
    return os.environ.get("CIRCLECI") == "true"


def running_in_ci():
    return running_in_circleci() or running_in_github_actions() or running_in_gitlab_ci()


def running_in_pyapp():
    return os.environ.get("PYAPP") == "1"


def running_in_pre_commit():
    return os.environ.get("PRE_COMMIT") == "1"


def bin_name(name):
    """
    Generate platform dependent names for binaries
    """
    if sys.platform == 'win32':
        return f"{name}.exe"
    return name


def get_distro():
    """
    Get the distro name. Windows and Darwin stays the same.
    Linux is the only one that needs to be determined using the /etc/os-release file.
    """
    system = platform.system()
    arch = platform.machine()
    if system == 'Linux' and os.path.isfile('/etc/os-release'):
        with open('/etc/os-release', encoding="utf-8") as f:
            for line in f:
                if line.startswith('ID='):
                    system = line.strip().removeprefix('ID=').replace('"', '')
                    break
    return f"{system}_{arch}".lower()


def get_goenv(ctx, var):
    return ctx.run(f"go env {var}", hide=True).stdout.strip()


def get_gopath(ctx):
    return get_goenv(ctx, "GOPATH")


def get_gobin(ctx):
    gobin = get_goenv(ctx, "GOBIN")
    if not gobin:
        gopath = get_gopath(ctx)
        gobin = os.path.join(gopath, "bin")

    return gobin


def get_rtloader_paths(embedded_path=None, rtloader_root=None):
    rtloader_lib = []
    rtloader_headers = ""
    rtloader_common_headers = ""

    for base_path in [rtloader_root, embedded_path]:
        if not base_path:
            continue

        for libdir in ["lib", "lib64", "build/rtloader"]:
            if os.path.exists(os.path.join(base_path, libdir, RTLOADER_LIB_NAME)):
                rtloader_lib.append(os.path.join(base_path, libdir))

        header_path = os.path.join(base_path, "include")
        if not rtloader_headers and os.path.exists(os.path.join(header_path, RTLOADER_HEADER_NAME)):
            rtloader_headers = header_path

        common_path = os.path.join(base_path, "common")
        if not rtloader_common_headers and os.path.exists(common_path):
            rtloader_common_headers = common_path

    return rtloader_lib, rtloader_headers, rtloader_common_headers


def get_embedded_path(ctx):
    base = os.path.dirname(os.path.abspath(__file__))
    task_repo_root = os.path.abspath(os.path.join(base, "..", ".."))
    git_repo_root = get_root()
    gopath_root = f"{get_gopath(ctx)}/src/github.com/DataDog/datadog-agent"

    for root_candidate in [task_repo_root, git_repo_root, gopath_root]:
        test_embedded_path = os.path.join(root_candidate, "dev")
        if os.path.exists(test_embedded_path):
            return test_embedded_path

    return None


def get_xcode_version(ctx):
    """
    Get the version of XCode used depending on how it's installed.
    """
    if sys.platform != "darwin":
        raise ValueError("The get_xcode_version function is only available on macOS")
    xcode_path = ctx.run("xcode-select -p", hide=True).stdout.strip()
    if xcode_path == "/Library/Developer/CommandLineTools":
        xcode_version = ctx.run("pkgutil --pkg-info=com.apple.pkg.CLTools_Executables", hide=True).stdout.strip()
        xcode_version = re.search(r"version: ([0-9.]+)", xcode_version).group(1)
        xcode_version = re.search(r"([0-9]+.[0-9]+)", xcode_version).group(1)
    elif xcode_path.startswith("/Applications/Xcode"):
        xcode_version = ctx.run(
            "xcodebuild -version | grep -Eo 'Xcode [0-9.]+' | awk '{print $2}'", hide=True
        ).stdout.strip()
    else:
        raise ValueError(f"Unknown XCode installation at {xcode_path}.")
    return xcode_version


def get_build_flags(
    ctx: Context,
    static=False,
    install_path=None,
    run_path=None,
    embedded_path=None,
    rtloader_root=None,
    python_home_2=None,
    python_home_3=None,
    major_version='7',
    headless_mode=False,
    arch: Arch | None = None,
):
    """
    Build the common value for both ldflags and gcflags, and return an env accordingly.

    We need to invoke external processes here so this function need the
    Context object.
    """
    gcflags = ""
    ldflags = get_version_ldflags(ctx, major_version=major_version, install_path=install_path)
    # External linker flags; needs to be handled separately to avoid overrides
    extldflags = ""
    env = {"GO111MODULE": "on"}

    if sys.platform == 'win32':
        env["CGO_LDFLAGS_ALLOW"] = "-Wl,--allow-multiple-definition"
    else:
        # for pkg/ebpf/compiler on linux
        env['CGO_LDFLAGS_ALLOW'] = "-Wl,--wrap=.*"

    if embedded_path is None:
        embedded_path = get_embedded_path(ctx)
        if embedded_path is None:
            raise Exit("unable to locate embedded path please check your setup or set --embedded-path")

    rtloader_lib, rtloader_headers, rtloader_common_headers = get_rtloader_paths(embedded_path, rtloader_root)

    # setting the install path, allowing the agent to be installed in a custom location
    if sys.platform.startswith('linux') and install_path:
        ldflags += f"-X {REPO_PATH}/pkg/config/setup.InstallPath={install_path} "

    # setting the run path
    if sys.platform.startswith('linux') and run_path:
        ldflags += f"-X {REPO_PATH}/pkg/config/setup.defaultRunPath={run_path} "

    # setting python homes in the code
    if python_home_2:
        ldflags += f"-X {REPO_PATH}/pkg/collector/python.pythonHome2={python_home_2} "
    if python_home_3:
        ldflags += f"-X {REPO_PATH}/pkg/collector/python.pythonHome3={python_home_3} "

    ldflags += f"-X {REPO_PATH}/pkg/config/setup.ForceDefaultPython=true "
    ldflags += f"-X {REPO_PATH}/pkg/config/setup.DefaultPython=3 "

    # adding rtloader libs and headers to the env
    if rtloader_lib:
        if not headless_mode:
            print(
                f"--- Setting rtloader paths to lib:{','.join(rtloader_lib)} | header:{rtloader_headers} | common headers:{rtloader_common_headers}"
            )
        env['DYLD_LIBRARY_PATH'] = os.environ.get('DYLD_LIBRARY_PATH', '') + f":{':'.join(rtloader_lib)}"  # OSX
        env['LD_LIBRARY_PATH'] = os.environ.get('LD_LIBRARY_PATH', '') + f":{':'.join(rtloader_lib)}"  # linux
        env['CGO_LDFLAGS'] = os.environ.get('CGO_LDFLAGS', '') + f" -L{' -L '.join(rtloader_lib)}"

    if sys.platform == 'win32':
        env['CGO_LDFLAGS'] = os.environ.get('CGO_LDFLAGS', '') + ' -Wl,--allow-multiple-definition'

    extra_cgo_flags = " -Werror -Wno-deprecated-declarations"
    if rtloader_headers:
        extra_cgo_flags += f" -I{rtloader_headers}"
    if rtloader_common_headers:
        extra_cgo_flags += f" -I{rtloader_common_headers}"
    env['CGO_CFLAGS'] = os.environ.get('CGO_CFLAGS', '') + extra_cgo_flags

    # if `static` was passed ignore setting rpath, even if `embedded_path` was passed as well
    if static:
        ldflags += "-s -w -linkmode=external "
        extldflags += "-static "
    elif rtloader_lib:
        ldflags += f"-r {':'.join(rtloader_lib)} "

    if os.environ.get("DELVE"):
        gcflags = "all=-N -l"
        # if sys.platform == 'win32':
        # On windows, need to build with the extra argument -ldflags="-linkmode internal"
        # if you want to be able to use the delve debugger.
        #
        # Currently the presense of "-linkmode internal " actually causes link error which
        # is contrary to the assertions stated above and the line is temporary commented out.
        # ldflags += "-linkmode internal "
    elif os.environ.get("NO_GO_OPT"):
        gcflags = "-N -l"

    if sys.platform == "darwin":
        # On macOS work around https://github.com/golang/go/issues/38824
        # as done in https://go-review.googlesource.com/c/go/+/372798
        extldflags += "-Wl,-bind_at_load"

        # On macOS when using XCode 15 the -no_warn_duplicate_libraries linker flag is needed to avoid getting ld warnings
        # for duplicate libraries: `ld: warning: ignoring duplicate libraries: '-ldatadog-agent-rtloader', '-ldl'`.
        # Gotestsum sees the ld warnings as errors, breaking the test invoke task, so we have to remove them.
        # See https://indiestack.com/2023/10/xcode-15-duplicate-library-linker-warnings/
        try:
            xcode_version = get_xcode_version(ctx)
            if int(xcode_version.split('.')[0]) >= 15:
                extldflags += ",-no_warn_duplicate_libraries "
        except ValueError:
            print(
                color_message(
                    "Warning: Could not determine XCode version, not adding -no_warn_duplicate_libraries to extldflags",
                    Color.ORANGE,
                ),
                file=sys.stderr,
            )

    if arch and arch.is_cross_compiling():
        # For cross-compilation we need to be explicit about certain Go settings
        env["GOARCH"] = arch.go_arch
        env["CGO_ENABLED"] = "1"  # If we're cross-compiling, CGO is disabled by default. Ensure it's always enabled
        env["CC"] = arch.gcc_compiler()
    if os.getenv('DD_CC'):
        env['CC'] = os.getenv('DD_CC')
    if os.getenv('DD_CXX'):
        env['CXX'] = os.getenv('DD_CXX')

    if sys.platform == 'linux':
        # Enable lazy binding, which seems to be overridden when loading containerd
        # Required to fix go-nvml compilation (see https://github.com/NVIDIA/go-nvml/issues/18)
        extldflags += "-Wl,-z,lazy "

    if extldflags:
        ldflags += f"'-extldflags={extldflags}' "

    return ldflags, gcflags, env


def get_common_test_args(build_tags, failfast):
    return {
        "build_tags": ",".join(build_tags),
        "failfast": "-failfast" if failfast else "",
    }


def get_payload_version():
    """
    Return the Agent payload version (`x.y.z`) found in the go.mod file.
    """
    with open('go.mod') as f:
        for rawline in f:
            line = rawline.strip()
            whitespace_split = line.split(" ")
            if len(whitespace_split) < 2:
                continue
            pkgname = whitespace_split[0]
            if pkgname == "github.com/DataDog/agent-payload/v5":
                # Example of line
                # github.com/DataDog/agent-payload/v5 v5.0.2
                # github.com/DataDog/agent-payload/v5 v5.0.1-0.20200826134834-1ddcfb686e3f
                version_split = re.split(r'[ +]', line)
                if len(version_split) < 2:
                    raise Exception(
                        "Versioning of agent-payload in go.mod has changed, the version logic needs to be updated"
                    )
                version = version_split[1].split("-")[0].strip()
                if not re.search(r"^v\d+(\.\d+){2}$", version):
                    raise Exception(f"Version of agent-payload in go.mod is invalid: '{version}'")
                return version

    raise Exception("Could not find valid version for agent-payload in go.mod file")


def get_version_ldflags(ctx, major_version='7', install_path=None):
    """
    Compute the version from the git tags, and set the appropriate compiler
    flags
    """
    payload_v = get_payload_version()
    commit = get_commit_sha(ctx, short=True)

    ldflags = f"-X {REPO_PATH}/pkg/version.Commit={commit} "
    ldflags += (
        f"-X {REPO_PATH}/pkg/version.AgentVersion={get_version(ctx, include_git=True, major_version=major_version)} "
    )
    ldflags += f"-X {REPO_PATH}/pkg/serializer.AgentPayloadVersion={payload_v} "
    if install_path:
        package_version = os.path.basename(install_path)
        if package_version != "datadog-agent":
            ldflags += f"-X {REPO_PATH}/pkg/version.AgentPackageVersion={package_version} "
        if sys.platform == 'win32':
            # On Windows we don't have a version in the install_path
            # so, set the package_version tag in order for Fleet Automation to detect
            # upgrade in the health check.
            # https://github.com/DataDog/dd-go/blob/cada5b3c2929473a2bd4a4142011767fe2dcce52/remote-config/apps/rc-api-internal/updater/health_check.go#L219
            ldflags += f"-X {REPO_PATH}/pkg/version.AgentPackageVersion={get_version(ctx, include_git=True, major_version=major_version)}-1 "
    return ldflags


def get_go_version():
    """
    Get the version of Go used
    """
    return check_output(['go', 'version']).decode('utf-8').strip()


def get_root():
    """
    Get the root of the Go project
    """
    return check_output(['git', 'rev-parse', '--show-toplevel']).decode('utf-8').strip()


def get_git_branch_name():
    """
    Return the name of the current git branch
    """
    return check_output(["git", "rev-parse", "--abbrev-ref", "HEAD"]).decode('utf-8').strip()


def get_git_pretty_ref():
    """
    Return the name of the current Git branch or the tag if in a detached state
    """
    # https://docs.gitlab.com/ee/ci/variables/predefined_variables.html
    if running_in_gitlab_ci():
        return os.environ["CI_COMMIT_REF_NAME"]

    # https://docs.github.com/en/actions/learn-github-actions/variables#default-environment-variables
    if running_in_github_actions():
        return os.environ.get("GITHUB_HEAD_REF") or os.environ["GITHUB_REF"].split("/")[-1]

    # https://circleci.com/docs/variables/#built-in-environment-variables
    if running_in_circleci():
        return os.environ.get("CIRCLE_TAG") or os.environ["CIRCLE_BRANCH"]

    current_branch = get_git_branch_name()
    if current_branch != "HEAD":
        return current_branch

    try:
        return check_output(["git", "describe", "--tags", "--exact-match"]).decode('utf-8').strip()
    except CalledProcessError:
        return current_branch


@contextmanager
def timed(name="", quiet=False):
    """Context manager that prints how long it took"""
    start = time.time()
    res = SimpleNamespace()
    print(f"{name}")
    try:
        yield res
    finally:
        res.duration = time.time() - start
        if not quiet:
            print(f"{name} completed in {res.duration:.2f}s")


def clean_nested_paths(paths):
    """
    Clean a list of paths by removing paths that are included in other paths.

    Example:
    >>> clean_nested_paths(["./pkg/utils/toto", "./pkg/utils/", "./pkg", "./toto/pkg", "./pkg/utils/tata"])
    ["./pkg", "./toto/pkg"]
    """
    # sort the paths by length, so that the longest paths are at the beginning
    paths.sort()
    cleaned_paths = []
    for path in paths:
        # if the path is already included in another path, skip it
        if len(cleaned_paths) == 0:
            cleaned_paths.append(path)
        else:
            last_clean_path_splitted = cleaned_paths[-1].split("/")
            path_splitted = path.split("/")
            for idx, element in enumerate(last_clean_path_splitted):
                if idx >= len(path_splitted) or element != path_splitted[idx]:
                    cleaned_paths.append(path)
                    break

    return cleaned_paths


@contextmanager
def environ(env):
    original_environ = os.environ.copy()
    os.environ.update(env)
    yield
    for var in env:
        if var in original_environ:
            os.environ[var] = original_environ[var]
        else:
            os.environ.pop(var)


def is_pr_context(branch, pr_id, test_name):
    if branch == DEFAULT_BRANCH:
        print(f"Running on {DEFAULT_BRANCH}, skipping check for {test_name}.")
        return False
    if not pr_id:
        print(f"PR not found, skipping check for {test_name}.")
        return False
    return True


@contextmanager
def gitlab_section(section_name, collapsed=False, echo=False):
    """
    - echo: If True, will echo the gitlab section in bold in CLI mode instead of not showing anything
    """
    # Replace with "_" every special character (" ", ":", "/", "\") which prevent the section generation
    section_id = re.sub(r"[ :/\\]", "_", section_name)
    in_ci = running_in_gitlab_ci()
    try:
        if in_ci:
            collapsed = '[collapsed=true]' if collapsed else ''
            print(f"\033[0Ksection_start:{int(time.time())}:{section_id}{collapsed}\r\033[0K{section_name + '...'}")
        elif echo:
            print(color_message(f"> {section_name}...", 'bold'))
        yield
    finally:
        if in_ci:
            print(f"\033[0Ksection_end:{int(time.time())}:{section_id}\r\033[0K")


def retry_function(action_name_fmt, max_retries=2, retry_delay=1):
    """
    Decorator to retry a function in case of failure and print its traceback.
    - action_name_fmt: String that will be formatted with the function arguments to describe the action (for example: "Running {0}" will display "Running arg1" if the function is called with arg1 and "Refresh {0.id}" will display "Refresh 123" if the function is called with an object with an id of 123)
    """

    def decorator(f):
        @wraps(f)
        def wrapper(*args, **kwargs):
            action_name = action_name_fmt.format(*args)

            for i in range(max_retries + 1):
                try:
                    res = f(*args, **kwargs)
                    if i != 0:
                        print(color_message(f'Note: {action_name} successful after {i} retries', 'green'))

                    # Action ok
                    return res
                except KeyboardInterrupt:
                    # Let the user interrupt without retries
                    raise
                except Exception:
                    if i == max_retries:
                        print(
                            color_message(f'Error: {action_name} failed after {max_retries} retries', Color.RED),
                            file=sys.stderr,
                        )
                        # The stack trace is not printed here but the error is raised if we
                        # want to catch it above
                        raise
                    else:
                        print(
                            color_message(
                                f'Warning: {action_name} failed (retry {i + 1}/{max_retries}), retrying in {retry_delay}s',
                                Color.ORANGE,
                            ),
                            file=sys.stderr,
                        )
                        with gitlab_section(f"Retry {i + 1}/{max_retries} {action_name}", collapsed=True):
                            traceback.print_exc()
                        time.sleep(retry_delay)
                        print(color_message(f'Retrying {action_name}', 'blue'))

        return wrapper

    return decorator


def parse_kernel_version(version: str) -> tuple[int, int, int, int]:
    """
    Parse a kernel version contained in the given string and return a
    tuple with kernel version, major and minor revision and patch number
    """
    kernel_version_regex = re.compile(r'(\d+)\.(\d+)(\.(\d+))?(-(\d+))?')
    match = kernel_version_regex.search(version)
    if match is None:
        raise ValueError(f"Cannot parse kernel version from {version}")

    return (int(match.group(1)), int(match.group(2)), int(match.group(4) or "0"), int(match.group(6) or "0"))


def guess_from_labels(issue):
    for label in issue.labels:
        if label.name.startswith("team/") and "triage" not in label.name:
            return label.name.split("/")[-1]
    return 'triage'


def guess_from_keywords(issue):
    text = f"{issue.title} {issue.body}".casefold().split()
    c = Counter(text)
    for word in c.most_common():
        team = simple_match(word[0])
        if team:
            return team
        team = file_match(word[0])
        if team:
            return team
    return "triage"


def simple_match(word):
    pattern_matching = {
        "agent-apm": ['apm', 'java', 'dotnet', 'ruby', 'trace'],
        "containers": [
            'container',
            'pod',
            'kubernetes',
            'orchestrator',
            'docker',
            'k8s',
            'kube',
            'cluster',
            'kubelet',
            'helm',
        ],
        "agent-metrics-logs": ['logs', 'metric', 'log-ag', 'statsd', 'tags', 'hostnam'],
        "agent-delivery": ['omnibus', 'packaging', 'script'],
        "remote-config": ['installer', 'oci'],
        "agent-cspm": ['cspm'],
        "ebpf-platform": ['ebpf', 'system-prob', 'sys-prob'],
        "agent-security": ['security', 'vuln', 'security-agent'],
        "agent-shared-components": ['fips', 'inventory', 'payload', 'jmx', 'intak', 'gohai'],
        "fleet": ['fleet', 'fleet-automation'],
        "opentelemetry": ['otel', 'opentelemetry'],
        "windows-agent": ['windows', 'sys32', 'powershell'],
        "networks": ['tcp', 'udp', 'socket', 'network'],
        "serverless": ['serverless'],
        "integrations": ['integration', 'python', 'checks'],
    }
    for team, words in pattern_matching.items():
        if any(w in word for w in words):
            return team
    return None


def file_match(word):
    dd_folders = [
        'chocolatey',
        'cmd',
        'comp',
        'dev',
        'devenv',
        'docs',
        'internal',
        'omnibus',
        'pkg',
        'rtloader',
        'tasks',
        'test',
        'tools',
    ]
    p = Path(word)
    if len(p.parts) > 1 and p.suffix:
        path_folder = next((f for f in dd_folders if f in p.parts), None)
        if path_folder:
            file = '/'.join(p.parts[p.parts.index(path_folder) :])
            return (
                search_owners(file, ".github/CODEOWNERS")[0].casefold().replace("@datadog/", "")
            )  # only return the first owner
    return None


def team_to_label(team):
    dico = {
        'apm-core-reliability-and-performance': "agent-apm",
        'universal-service-monitoring': "usm",
        'software-integrity-and-trust': "agent-security",
        'agent-all': "triage",
        'telemetry-and-analytics': "agent-apm",
        'fleet': "fleet-automation",
        'debugger': "dynamic-intrumentation",
        'container-integrations': "containers",
        'agent-e2e-testing': "agent-e2e-test",
        'agent-integrations': "integrations",
        'asm-go': "agent-security",
    }
    return dico.get(team, team)


@contextmanager
def download_to_tempfile(url, checksum=None):
    """
    Download a file from @url to a temporary file and yields the path.

    The temporary file is removed when the context manager exits.

    if @checksum is provided it will be updated with each chunk of the file
    """
    fd, tmp_path = tempfile.mkstemp()
    try:
        with requests.get(url, stream=True) as r:
            r.raise_for_status()
            with os.fdopen(fd, "wb") as f:
                # fd will be closed by context manager, so we no longer need it
                fd = None
                for chunk in r.iter_content(chunk_size=8192):
                    if checksum:
                        checksum.update(chunk)
                    f.write(chunk)
        yield tmp_path
    finally:
        if fd is not None:
            os.close(fd)
        if os.path.exists(tmp_path):
            os.remove(tmp_path)


def experimental(message):
    """
    Marks this task as experimental and prints the message.

    Note: This decorator must be placed after the `task` decorator.
    """

    def decorator(f):
        @wraps(f)
        def wrapper(*args, **kwargs):
            fname = f.__name__
            print(color_message(f"Warning: {fname} is experimental: {message}", Color.ORANGE), file=sys.stderr)

            return f(*args, **kwargs)

        return wrapper

    return decorator
