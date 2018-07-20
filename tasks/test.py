"""
High level testing tasks
"""
from __future__ import print_function

import os
import fnmatch
import re
import operator
import sys

import invoke
from invoke import task
from invoke.exceptions import Exit

from .utils import get_build_flags, get_version, pkg_config_path
from .go import fmt, lint, vet, misspell, ineffassign
from .build_tags import get_default_build_tags, get_build_tags
from .agent import integration_tests as agent_integration_tests
from .dogstatsd import integration_tests as dsd_integration_tests
from .cluster_agent import integration_tests as dca_integration_tests

PROFILE_COV = "profile.cov"

# List of packages to ignore when running tests on Windows platform
WIN_PKG_BLACKLIST = [
    "./pkg\\util\\xc",
    "./pkg\\util\\container",
    "./pkg\\util\\kubernetes",
]

NOTWIN_PKG_BLACKLIST = [
    "./pkg/util/winutil",
    "./pkg/util/winutil/pdhutil",
]
DEFAULT_TOOL_TARGETS = [
    "./pkg",
    "./cmd",
]

DEFAULT_TEST_TARGETS = [
    "./pkg",
]


@task()
def test(ctx, targets=None, coverage=False, build_include=None, build_exclude=None,
    race=False, profile=False, use_embedded_libs=False, fail_on_fmt=False, timeout=120):
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

    build_include = get_default_build_tags() if build_include is None else build_include.split(",")
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    build_tags = get_build_tags(build_include, build_exclude)

    timeout = int(timeout)

    # explicitly run these tasks instead of using pre-tasks so we can
    # pass the `target` param (pre-tasks are invoked without parameters)
    print("--- Linting:")
    lint_filenames(ctx)
    fmt(ctx, targets=tool_targets, fail_on_fmt=fail_on_fmt)
    lint(ctx, targets=tool_targets)
    print("--- Vetting:")
    vet(ctx, targets=tool_targets, use_embedded_libs=use_embedded_libs)
    print("--- Misspelling:")
    misspell(ctx, targets=tool_targets)
    print("--- ineffassigning:")
    ineffassign(ctx, targets=tool_targets)

    with open(PROFILE_COV, "w") as f_cov:
        f_cov.write("mode: count")

    ldflags, gcflags, env = get_build_flags(ctx, use_embedded_libs=use_embedded_libs)

    if profile:
        test_profiler = TestProfiler()
    else:
        test_profiler = None  # Use stdout

    race_opt = ""
    covermode_opt = ""
    if race:
        race_opt = "-race"
    if coverage:
        if race:
            # atomic is quite expensive but it's the only way to run
            # both the coverage and the race detector at the same time
            # without getting false positives from the cover counter
            covermode_opt = "-covermode=atomic"
        else:
            covermode_opt = "-covermode=count"

    if coverage:
        matches = []
        for target in test_targets:
            for root, _, filenames in os.walk(target):
                if fnmatch.filter(filenames, "*.go"):
                    matches.append(root)
    else:
        matches = ["{}/...".format(t) for t in test_targets]
    print("\n--- Running unit tests:")
    for match in matches:
        if sys.platform == 'win32':
            if match in WIN_PKG_BLACKLIST:
                print("Skipping blacklisted directory {}\n".format(match))
                continue
        else:
            if match in NOTWIN_PKG_BLACKLIST:
                print("Skipping blacklisted directory {}\n".format(match))
                continue

        coverprofile = ""
        if coverage:
            profile_tmp = "{}/profile.tmp".format(match)
            coverprofile = "-coverprofile={}".format(profile_tmp)
        cmd = 'go test -timeout {timeout}s -tags "{go_build_tags}" -gcflags="{gcflags}" -ldflags="{ldflags}" '
        cmd += '{race_opt} -short {covermode_opt} {coverprofile} {pkg_folder}'
        args = {
            "go_build_tags": " ".join(build_tags),
            "gcflags": gcflags,
            "ldflags": ldflags,
            "race_opt": race_opt,
            "covermode_opt": covermode_opt,
            "coverprofile": coverprofile,
            "pkg_folder": match,
            "timeout": timeout,
        }
        ctx.run(cmd.format(**args), env=env, out_stream=test_profiler)

        if coverage:
            if os.path.exists(profile_tmp):
                ctx.run("cat {} | tail -n +2 >> {}".format(profile_tmp, PROFILE_COV))
                os.remove(profile_tmp)

    if coverage:
        print("\n--- Test coverage:")
        ctx.run("go tool cover -func {}".format(PROFILE_COV))

    if profile:
        print ("\n--- Top 15 packages sorted by run time:")
        test_profiler.print_sorted(15)


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

        # first check 'noreno' label
        res = requests.get("https://api.github.com/repos/DataDog/datadog-agent/issues/{}".format(pr_id))
        issue = res.json()
        if any([l['name'] == 'noreno' for l in issue.get('labels', {})]):
            print("'noreno' label found on the PR: skipping linting")
            return

        # Then check that at least one note was touched by the PR
        url = "https://api.github.com/repos/DataDog/datadog-agent/pulls/{}/files".format(pr_id)
        # traverse paginated github response
        while True:
            res = requests.get(url)
            files = res.json()
            for notesdir in ["releasenotes/notes/", "releasenotes/cluster-agent/notes/"]:
                if any([f['filename'].startswith(notesdir) for f in files]):
                    break

            if 'next' in res.links:
                url = res.links['next']['url']
            else:
                print("Error: No releasenote was found for this PR. Please add one using 'reno'.")
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
                    for notesdir in ["releasenotes/notes/", "releasenotes/cluster-agent/notes/"]:
                        if any([f['filename'].startswith(notesdir) for f in files]):
                            break

                    if 'next' in res.links:
                        url = res.links['next']['url']
                    else:
                        print("Error: No releasenote was found for this PR. Please add one using 'reno'.")
                        raise Exit(code=1)

    ctx.run("reno lint")


@task
def lint_filenames(ctx):
    """
    Scan files to ensure there are no filenames containing illegal characters
    """
    if sys.platform == 'win32':
        print("Running on windows, no need to check filenames for illegal characters")
    else:
        print("Checking filenames for illegal characters")
        forbidden_chars = '<>:"\\|?*'
        files = ctx.run("git ls-files -z", hide=True).stdout.split("\0")
        failure = False
        for file in files:
            if any(char in file for char in forbidden_chars):
                print("Error: Found illegal character in path {}".format(file))
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


@task
def version(ctx, url_safe=False, git_sha_length=7):
    """
    Get the agent version.
    url_safe: get the version that is able to be addressed as a url
    git_sha_length: different versions of git have a different short sha length,
                    use this to explicitly set the version
                    (the windows builder and the default ubuntu version have such an incompatibility)
    """
    print(get_version(ctx, include_git=True, url_safe=url_safe, git_sha_length=git_sha_length))


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
