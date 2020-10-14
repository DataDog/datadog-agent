"""
High level testing tasks
"""
from __future__ import print_function

import copy
import operator
import os
import re
import sys

import yaml
from invoke import task
from invoke.exceptions import Exit

from .agent import integration_tests as agent_integration_tests
from .build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from .cluster_agent import integration_tests as dca_integration_tests
from .dogstatsd import integration_tests as dsd_integration_tests
from .go import fmt, generate, golangci_lint, ineffassign, lint, lint_licenses, misspell, staticcheck, vet
from .trace_agent import integration_tests as trace_integration_tests
from .utils import get_build_flags

# We use `basestring` in the code for compat with python2 unicode strings.
# This makes the same code work in python3 as well.
try:
    basestring
except NameError:
    basestring = str

PROFILE_COV = "profile.cov"

DEFAULT_TOOL_TARGETS = [
    "./pkg",
    "./cmd",
]

DEFAULT_TEST_TARGETS = [
    "./pkg",
    "./cmd",
]


def ensure_bytes(s):
    if not isinstance(s, bytes):
        return s.encode('utf-8')

    return s


@task()
def test(
    ctx,
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
    timeout=120,
    arch="x64",
    cache=True,
    skip_linters=False,
    go_mod="vendor",
):
    """
    Run all the tools and tests on the given targets. If targets are not specified,
    the value from `invoke.yaml` will be used.

    Example invokation:
        inv test --targets=./pkg/collector/check,./pkg/aggregator --race
    """
    if isinstance(targets, basestring):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        tool_targets = test_targets = targets.split(',')
    elif targets is None:
        tool_targets = DEFAULT_TOOL_TARGETS
        test_targets = DEFAULT_TEST_TARGETS
    else:
        tool_targets = test_targets = targets

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
        print("--- [skipping linters]")
    else:
        print("--- Linting licenses:")
        lint_licenses(ctx)

        print("--- Linting filenames:")
        lint_filenames(ctx)

        # Until all packages whitelisted in .golangci.yml are fixed and removed
        # from the 'skip-dirs' list we need to keep using the old functions that
        # lint without build flags (linting some file is better than no linting).
        print("--- Vetting and linting (legacy):")
        vet(ctx, targets=tool_targets, rtloader_root=rtloader_root, build_tags=build_tags, arch=arch)
        fmt(ctx, targets=tool_targets, fail_on_fmt=fail_on_fmt)
        lint(ctx, targets=tool_targets)
        misspell(ctx, targets=tool_targets)
        ineffassign(ctx, targets=tool_targets)
        staticcheck(ctx, targets=tool_targets)

        # for now we only run golangci_lint on Unix as the Windows env need more work
        if sys.platform != 'win32':
            print("--- golangci_lint:")
            golangci_lint(ctx, targets=tool_targets, rtloader_root=rtloader_root, build_tags=build_tags, arch=arch)

    with open(PROFILE_COV, "w") as f_cov:
        f_cov.write("mode: count")

    ldflags, gcflags, env = get_build_flags(
        ctx,
        rtloader_root=rtloader_root,
        python_home_2=python_home_2,
        python_home_3=python_home_3,
        major_version=major_version,
        python_runtimes='3',
        arch=arch,
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

    if coverage:
        if race:
            # atomic is quite expensive but it's the only way to run
            # both the coverage and the race detector at the same time
            # without getting false positives from the cover counter
            covermode_opt = "-covermode=atomic"
        else:
            covermode_opt = "-covermode=count"

    matches = ["{}/...".format(t) for t in test_targets]
    print("\n--- Running unit tests:")

    coverprofile = ""
    if coverage:
        coverprofile = "-coverprofile={}".format(PROFILE_COV)

    nocache = '-count=1' if not cache else ''

    build_tags.append("test")
    cmd = 'gotestsum --format pkgname -- {verbose} -mod={go_mod} -vet=off -timeout {timeout}s -tags "{go_build_tags}" -gcflags="{gcflags}" '
    cmd += '-ldflags="{ldflags}" {build_cpus} {race_opt} -short {covermode_opt} {coverprofile} {nocache} {pkg_folder}'
    args = {
        "go_mod": go_mod,
        "go_build_tags": " ".join(build_tags),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "race_opt": race_opt,
        "build_cpus": build_cpus_opt,
        "covermode_opt": covermode_opt,
        "coverprofile": coverprofile,
        "pkg_folder": ' '.join(matches),
        "timeout": timeout,
        "verbose": '-v' if verbose else '',
        "nocache": nocache,
    }
    ctx.run(cmd.format(**args), env=env, out_stream=test_profiler)

    if coverage:
        print("\n--- Test coverage:")
        ctx.run("go tool cover -func {}".format(PROFILE_COV))

    if profile:
        print("\n--- Top 15 packages sorted by run time:")
        test_profiler.print_sorted(15)


@task
def lint_teamassignment(ctx):
    """
    Make sure PRs are assigned a team label
    """
    pr_url = os.environ.get("CIRCLE_PULL_REQUEST")
    if pr_url:
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

    # The PR has not been created yet
    else:
        print("PR not yet created, skipping check for team assignment")


@task
def lint_milestone(ctx):
    """
    Make sure PRs are assigned a milestone
    """
    pr_url = os.environ.get("CIRCLE_PULL_REQUEST")
    if pr_url:
        import requests

        pr_id = pr_url.rsplit('/')[-1]

        res = requests.get("https://api.github.com/repos/DataDog/datadog-agent/issues/{}".format(pr_id))
        pr = res.json()
        if pr.get("milestone"):
            print("Milestone: %s" % pr["milestone"].get("title", "NO_TITLE"))
            return

        print("PR %s requires a milestone" % pr_url)
        raise Exit(code=1)

    # The PR has not been created yet
    else:
        print("PR not yet created, skipping check for milestone")


@task
def lint_releasenote(ctx):
    """
    Lint release notes with Reno
    """

    # checking if a releasenote has been added/changed
    pr_url = os.environ.get("CIRCLE_PULL_REQUEST")
    if pr_url:
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

    # The PR has not been created yet, let's compare with master (the usual base branch of the future PR)
    else:
        branch = os.environ.get("CIRCLE_BRANCH")
        if branch is None:
            print("No branch found, skipping reno linting")
        else:
            if re.match(r".*/.*", branch) is None:
                print("{} is not a feature branch, skipping reno linting".format(branch))
            else:
                import requests

                # Then check that in the diff with master, at least one note was touched
                url = "https://api.github.com/repos/DataDog/datadog-agent/compare/master...{}".format(branch)
                # traverse paginated github response
                while True:
                    res = requests.get(url)
                    files = res.json().get("files", {})
                    if any(
                        [
                            f['filename'].startswith("releasenotes/notes/")
                            or f['filename'].startswith("releasenotes-dca/notes/")
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
def e2e_tests(ctx, target="gitlab", image=""):
    """
    Run e2e tests in several environments.
    """
    choices = ["gitlab", "dev", "local"]
    if target not in choices:
        print('target %s not in %s' % (target, choices))
        raise Exit(1)
    if not os.getenv("DATADOG_AGENT_IMAGE"):
        if not image:
            print("define DATADOG_AGENT_IMAGE envvar or image flag")
            raise Exit(1)
        os.environ["DATADOG_AGENT_IMAGE"] = image

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
    ctx, jobs_to_process, yml_file_src='.gitlab-ci.yml', yml_file_dest='.gitlab-ci.yml', dont_include_deps=False
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
def make_kitchen_gitlab_yml(ctx):
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
            and name != 'build_system-probe_with-bcc-arm64'
            and name != 'build_system-probe_with-bcc-x64'
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
def check_gitlab_broken_dependencies(ctx):
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
    https://github.com/DataDog/datadog-agent/blob/master/docs/dev/agent_dev_env.md#pre-commit-hooks"""
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
