"""
Miscellaneous functions, no tasks here
"""


import contextlib
import json
import os
import re
import sys
import time
from subprocess import check_output
from types import SimpleNamespace

from invoke import task
from invoke.exceptions import Exit

from .libs.common.color import color_message

# constants
DEFAULT_BRANCH = "main"
GITHUB_ORG = "DataDog"
REPO_NAME = "datadog-agent"
GITHUB_REPO_NAME = f"{GITHUB_ORG}/{REPO_NAME}"
REPO_PATH = f"github.com/{GITHUB_REPO_NAME}"
ALLOWED_REPO_NON_NIGHTLY_BRANCHES = {"stable", "beta", "none"}
ALLOWED_REPO_NIGHTLY_BRANCHES = {"nightly", "oldnightly"}
ALLOWED_REPO_ALL_BRANCHES = ALLOWED_REPO_NON_NIGHTLY_BRANCHES.union(ALLOWED_REPO_NIGHTLY_BRANCHES)
if sys.platform == "darwin":
    RTLOADER_LIB_NAME = "libdatadog-agent-rtloader.dylib"
elif sys.platform == "win32":
    RTLOADER_LIB_NAME = "libdatadog-agent-rtloader.a"
else:
    RTLOADER_LIB_NAME = "libdatadog-agent-rtloader.so"
RTLOADER_HEADER_NAME = "datadog_agent_rtloader.h"
AGENT_VERSION_CACHE_NAME = "agent-version.cache"


def get_all_allowed_repo_branches():
    return ALLOWED_REPO_ALL_BRANCHES


def is_allowed_repo_branch(branch):
    return branch in ALLOWED_REPO_ALL_BRANCHES


def is_allowed_repo_nightly_branch(branch):
    return branch in ALLOWED_REPO_NIGHTLY_BRANCHES


def bin_name(name):
    """
    Generate platform dependent names for binaries
    """
    if sys.platform == 'win32':
        return f"{name}.exe"
    return name


def get_gopath(ctx):
    gopath = os.environ.get("GOPATH")
    if not gopath:
        gopath = ctx.run("go env GOPATH", hide=True).stdout.strip()

    return gopath


def get_gobin(ctx):
    gobin = os.environ.get("GOBIN")
    if not gobin:
        gobin = ctx.run("go env GOBIN", hide=True).stdout.strip()
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


def has_both_python(python_runtimes):
    python_runtimes = python_runtimes.split(',')
    return '2' in python_runtimes and '3' in python_runtimes


def get_win_py_runtime_var(python_runtimes):
    python_runtimes = python_runtimes.split(',')

    return "PY2_RUNTIME" if '2' in python_runtimes else "PY3_RUNTIME"


def get_build_flags(
    ctx,
    static=False,
    prefix=None,
    embedded_path=None,
    rtloader_root=None,
    python_home_2=None,
    python_home_3=None,
    major_version='7',
    python_runtimes='3',
):
    """
    Build the common value for both ldflags and gcflags, and return an env accordingly.

    We need to invoke external processes here so this function need the
    Context object.
    """
    gcflags = ""
    ldflags = get_version_ldflags(ctx, prefix, major_version=major_version)
    # External linker flags; needs to be handled separately to avoid overrides
    extldflags = ""
    env = {"GO111MODULE": "on"}

    if sys.platform == 'win32':
        env["CGO_LDFLAGS_ALLOW"] = "-Wl,--allow-multiple-definition"
    else:
        # for pkg/ebpf/compiler on linux
        env['CGO_LDFLAGS_ALLOW'] = "-Wl,--wrap=.*"

    if embedded_path is None:
        base = os.path.dirname(os.path.abspath(__file__))
        task_repo_root = os.path.abspath(os.path.join(base, ".."))
        git_repo_root = get_root()
        gopath_root = f"{get_gopath(ctx)}/src/github.com/DataDog/datadog-agent"

        for root_candidate in [task_repo_root, git_repo_root, gopath_root]:
            test_embedded_path = os.path.join(root_candidate, "dev")
            if os.path.exists(test_embedded_path):
                embedded_path = test_embedded_path

    if embedded_path is None:
        raise Exit("unable to locate embedded path please check your setup or set --embedded-path")

    rtloader_lib, rtloader_headers, rtloader_common_headers = get_rtloader_paths(embedded_path, rtloader_root)

    # setting python homes in the code
    if python_home_2:
        ldflags += f"-X {REPO_PATH}/pkg/collector/python.pythonHome2={python_home_2} "
    if python_home_3:
        ldflags += f"-X {REPO_PATH}/pkg/collector/python.pythonHome3={python_home_3} "

    # If we're not building with both Python, we want to force the use of DefaultPython
    if not has_both_python(python_runtimes):
        ldflags += f"-X {REPO_PATH}/pkg/config.ForceDefaultPython=true "

    ldflags += f"-X {REPO_PATH}/pkg/config.DefaultPython={get_default_python(python_runtimes)} "

    # adding rtloader libs and headers to the env
    if rtloader_lib:
        print(
            f"--- Setting rtloader paths to lib:{','.join(rtloader_lib)} | header:{rtloader_headers} | common headers:{rtloader_common_headers}"
        )
        env['DYLD_LIBRARY_PATH'] = os.environ.get('DYLD_LIBRARY_PATH', '') + f":{':'.join(rtloader_lib)}"  # OSX
        env['LD_LIBRARY_PATH'] = os.environ.get('LD_LIBRARY_PATH', '') + f":{':'.join(rtloader_lib)}"  # linux
        env['CGO_LDFLAGS'] = os.environ.get('CGO_LDFLAGS', '') + f" -L{' -L '.join(rtloader_lib)}"

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

    # On macOS work around https://github.com/golang/go/issues/38824
    # as done in https://go-review.googlesource.com/c/go/+/372798
    if sys.platform == "darwin":
        extldflags += "-Wl,-bind_at_load "

    if extldflags:
        ldflags += f"'-extldflags={extldflags}' "

    return ldflags, gcflags, env


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


def get_version_ldflags(ctx, prefix=None, major_version='7'):
    """
    Compute the version from the git tags, and set the appropriate compiler
    flags
    """
    payload_v = get_payload_version()
    commit = get_git_commit()

    ldflags = f"-X {REPO_PATH}/pkg/version.Commit={commit} "
    ldflags += f"-X {REPO_PATH}/pkg/version.AgentVersion={get_version(ctx, include_git=True, prefix=prefix, major_version=major_version)} "
    ldflags += f"-X {REPO_PATH}/pkg/serializer.AgentPayloadVersion={payload_v} "

    return ldflags


def get_git_commit():
    """
    Get the current commit
    """
    return check_output(['git', 'rev-parse', '--short', 'HEAD']).decode('utf-8').strip()


def get_default_python(python_runtimes):
    """
    Get the default python for the current build:
    - default to 2 if python_runtimes includes 2 (so that builds with 2 and 3 default to 2)
    - default to 3 otherwise.
    """
    return "2" if '2' in python_runtimes.split(',') else "3"


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


def query_version(ctx, git_sha_length=7, prefix=None, major_version_hint=None):
    # The string that's passed in will look something like this: 6.0.0-beta.0-1-g4f19118
    # if the tag is 6.0.0-beta.0, it has been one commit since the tag and that commit hash is g4f19118
    cmd = "git describe --tags --candidates=50"
    if prefix and type(prefix) == str:
        cmd += f" --match \"{prefix}-*\""
    else:
        if major_version_hint:
            cmd += r' --match "{}\.*"'.format(major_version_hint)  # noqa: FS002
        else:
            cmd += " --match \"[0-9]*\""
    if git_sha_length and type(git_sha_length) == int:
        cmd += f" --abbrev={git_sha_length}"
    described_version = ctx.run(cmd, hide=True).stdout.strip()

    # for the example above, 6.0.0-beta.0-1-g4f19118, this will be 1
    commit_number_match = re.match(r"^.*-(?P<commit_number>\d+)-g[0-9a-f]+$", described_version)
    commit_number = 0
    if commit_number_match:
        commit_number = int(commit_number_match.group('commit_number'))

    version_re = r"v?(?P<version>\d+\.\d+\.\d+)(?:(?:-|\.)(?P<pre>[0-9A-Za-z.-]+))?"
    if prefix and type(prefix) == str:
        version_re = r"^(?:{}-)?".format(prefix) + version_re  # noqa: FS002
    else:
        version_re = r"^" + version_re
    if commit_number == 0:
        version_re += r"(?P<git_sha>)$"
    else:
        version_re += r"-\d+-g(?P<git_sha>[0-9a-f]+)$"

    version_match = re.match(version_re, described_version)

    if not version_match:
        raise Exception("Could not query valid version from tags of local git repository")

    # version: for the tag 6.0.0-beta.0, this will match 6.0.0
    # pre: for the output, 6.0.0-beta.0-1-g4f19118, this will match beta.0
    # if there have been no commits since, it will be just 6.0.0-beta.0,
    # and it will match beta.0
    # git_sha: for the output, 6.0.0-beta.0-1-g4f19118, this will match g4f19118
    version, pre, git_sha = version_match.group('version', 'pre', 'git_sha')

    # When we're on a tag, `git describe --tags --candidates=50` doesn't include a commit sha.
    # We need it, so we fetch it another way.
    if not git_sha:
        cmd = "git rev-parse HEAD"
        # The git sha shown by `git describe --tags --candidates=50` is the first 7 characters of the sha,
        # therefore we keep the same number of characters.
        git_sha = ctx.run(cmd, hide=True).stdout.strip()[:7]

    pipeline_id = os.getenv("CI_PIPELINE_ID", None)

    return version, pre, commit_number, git_sha, pipeline_id


def cache_version(ctx, git_sha_length=7, prefix=None):
    """
    Generate a json cache file containing all needed variables used by get_version.
    """
    packed_data = {}
    for maj_version in ['6', '7']:
        version, pre, commits_since_version, git_sha, pipeline_id = query_version(
            ctx, git_sha_length, prefix, major_version_hint=maj_version
        )
        packed_data[maj_version] = [version, pre, commits_since_version, git_sha, pipeline_id]
    packed_data["nightly"] = is_allowed_repo_nightly_branch(os.getenv("BUCKET_BRANCH"))
    with open(AGENT_VERSION_CACHE_NAME, "w") as file:
        json.dump(packed_data, file, indent=4)


def get_version(
    ctx, include_git=False, url_safe=False, git_sha_length=7, prefix=None, major_version='7', include_pipeline_id=False
):
    version = ""
    pipeline_id = os.getenv("CI_PIPELINE_ID")
    project_name = os.getenv("CI_PROJECT_NAME")
    try:
        agent_version_cache_file_exist = os.path.exists(AGENT_VERSION_CACHE_NAME)
        if not agent_version_cache_file_exist:
            if pipeline_id and pipeline_id.isdigit() and project_name == REPO_NAME:
                ctx.run(
                    f"aws s3 cp s3://dd-ci-artefacts-build-stable/datadog-agent/{pipeline_id}/{AGENT_VERSION_CACHE_NAME} .",
                    hide="stdout",
                )
                agent_version_cache_file_exist = True

        if agent_version_cache_file_exist:
            with open(AGENT_VERSION_CACHE_NAME, "r") as file:
                cache_data = json.load(file)

            version, pre, commits_since_version, git_sha, pipeline_id = cache_data[major_version]
            is_nightly = cache_data["nightly"]

            if pre:
                version = f"{version}-{pre}"
    except (IOError, json.JSONDecodeError, IndexError) as e:
        # If a cache file is found but corrupted we ignore it.
        print(f"Error while recovering the version from {AGENT_VERSION_CACHE_NAME}: {e}")
        version = ""
    # If we didn't load the cache
    if not version:
        print("[WARN] Agent version cache file hasn't been loaded !")
        # we only need the git info for the non omnibus builds, omnibus includes all this information by default
        version, pre, commits_since_version, git_sha, pipeline_id = query_version(
            ctx, git_sha_length, prefix, major_version_hint=major_version
        )

        is_nightly = is_allowed_repo_nightly_branch(os.getenv("BUCKET_BRANCH"))
        if pre:
            version = f"{version}-{pre}"

    if not commits_since_version and is_nightly and include_git:
        if url_safe:
            version = f"{version}.git.{0}.{git_sha}"
        else:
            version = f"{version}+git.{0}.{git_sha}"

    if commits_since_version and include_git:
        if url_safe:
            version = f"{version}.git.{commits_since_version}.{git_sha}"
        else:
            version = f"{version}+git.{commits_since_version}.{git_sha}"

    if is_nightly and include_git and include_pipeline_id and pipeline_id is not None:
        version = f"{version}.pipeline.{pipeline_id}"

    # version could be unicode as it comes from `query_version`
    return str(version)


def get_version_numeric_only(ctx, major_version='7'):
    # we only need the git info for the non omnibus builds, omnibus includes all this information by default
    version = ""
    pipeline_id = os.getenv("CI_PIPELINE_ID")
    project_name = os.getenv("CI_PROJECT_NAME")
    if pipeline_id and pipeline_id.isdigit() and project_name == REPO_NAME:
        try:
            if not os.path.exists(AGENT_VERSION_CACHE_NAME):
                ctx.run(
                    f"aws s3 cp s3://dd-ci-artefacts-build-stable/datadog-agent/{pipeline_id}/{AGENT_VERSION_CACHE_NAME} .",
                    hide="stdout",
                )

            with open(AGENT_VERSION_CACHE_NAME, "r") as file:
                cache_data = json.load(file)

            version, *_ = cache_data[major_version]
        except (IOError, json.JSONDecodeError, IndexError) as e:
            # If a cache file is found but corrupted we ignore it.
            print(f"Error while recovering the version from {AGENT_VERSION_CACHE_NAME}: {e}")
            version = ""
    if not version:
        version, *_ = query_version(ctx, major_version_hint=major_version)
    return version


def load_release_versions(_, target_version):
    with open("release.json", "r") as f:
        versions = json.load(f)
        if target_version in versions:
            # windows runners don't accepts anything else than strings in the
            # environment when running a subprocess.
            return {str(k): str(v) for k, v in versions[target_version].items()}
    raise Exception(f"Could not find '{target_version}' version in release.json")


@task()
def generate_config(ctx, build_type, output_file, env=None):
    """
    Generates the datadog.yaml configuration file.
    """
    args = {
        "go_file": "./pkg/config/render_config.go",
        "build_type": build_type,
        "template_file": "./pkg/config/config_template.yaml",
        "output_file": output_file,
    }
    cmd = "go run {go_file} {build_type} {template_file} {output_file}"
    return ctx.run(cmd.format(**args), env=env or {})


##
## release.json entry mapping functions
##


def nightly_entry_for(agent_major_version):
    if agent_major_version == 6:
        return "nightly"
    return f"nightly-a{agent_major_version}"


def release_entry_for(agent_major_version):
    return f"release-a{agent_major_version}"


def check_clean_branch_state(ctx, github, branch):
    """
    Check we are in a clean situation to create a new branch:
    No uncommitted change, and branch doesn't exist locally or upstream
    """
    if check_uncommitted_changes(ctx):
        raise Exit(
            color_message(
                "There are uncomitted changes in your repository. Please commit or stash them before trying again.",
                "red",
            ),
            code=1,
        )
    if check_local_branch(ctx, branch):
        raise Exit(
            color_message(
                f"The branch {branch} already exists locally. Please remove it before trying again.",
                "red",
            ),
            code=1,
        )

    if github.get_branch(branch) is not None:
        raise Exit(
            color_message(
                f"The branch {branch} already exists upstream. Please remove it before trying again.",
                "red",
            ),
            code=1,
        )


def check_uncommitted_changes(ctx):
    """
    Checks if there are uncommitted changes in the local git repository.
    """
    modified_files = ctx.run("git --no-pager diff --name-only HEAD | wc -l", hide=True).stdout.strip()

    # Return True if at least one file has uncommitted changes.
    return modified_files != "0"


def check_local_branch(ctx, branch):
    """
    Checks if the given branch exists locally
    """
    matching_branch = ctx.run(f"git --no-pager branch --list {branch} | wc -l", hide=True).stdout.strip()

    # Return True if a branch is returned by git branch --list
    return matching_branch != "0"


@contextlib.contextmanager
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
