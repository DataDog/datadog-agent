"""
High level testing tasks
"""


import copy
import operator
import os
import re
import sys
from contextlib import contextmanager

import yaml
from invoke import task
from invoke.exceptions import Exit

from .agent import integration_tests as agent_integration_tests
from .build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from .cluster_agent import integration_tests as dca_integration_tests
from .dogstatsd import integration_tests as dsd_integration_tests
from .go import fmt, generate, golangci_lint, ineffassign, lint, misspell, staticcheck, vet
from .modules import DEFAULT_MODULES, GoModule
from .trace_agent import integration_tests as trace_integration_tests
from .utils import DEFAULT_BRANCH, get_build_flags

PROFILE_COV = "profile.cov"


def ensure_bytes(s):
    if not isinstance(s, bytes):
        return s.encode('utf-8')

    return s


@contextmanager
def environ(env):
    original_environ = os.environ.copy()
    os.environ.update(env)
    yield
    for var in env.keys():
        if var in original_environ:
            os.environ[var] = original_environ[var]
        else:
            os.environ.pop(var)


TOOL_LIST = [
    'github.com/client9/misspell/cmd/misspell',
    'github.com/frapposelli/wwhrd',
    'github.com/fzipp/gocyclo',
    'github.com/go-enry/go-license-detector/v4/cmd/license-detector',
    'github.com/golangci/golangci-lint/cmd/golangci-lint',
    'github.com/gordonklaus/ineffassign',
    'github.com/goware/modvendor',
    'github.com/mgechev/revive',
    'github.com/stormcat24/protodep',
    'gotest.tools/gotestsum',
    'honnef.co/go/tools/cmd/staticcheck',
    'github.com/vektra/mockery/v2',
]

TOOL_LIST_PROTO = [
    'github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway',
    'github.com/golang/protobuf/protoc-gen-go',
    'github.com/golang/mock/mockgen',
]

TOOLS = {
    'internal/tools': TOOL_LIST,
    'internal/tools/proto': TOOL_LIST_PROTO,
}


@task
def install_tools(ctx):
    """Install all Go tools for testing."""
    with environ({'GO111MODULE': 'on'}):
        for path, tools in TOOLS.items():
            with ctx.cd(path):
                for tool in tools:
                    ctx.run("go install {}".format(tool))


@task()
def test(
    ctx,
    module=None,
    targets=None,
    coverage=False,
    build_include=None,
    build_exclude=None,
    verbose=False,
    race=False,
    profile=False,
    fail_on_fmt=False,
    rtloader_root=None,
    python_home_2=None,
    python_home_3=None,
    cpus=0,
    major_version='7',
    python_runtimes='3',
    timeout=180,
    arch="x64",
    cache=True,
    skip_linters=False,
    save_result_json=None,
    rerun_fails=None,
    go_mod="mod",
):
    """
    Run all the tools and tests on the given module and targets.

    A module should be provided as the path to one of the go modules in the repository.

    Targets should be provided as a comma-separated list of relative paths within the given module.
    If targets are provided but no module is set, the main module (".") is used.

    If no module or target is set the tests are run against all modules and targets.

    Example invokation:
        inv test --targets=./pkg/collector/check,./pkg/aggregator --race
        inv test --module=. --race
    """
    if isinstance(module, str):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        if isinstance(targets, str):
            modules = [GoModule(module, targets=targets.split(','))]
        else:
            modules = [m for m in DEFAULT_MODULES.values() if m.path == module]
    elif isinstance(targets, str):
        modules = [GoModule(".", targets=targets.split(','))]
    else:
        print("Using default modules and targets")
        modules = DEFAULT_MODULES.values()

    build_include = (
        get_default_build_tags(build="test-with-process-tags", arch=arch)
        if build_include is None
        else filter_incompatible_tags(build_include.split(","), arch=arch)
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    build_tags = get_build_tags(build_include, build_exclude)

    timeout = int(timeout)

    # explicitly run these tasks instead of using pre-tasks so we can
    # pass the `target` param (pre-tasks are invoked without parameters)
    print("--- go generating:")
    generate(ctx)

    if skip_linters:
        print("--- [skipping Go linters]")
    else:
        # Until all packages whitelisted in .golangci.yml are fixed and removed
        # from the 'skip-dirs' list we need to keep using the old functions that
        # lint without build flags (linting some file is better than no linting).
        print("--- Vetting and linting (legacy):")
        for module in modules:
            print("----- Module '{}'".format(module.full_path()))
            if not module.condition():
                print("----- Skipped")
                continue

            with ctx.cd(module.full_path()):
                vet(ctx, targets=module.targets, rtloader_root=rtloader_root, build_tags=build_tags, arch=arch)
                fmt(ctx, targets=module.targets, fail_on_fmt=fail_on_fmt)
                lint(ctx, targets=module.targets)
                misspell(ctx, targets=module.targets)
                ineffassign(ctx, targets=module.targets)
                staticcheck(ctx, targets=module.targets, build_tags=build_tags, arch=arch)

        # for now we only run golangci_lint on Unix as the Windows env need more work
        if sys.platform != 'win32':
            print("--- golangci_lint:")
            for module in modules:
                print("----- Module '{}'".format(module.full_path()))
                if not module.condition():
                    print("----- Skipped")
                    continue

                with ctx.cd(module.full_path()):
                    golangci_lint(
                        ctx, targets=module.targets, rtloader_root=rtloader_root, build_tags=build_tags, arch=arch
                    )

    with open(PROFILE_COV, "w") as f_cov:
        f_cov.write("mode: count")

    ldflags, gcflags, env = get_build_flags(
        ctx,
        rtloader_root=rtloader_root,
        python_home_2=python_home_2,
        python_home_3=python_home_3,
        major_version=major_version,
        python_runtimes=python_runtimes,
    )

    if sys.platform == 'win32':
        env['CGO_LDFLAGS'] += ' -Wl,--allow-multiple-definition'

    if profile:
        test_profiler = TestProfiler()
    else:
        test_profiler = None  # Use stdout

    race_opt = ""
    covermode_opt = ""
    build_cpus_opt = ""
    if cpus:
        build_cpus_opt = "-p {}".format(cpus)
    if race:
        # race doesn't appear to be supported on non-x64 platforms
        if arch == "x86":
            print("\n -- Warning... disabling race test, not supported on this platform --\n")
        else:
            race_opt = "-race"

        # Needed to fix an issue when using -race + gcc 10.x on Windows
        # https://github.com/bazelbuild/rules_go/issues/2614
        if sys.platform == 'win32':
            ldflags += " -linkmode=external"

    if coverage:
        if race:
            # atomic is quite expensive but it's the only way to run
            # both the coverage and the race detector at the same time
            # without getting false positives from the cover counter
            covermode_opt = "-covermode=atomic"
        else:
            covermode_opt = "-covermode=count"

    print("\n--- Running unit tests:")

    coverprofile = ""
    if coverage:
        coverprofile = "-coverprofile={}".format(PROFILE_COV)

    nocache = '-count=1' if not cache else ''

    build_tags.append("test")
    TMP_JSON = 'tmp.json'
    if save_result_json and os.path.isfile(save_result_json):
        # Remove existing file since we append to it.
        # We don't need to do that for TMP_JSON since gotestsum overwrites the output.
        print("Removing existing '{}' file".format(save_result_json))
        os.remove(save_result_json)

    cmd = 'gotestsum {json_flag} --format pkgname {rerun_fails} --packages="{packages}" -- {verbose} -mod={go_mod} -vet=off -timeout {timeout}s -tags "{go_build_tags}" -gcflags="{gcflags}" '
    cmd += '-ldflags="{ldflags}" {build_cpus} {race_opt} -short {covermode_opt} {coverprofile} {nocache}'
    args = {
        "go_mod": go_mod,
        "go_build_tags": " ".join(build_tags),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "race_opt": race_opt,
        "build_cpus": build_cpus_opt,
        "covermode_opt": covermode_opt,
        "coverprofile": coverprofile,
        "timeout": timeout,
        "verbose": '-v' if verbose else '',
        "nocache": nocache,
        "json_flag": '--jsonfile "{}" '.format(TMP_JSON) if save_result_json else "",
        "rerun_fails": "--rerun-fails={}".format(rerun_fails) if rerun_fails else "",
    }

    failed_modules = []
    for module in modules:
        print("----- Module '{}'".format(module.full_path()))
        if not module.condition():
            print("----- Skipped")
            continue

        with ctx.cd(module.full_path()):
            res = ctx.run(
                cmd.format(
                    packages=' '.join("{}/...".format(t) if not t.endswith("/...") else t for t in module.targets),
                    **args
                ),
                env=env,
                out_stream=test_profiler,
                warn=True,
            )

        if res.exited is None or res.exited > 0:
            failed_modules.append(module.full_path())

        if save_result_json:
            with open(save_result_json, 'ab') as json_file, open(
                os.path.join(module.full_path(), TMP_JSON), 'rb'
            ) as module_file:
                json_file.write(module_file.read())

    if failed_modules:
        # Exit if any of the modules failed
        raise Exit(code=1, message="Unit tests failed in the following modules: {}".format(', '.join(failed_modules)))

    if coverage:
        print("\n--- Test coverage:")
        ctx.run("go tool cover -func {}".format(PROFILE_COV))

    if profile:
        print("\n--- Top 15 packages sorted by run time:")
        test_profiler.print_sorted(15)


@task
def lint_teamassignment(_):
    """
    Make sure PRs are assigned a team label
    """
    branch = os.environ.get("CIRCLE_BRANCH")
    pr_url = os.environ.get("CIRCLE_PULL_REQUEST")

    if branch == DEFAULT_BRANCH:
        print("Running on {}, skipping check for team assignment.".format(DEFAULT_BRANCH))
    elif pr_url:
        import requests

        pr_id = pr_url.rsplit('/')[-1]

        res = requests.get("https://api.github.com/repos/DataDog/datadog-agent/issues/{}".format(pr_id))
        issue = res.json()

        for label in issue.get('labels', {}):
            if re.match('team/', label['name']):
                print("Team Assignment: {}".format(label['name']))
                return

        print("PR {} requires team assignment".format(pr_url))
        raise Exit(code=1)

    # No PR is associated with this build: given that we have the "run only on PRs" setting activated,
    # this can only happen when we're building on a tag. We don't need to check for a team assignment.
    else:
        print("PR not found, skipping check for team assignment.")


@task
def lint_milestone(_):
    """
    Make sure PRs are assigned a milestone
    """
    branch = os.environ.get("CIRCLE_BRANCH")
    pr_url = os.environ.get("CIRCLE_PULL_REQUEST")

    if branch == DEFAULT_BRANCH:
        print("Running on {}, skipping check for milestone.".format(DEFAULT_BRANCH))
    elif pr_url:
        import requests

        pr_id = pr_url.rsplit('/')[-1]

        res = requests.get("https://api.github.com/repos/DataDog/datadog-agent/issues/{}".format(pr_id))
        pr = res.json()
        if pr.get("milestone"):
            print("Milestone: {}".format(pr["milestone"].get("title", "NO_TITLE")))
            return

        print("PR {} requires a milestone.".format(pr_url))
        raise Exit(code=1)

    # No PR is associated with this build: given that we have the "run only on PRs" setting activated,
    # this can only happen when we're building on a tag. We don't need to check for a milestone.
    else:
        print("PR not found, skipping check for milestone.")


@task
def lint_releasenote(ctx):
    """
    Lint release notes with Reno
    """

    branch = os.environ.get("CIRCLE_BRANCH")
    pr_url = os.environ.get("CIRCLE_PULL_REQUEST")

    if branch == DEFAULT_BRANCH:
        print("Running on {}, skipping release note check.".format(DEFAULT_BRANCH))
    # Check if a releasenote has been added/changed
    elif pr_url:
        import requests

        pr_id = pr_url.rsplit('/')[-1]

        # first check 'changelog/no-changelog' label
        res = requests.get("https://api.github.com/repos/DataDog/datadog-agent/issues/{}".format(pr_id))
        issue = res.json()
        if any([l['name'] == 'changelog/no-changelog' for l in issue.get('labels', {})]):
            print("'changelog/no-changelog' label found on the PR: skipping linting")
            return

        # Then check that at least one note was touched by the PR
        url = "https://api.github.com/repos/DataDog/datadog-agent/pulls/{}/files".format(pr_id)
        # traverse paginated github response
        while True:
            res = requests.get(url)
            files = res.json()
            if any(
                [
                    f['filename'].startswith("releasenotes/notes/")
                    or f['filename'].startswith("releasenotes-dca/notes/")
                    or f['filename'].startswith("releasenotes-installscript/notes/")
                    for f in files
                ]
            ):
                break

            if 'next' in res.links:
                url = res.links['next']['url']
            else:
                print(
                    "Error: No releasenote was found for this PR. Please add one using 'reno'"
                    ", or apply the label 'changelog/no-changelog' to the PR."
                )
                raise Exit(code=1)
    # No PR is associated with this build: given that we have the "run only on PRs" setting activated,
    # this can only happen when we're building on a tag. We don't need to check for release notes.
    else:
        print("PR not found, skipping release note check.")

    ctx.run("reno lint")


@task
def lint_filenames(ctx):
    """
    Scan files to ensure there are no filenames too long or containing illegal characters
    """
    files = ctx.run("git ls-files -z", hide=True).stdout.split("\0")
    failure = False

    if sys.platform == 'win32':
        print("Running on windows, no need to check filenames for illegal characters")
    else:
        print("Checking filenames for illegal characters")
        forbidden_chars = '<>:"\\|?*'
        for file in files:
            if any(char in file for char in forbidden_chars):
                print("Error: Found illegal character in path {}".format(file))
                failure = True

    print("Checking filename length")
    # Approximated length of the prefix of the repo during the windows release build
    prefix_length = 160
    # Maximum length supported by the win32 API
    max_length = 255
    for file in files:
        if not file.startswith('test/kitchen/') and prefix_length + len(file) > max_length:
            print(
                "Error: path {} is too long ({} characters too many)".format(
                    file, prefix_length + len(file) - max_length
                )
            )
            failure = True

    if failure:
        raise Exit(code=1)


@task
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False):
    """
    Run all the available integration tests
    """
    agent_integration_tests(ctx, install_deps, race, remote_docker)
    dsd_integration_tests(ctx, install_deps, race, remote_docker)
    dca_integration_tests(ctx, install_deps, race, remote_docker)
    trace_integration_tests(ctx, install_deps, race, remote_docker)


@task
def e2e_tests(ctx, target="gitlab", agent_image="", dca_image=""):
    """
    Run e2e tests in several environments.
    """
    choices = ["gitlab", "dev", "local"]
    if target not in choices:
        print('target %s not in %s' % (target, choices))
        raise Exit(1)
    if not os.getenv("DATADOG_AGENT_IMAGE"):
        if not agent_image:
            print("define DATADOG_AGENT_IMAGE envvar or image flag")
            raise Exit(1)
        os.environ["DATADOG_AGENT_IMAGE"] = agent_image
    if not os.getenv("DATADOG_CLUSTER_AGENT_IMAGE"):
        if not dca_image:
            print("define DATADOG_CLUSTER_AGENT_IMAGE envvar or image flag")
            raise Exit(1)
        os.environ["DATADOG_CLUSTER_AGENT_IMAGE"] = dca_image

    ctx.run("./test/e2e/scripts/setup-instance/00-entrypoint-%s.sh" % target)


class TestProfiler:
    times = []
    parser = re.compile(r"^ok\s+github.com\/DataDog\/datadog-agent\/(\S+)\s+([0-9\.]+)s", re.MULTILINE)

    def write(self, txt):
        # Output to stdout
        # NOTE: write to underlying stream on Python 3 to avoid unicode issues when default encoding is not UTF-8
        getattr(sys.stdout, 'buffer', sys.stdout).write(ensure_bytes(txt))
        # Extract the run time
        for result in self.parser.finditer(txt):
            self.times.append((result.group(1), float(result.group(2))))

    def flush(self):
        sys.stdout.flush()

    def reset(self):
        self.out_buffer = ""

    def print_sorted(self, limit=0):
        if self.times:
            sorted_times = sorted(self.times, key=operator.itemgetter(1), reverse=True)

            if limit:
                sorted_times = sorted_times[:limit]
            for pkg, time in sorted_times:
                print("{}s\t{}".format(time, pkg))


@task
def make_simple_gitlab_yml(
    _, jobs_to_process, yml_file_src='.gitlab-ci.yml', yml_file_dest='.gitlab-ci.yml', dont_include_deps=False
):
    """
    Replaces .gitlab-ci.yml with one containing only the steps needed to run the given jobs.

    Keyword arguments:
        jobs_to_run -- a comma separated list of jobs to execute, for example "iot_agent_rpm-arm64,iot_agent_rpm-armhf"
        yml_file_src -- the source YAML file
        yml_file_dest -- the destination YAML file
        dont_include_deps -- this flag controls whether or not dependent jobs will be included in the final job list. Specify it if you only want to run the jobs listed in 'jobs_to_run'
    """
    with open(yml_file_src) as f:
        data = yaml.load(f, Loader=yaml.FullLoader)

    jobs_processed = set(['stages', 'variables', 'include', 'default'])
    jobs_to_process = set(jobs_to_process.split(','))
    while jobs_to_process:
        job_name = jobs_to_process.pop()
        if job_name in data:
            job = data[job_name]
            jobs_processed.add(job_name)

            # Process dependencies
            if not dont_include_deps:
                needs = job.get("needs", None)
                if needs is not None:
                    jobs_to_process.update(needs)

            # Process base jobs
            extends = job.get("extends", None)
            if extends is not None:
                if isinstance(extends, str):
                    extends = [extends]
                jobs_to_process.update(extends)

            # Delete rules that may prevent our job from running
            if 'rules' in job:
                del job['rules']
            if 'except' in job:
                del job['except']
            if 'only' in job:
                del job['only']

    out = copy.deepcopy(data)
    for k, _ in data.items():
        if k not in jobs_processed:
            del out[k]
            continue

    with open(yml_file_dest, 'w') as f:
        yaml.dump(out, f)


@task
def make_kitchen_gitlab_yml(_):
    """
    Replaces .gitlab-ci.yml with one containing only the steps needed to run kitchen-tests
    """
    with open('.gitlab-ci.yml') as f:
        data = yaml.load(f, Loader=yaml.FullLoader)

    data['stages'] = [
        'deps_build',
        'deps_fetch',
        'binary_build',
        'package_build',
        'testkitchen_deploy',
        'testkitchen_testing',
        'testkitchen_cleanup',
    ]
    for name, job in data.items():
        if isinstance(job, dict) and job.get('stage', None) not in ([None] + data['stages']):
            del data[name]
            continue
        if (
            isinstance(job, dict)
            and job.get('stage', None) == 'binary_build'
            and name != 'build_system-probe-arm64'
            and name != 'build_system-probe-x64'
        ):
            del data[name]
            continue
        if 'except' in job:
            del job['except']
        if 'only' in job:
            del job['only']
        if 'rules' in job:
            del job['rules']
        if len(job) == 0:
            del data[name]
            continue

    for name, job in data.items():
        if 'extends' in job:
            extended = job['extends']
            if not isinstance(extended, list):
                extended = [extended]
            for job in extended:
                if job not in data:
                    del data[name]

    for _, job in data.items():
        if 'needs' in job:
            needed = job['needs']
            new_needed = []
            for n in needed:
                if n in data:
                    new_needed.append(n)
            job['needs'] = new_needed

    with open('.gitlab-ci.yml', 'w') as f:
        yaml.dump(data, f, default_style='"')


@task
def check_gitlab_broken_dependencies(_):
    """
    Checks that a gitlab job doesn't depend on (need) other jobs that will be excluded from the build,
    since this would make gitlab fail when triggering a pipeline with those jobs excluded.
    """
    with open('.gitlab-ci.yml') as f:
        data = yaml.load(f, Loader=yaml.FullLoader)

    def is_unwanted(job, version):
        e = job.get('except', {})
        return isinstance(e, dict) and '$RELEASE_VERSION_{} == ""'.format(version) in e.get('variables', {})

    for version in [6, 7]:
        for k, v in data.items():
            if isinstance(v, dict) and not is_unwanted(v, version) and "needs" in v:
                needed = v['needs']
                for need in needed:
                    if is_unwanted(data[need], version):
                        print("{} needs on {} but it won't be built for A{}".format(k, need, version))


@task
def lint_python(ctx):
    """
    Lints Python files.
    See 'setup.cfg' and 'pyproject.toml' file for configuration.
    If running locally, you probably want to use the pre-commit instead.
    """

    print(
        """Remember to set up pre-commit to lint your files before committing:
    https://github.com/DataDog/datadog-agent/blob/{}/docs/dev/agent_dev_env.md#pre-commit-hooks""".format(
            DEFAULT_BRANCH
        )
    )

    ctx.run("flake8 .")
    ctx.run("black --check --diff .")
    ctx.run("isort --check-only --diff .")


@task
def install_shellcheck(ctx, version="0.7.0", destination="/usr/local/bin"):
    """
    Installs the requested version of shellcheck in the specified folder (by default /usr/local/bin).
    Required to run the shellcheck pre-commit hook.
    """

    if sys.platform == 'win32':
        print("shellcheck is not supported on Windows")
        raise Exit(code=1)
    if sys.platform.startswith('darwin'):
        platform = "darwin"
    if sys.platform.startswith('linux'):
        platform = "linux"

    ctx.run(
        "wget -qO- \"https://github.com/koalaman/shellcheck/releases/download/v{sc_version}/shellcheck-v{sc_version}.{platform}.x86_64.tar.xz\" | tar -xJv -C /tmp".format(
            sc_version=version, platform=platform
        )
    )
    ctx.run(
        "cp \"/tmp/shellcheck-v{sc_version}/shellcheck\" {destination}".format(
            sc_version=version, destination=destination
        )
    )
    ctx.run("rm -rf \"/tmp/shellcheck-v{sc_version}\"".format(sc_version=version))
