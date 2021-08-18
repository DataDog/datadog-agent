"""
Miscellaneous functions, no tasks here
"""


import json
import os
import re
import sys
from subprocess import check_output

from invoke import task

# constants
ORG_PATH = "github.com/DataDog"
DEFAULT_BRANCH = "main"
REPO_PATH = "{}/datadog-agent".format(ORG_PATH)
ALLOWED_REPO_NON_NIGHTLY_BRANCHES = {"stable", "beta", "none"}
ALLOWED_REPO_NIGHTLY_BRANCHES = {"nightly", "oldnightly"}
ALLOWED_REPO_ALL_BRANCHES = ALLOWED_REPO_NON_NIGHTLY_BRANCHES.union(ALLOWED_REPO_NIGHTLY_BRANCHES)


def get_all_allowed_repo_branches():
    return ALLOWED_REPO_ALL_BRANCHES


def is_allowed_repo_branch(branch):
    return branch in ALLOWED_REPO_ALL_BRANCHES


def is_allowed_repo_nightly_branch(branch):
    return branch in ALLOWED_REPO_NIGHTLY_BRANCHES


def bin_name(name, android=False):
    """
    Generate platform dependent names for binaries
    """
    if android:
        return "{}.aar".format(name)

    if sys.platform == 'win32':
        return "{}.exe".format(name)
    return name


def get_gopath(ctx):
    gopath = os.environ.get("GOPATH")
    if not gopath:
        gopath = ctx.run("go env GOPATH", hide=True).stdout.strip()

    return gopath


def get_multi_python_location(embedded_path=None, rtloader_root=None):
    rtloader_lib = []
    if rtloader_root is None:
        for libdir in ["lib", "lib64"]:
            libpath = os.path.join(embedded_path, libdir)
            if os.path.exists(libpath):
                rtloader_lib.append(libpath)
    else:  # if rtloader_root is specified we're working in dev mode from the rtloader folder
        rtloader_lib.append("{}/rtloader".format(rtloader_root))

    rtloader_headers = "{}/include".format(rtloader_root or embedded_path)
    rtloader_common_headers = "{}/common".format(rtloader_root or embedded_path)

    return rtloader_lib, rtloader_headers, rtloader_common_headers


def get_nikos_linker_flags(nikos_libs_path):
    nikos_libs = [
        'dnf',
        'gio-2.0',
        'modulemd',
        'gobject-2.0',
        'ffi',
        'yaml',
        'gmodule-2.0',
        'repo',
        'glib-2.0',
        'pcre',
        'z',
        'solvext',
        'rpm',
        'rpmio',
        'bz2',
        'solv',
        'gpgme',
        'assuan',
        'gcrypt',
        'gpg-error',
        'sqlite3',
        'curl',
        'nghttp2',
        'ssl',
        'crypto',
        'json-c',
        'lzma',
        'xml2',
        'popt',
        'zstd',
    ]
    # hardcode the path to each library to ensure we link against the version which was built by omnibus-nikos
    linker_flags = map(lambda lib: nikos_libs_path + '/lib' + lib + '.a', nikos_libs)

    return (
        ' -L' + nikos_libs_path + ' ' + ' '.join(linker_flags) + ' -l:libresolv.a -static-libstdc++ -pthread -ldl -lm'
    )


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
    nikos_embedded_path=None,
):
    """
    Build the common value for both ldflags and gcflags, and return an env accordingly.

    We need to invoke external processes here so this function need the
    Context object.
    """
    gcflags = ""
    ldflags = get_version_ldflags(ctx, prefix, major_version=major_version)
    env = {"GO111MODULE": "on"}

    if sys.platform == 'win32':
        env["CGO_LDFLAGS_ALLOW"] = "-Wl,--allow-multiple-definition"
    else:
        # for pkg/ebpf/compiler on linux
        env['CGO_LDFLAGS_ALLOW'] = "-Wl,--wrap=.*"

    if embedded_path is None:
        # fall back to local dev path
        embedded_path = "{}/src/github.com/DataDog/datadog-agent/dev".format(get_gopath(ctx))

    rtloader_lib, rtloader_headers, rtloader_common_headers = get_multi_python_location(embedded_path, rtloader_root)

    # setting python homes in the code
    if python_home_2:
        ldflags += "-X {}/pkg/collector/python.pythonHome2={} ".format(REPO_PATH, python_home_2)
    if python_home_3:
        ldflags += "-X {}/pkg/collector/python.pythonHome3={} ".format(REPO_PATH, python_home_3)

    # If we're not building with both Python, we want to force the use of DefaultPython
    if not has_both_python(python_runtimes):
        ldflags += "-X {}/pkg/config.ForceDefaultPython=true ".format(REPO_PATH)

    ldflags += "-X {}/pkg/config.DefaultPython={} ".format(REPO_PATH, get_default_python(python_runtimes))

    # adding rtloader libs and headers to the env
    if rtloader_lib:
        env['DYLD_LIBRARY_PATH'] = os.environ.get('DYLD_LIBRARY_PATH', '') + ":{}".format(':'.join(rtloader_lib))  # OSX
        env['LD_LIBRARY_PATH'] = os.environ.get('LD_LIBRARY_PATH', '') + ":{}".format(':'.join(rtloader_lib))  # linux
        env['CGO_LDFLAGS'] = os.environ.get('CGO_LDFLAGS', '') + " -L{}".format(' -L '.join(rtloader_lib))
    env['CGO_CFLAGS'] = os.environ.get('CGO_CFLAGS', '') + " -Werror -Wno-deprecated-declarations -I{} -I{}".format(
        rtloader_headers, rtloader_common_headers
    )

    # adding nikos libs to the env
    if nikos_embedded_path:
        env['PKG_CONFIG_PATH'] = env.get('PKG_CONFIG_PATH', '') + ':' + nikos_embedded_path + '/lib/pkgconfig'
        env["CGO_LDFLAGS"] = env.get('CGO_LDFLAGS', '') + get_nikos_linker_flags(nikos_embedded_path + '/lib')

    # if `static` was passed ignore setting rpath, even if `embedded_path` was passed as well
    if static:
        ldflags += "-s -w -linkmode=external '-extldflags=-static' "
    elif rtloader_lib:
        ldflags += "-r {} ".format(':'.join(rtloader_lib))

    if os.environ.get("DELVE"):
        gcflags = "all=-N -l"
        if sys.platform == 'win32':
            # On windows, need to build with the extra argument -ldflags="-linkmode internal"
            # if you want to be able to use the delve debugger.
            ldflags += "-linkmode internal "
    elif os.environ.get("NO_GO_OPT"):
        gcflags = "-N -l"

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
            if pkgname == "github.com/DataDog/agent-payload":
                # Example of line
                # github.com/DataDog/agent-payload v4.40.0+incompatible
                # github.com/DataDog/agent-payload v4.42.1-0.20200826134834-1ddcfb686e3f+incompatible
                version_split = re.split(r'[ +]', line)
                if len(version_split) < 2:
                    raise Exception(
                        "Versioning of agent-payload in go.mod has changed, the version logic needs to be updated"
                    )
                version = version_split[1].split("-")[0].strip()
                if not re.search(r"^v\d+(\.\d+){2}$", version):
                    raise Exception("Version of agent-payload in go.mod is invalid: '{}'".format(version))
                return version

    raise Exception("Could not find valid version for agent-payload in go.mod file")


def get_version_ldflags(ctx, prefix=None, major_version='7'):
    """
    Compute the version from the git tags, and set the appropriate compiler
    flags
    """
    payload_v = get_payload_version()
    commit = get_git_commit()

    ldflags = "-X {}/pkg/version.Commit={} ".format(REPO_PATH, commit)
    ldflags += "-X {}/pkg/version.AgentVersion={} ".format(
        REPO_PATH, get_version(ctx, include_git=True, prefix=prefix, major_version=major_version)
    )
    ldflags += "-X {}/pkg/serializer.AgentPayloadVersion={} ".format(REPO_PATH, payload_v)

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
        cmd += " --match \"{}-*\"".format(prefix)
    else:
        if major_version_hint:
            cmd += r' --match "{}\.*"'.format(major_version_hint)
        else:
            cmd += " --match \"[0-9]*\""
    if git_sha_length and type(git_sha_length) == int:
        cmd += " --abbrev={}".format(git_sha_length)
    described_version = ctx.run(cmd, hide=True).stdout.strip()

    # for the example above, 6.0.0-beta.0-1-g4f19118, this will be 1
    commit_number_match = re.match(r"^.*-(?P<commit_number>\d+)-g[0-9a-f]+$", described_version)
    commit_number = 0
    if commit_number_match:
        commit_number = int(commit_number_match.group('commit_number'))

    version_re = r"v?(?P<version>\d+\.\d+\.\d+)(?:(?:-|\.)(?P<pre>[0-9A-Za-z.-]+))?"
    if prefix and type(prefix) == str:
        version_re = r"^(?:{}-)?".format(prefix) + version_re
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


def get_version(
    ctx, include_git=False, url_safe=False, git_sha_length=7, prefix=None, major_version='7', include_pipeline_id=False
):
    # we only need the git info for the non omnibus builds, omnibus includes all this information by default

    version = ""
    version, pre, commits_since_version, git_sha, pipeline_id = query_version(
        ctx, git_sha_length, prefix, major_version_hint=major_version
    )

    is_nightly = is_allowed_repo_nightly_branch(os.getenv("DEB_RPM_BUCKET_BRANCH"))
    if pre:
        version = "{0}-{1}".format(version, pre)

    if not commits_since_version and is_nightly and include_git:
        if url_safe:
            version = "{0}.git.{1}.{2}".format(version, 0, git_sha)
        else:
            version = "{0}+git.{1}.{2}".format(version, 0, git_sha)

    if commits_since_version and include_git:
        if url_safe:
            version = "{0}.git.{1}.{2}".format(version, commits_since_version, git_sha)
        else:
            version = "{0}+git.{1}.{2}".format(version, commits_since_version, git_sha)

    if is_nightly and include_git and include_pipeline_id and pipeline_id is not None:
        version = "{0}.pipeline.{1}".format(version, pipeline_id)

    # version could be unicode as it comes from `query_version`
    return str(version)


def get_version_numeric_only(ctx, major_version='7'):
    # we only need the git info for the non omnibus builds, omnibus includes all this information by default

    version, *_ = query_version(ctx, major_version_hint=major_version)
    return version


def load_release_versions(_, target_version):
    with open("release.json", "r") as f:
        versions = json.load(f)
        if target_version in versions:
            # windows runners don't accepts anything else than strings in the
            # environment when running a subprocess.
            return {str(k): str(v) for k, v in versions[target_version].items()}
    raise Exception("Could not find '{}' version in release.json".format(target_version))


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


def bundle_files(ctx, bindata_files, dir_prefix, go_dir, pkg, tag, split=True):
    assets_cmd = (
        "go run github.com/shuLhan/go-bindata/cmd/go-bindata -tags '{bundle_tag}' {split}"
        + " -pkg {pkg} -prefix '{dir_prefix}' -modtime 1 -o '{go_dir}' '{bindata_files}'"
    )
    ctx.run(
        assets_cmd.format(
            dir_prefix=dir_prefix,
            go_dir=go_dir,
            bundle_tag=tag,
            pkg=pkg,
            split="-split" if split else "",
            bindata_files="' '".join(bindata_files),
        )
    )
    ctx.run("gofmt -w -s {go_dir}".format(go_dir=go_dir))
