"""
High level testing tasks
"""
from __future__ import print_function

import os
import fnmatch
import re
import operator
import sys
import yaml

import invoke
from invoke import task
from invoke.exceptions import Exit

from .utils import get_build_flags, get_version
from .go import fmt, lint, vet, misspell, ineffassign, lint_licenses, golangci_lint, generate
from .build_tags import get_default_build_tags, get_build_tags
from .agent import integration_tests as agent_integration_tests
from .dogstatsd import integration_tests as dsd_integration_tests
from .trace_agent import integration_tests as trace_integration_tests
from .cluster_agent import integration_tests as dca_integration_tests

#We use `basestring` in the code for compat with python2 unicode strings.
#This makes the same code work in python3 as well.
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


@task()
def test(ctx, targets=None, coverage=False, build_include=None, build_exclude=None,
    verbose=False, race=False, profile=False, fail_on_fmt=False,
    rtloader_root=None, python_home_2=None, python_home_3=None, cpus=0, major_version='7',
    python_runtimes='3', timeout=120, arch="x64", cache=True, skip_linters=False,
    go_mod="vendor"):
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

    build_include = get_default_build_tags(process=True, arch=arch) if build_include is None else build_include.split(",")
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
        print("--- Linting filenames:")
        lint_filenames(ctx)
        print("--- Linting licenses:")
        lint_licenses(ctx)

        # Until all packages whitelisted in .golangci.yml are fixed and removed
        # from the 'skip-dirs' list we need to keep using the old functions that
        # lint without build flags (linting some file is better than no linting).
        print("--- Vetting and linting (legacy):")
        vet(ctx, targets=tool_targets, rtloader_root=rtloader_root, build_tags=build_tags, arch=arch)
        fmt(ctx, targets=tool_targets, fail_on_fmt=fail_on_fmt)
        lint(ctx, targets=tool_targets)
        misspell(ctx, targets=tool_targets)
        ineffassign(ctx, targets=tool_targets)

        # for now we only run golangci_lint on Unix as the Windows env need more work
        if sys.platform != 'win32':
            print("--- golangci_lint:")
            golangci_lint(ctx, targets=tool_targets, rtloader_root=rtloader_root, build_tags=build_tags)

    with open(PROFILE_COV, "w") as f_cov:
        f_cov.write("mode: count")

    ldflags, gcflags, env = get_build_flags(ctx, rtloader_root=rtloader_root,
            python_home_2=python_home_2, python_home_3=python_home_3, major_version=major_version,
            python_runtimes='3', arch=arch)

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
    cmd = 'go test {verbose} -mod={go_mod} -vet=off -timeout {timeout}s -tags "{go_build_tags}" -gcflags="{gcflags}" '
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
        print ("\n--- Top 15 packages sorted by run time:")
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
        if any([re.match('team/', l['name']) for l in issue.get('labels', {})]):
            print("Team Assignment: %s" % l['name'])
            return

        print("PR %s requires team assignment" % pr_url)
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
            if any([f['filename'].startswith("releasenotes/notes/") or \
                    f['filename'].startswith("releasenotes-dca/notes/") for f in files]):
                break

            if 'next' in res.links:
                url = res.links['next']['url']
            else:
                print("Error: No releasenote was found for this PR. Please add one using 'reno'"\
                      ", or apply the label 'changelog/no-changelog' to the PR.")
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
                    if any([f['filename'].startswith("releasenotes/notes/") or \
                            f['filename'].startswith("releasenotes-dca/notes/") for f in files]):
                        break

                    if 'next' in res.links:
                        url = res.links['next']['url']
                    else:
                        print("Error: No releasenote was found for this PR. Please add one using 'reno'"\
                              ", or apply the label 'changelog/no-changelog' to the PR.")
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
        if prefix_length + len(file) > max_length:
            print("Error: path {} is too long ({} characters too many)".format(file, prefix_length + len(file) - max_length))
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
    parser = re.compile("^ok\s+github.com\/DataDog\/datadog-agent\/(\S+)\s+([0-9\.]+)s", re.MULTILINE)

    def write(self, txt):
        # Output to stdout
        sys.stdout.write(txt)
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
def make_kitchen_gitlab_yml(ctx):
    """
    Replaces .gitlab-ci.yml with one containing only the steps needed to run kitchen-tests
    """
    with open('.gitlab-ci.yml') as f:
        data = yaml.load(f, Loader=yaml.FullLoader)

    data['stages'] = ['deps_build', 'binary_build', 'package_build', 'testkitchen_deploy', 'testkitchen_testing', 'testkitchen_cleanup']
    for k,v in data.items():
        if isinstance(v, dict) and v.get('stage', None) not in ([None] + data['stages']):
            del data[k]
            continue
        if isinstance(v, dict) and v.get('stage', None) == 'binary_build' and k != 'build_system-probe-arm64' and k != 'build_system-probe-x64' and k != 'build_system-probe_with-bcc-arm64' and k != 'build_system-probe_with-bcc-x64':
            del data[k]
            continue
        if 'except' in v:
            del v['except']
        if 'only' in v:
            del v['only']
        if len(v) == 0:
            del data[k]
            continue

    for k,v in data.items():
        if 'extends' in v:
            extended = v['extends']
            if extended not in data:
                del data[k]
        if 'needs' in v:
            needed = v['needs']
            new_needed = []
            for n in needed:
                if n in data:
                   new_needed.append(n)
            v['needs'] = new_needed

    with open('.gitlab-ci.yml', 'w') as f:
        documents = yaml.dump(data, f, default_style='"')

@task
def check_gitlab_broken_dependencies(ctx):
    """
    Checks that a gitlab job doesn't depend on (need) other jobs that will be excluded from the build,
    since this would make gitlab fail when triggering a pipeline with those jobs excluded.
    """
    with open('.gitlab-ci.yml') as f:
        data = yaml.load(f, Loader=yaml.FullLoader)

    def is_unwanted(job, version):
        e = job.get('except',{})
        return isinstance(e, dict) and '$RELEASE_VERSION_{} == ""'.format(version) in e.get('variables',{})

    for version in [6,7]:
        for k,v in data.items():
            if isinstance(v, dict) and not is_unwanted(v, version) and "needs" in v:
                needed = v['needs']
                for need in needed:
                    if is_unwanted(data[need], version):
                        print("{} needs on {} but it won't be built for A{}".format(k, need, version))
