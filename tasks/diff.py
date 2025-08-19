"""
Diffing tasks
"""

import datetime
import json
import os
import tempfile
from datetime import timedelta

from invoke import task
from invoke.exceptions import Exit

from tasks.build_tags import get_default_build_tags
from tasks.flavor import AgentFlavor
from tasks.go import GOARCH_MAPPING, GOOS_MAPPING
from tasks.go import version as dot_go_version
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.datadog_api import create_count, send_metrics
from tasks.libs.common.git import check_uncommitted_changes, get_commit_sha, get_current_branch
from tasks.libs.common.worktree import agent_context
from tasks.release import _get_release_json_value

BINARIES: dict[str, dict] = {
    "agent": {
        "entrypoint": "cmd/agent",
        "platforms": ["linux/x64", "linux/arm64", "win32/x64", "darwin/x64", "darwin/arm64"],
    },
    "iot-agent": {
        "build": "agent",
        "entrypoint": "cmd/iot-agent",
        "flavor": AgentFlavor.iot,
        "platforms": ["linux/x64", "linux/arm64"],
    },
    "heroku-agent": {
        "build": "agent",
        "entrypoint": "cmd/agent",
        "flavor": AgentFlavor.heroku,
        "platforms": ["linux/x64"],
    },
    "cluster-agent": {"entrypoint": "cmd/cluster-agent", "platforms": ["linux/x64", "linux/arm64"]},
    "cluster-agent-cloudfoundry": {
        "entrypoint": "cmd/cluster-agent-cloudfoundry",
        "platforms": ["linux/x64", "linux/arm64"],
    },
    "dogstatsd": {"entrypoint": "cmd/dogstatsd", "platforms": ["linux/x64", "linux/arm64"]},
    "process-agent": {
        "entrypoint": "cmd/process-agent",
        "platforms": ["linux/x64", "linux/arm64", "win32/x64", "darwin/x64", "darwin/arm64"],
    },
    "heroku-process-agent": {
        "build": "process-agent",
        "entrypoint": "cmd/process-agent",
        "flavor": AgentFlavor.heroku,
        "platforms": ["linux/x64"],
    },
    "security-agent": {
        "entrypoint": "cmd/security-agent",
        "platforms": ["linux/x64", "linux/arm64", "win32/x64"],
    },
    "sbomgen": {
        "entrypoint": "cmd/sbomgen",
        "platforms": ["linux/x64", "linux/arm64"],
    },
    "serverless": {"entrypoint": "cmd/serverless", "platforms": ["linux/x64", "linux/arm64"]},
    "system-probe": {"entrypoint": "cmd/system-probe", "platforms": ["linux/x64", "linux/arm64", "win32/x64"]},
    "cws-instrumentation": {
        "entrypoint": "cmd/cws-instrumentation",
        "platforms": ["linux/x64", "linux/arm64"],
    },
    "trace-agent": {
        "entrypoint": "cmd/trace-agent",
        "platforms": ["linux/x64", "linux/arm64", "win32/x64", "darwin/x64", "darwin/arm64"],
    },
    "heroku-trace-agent": {
        "build": "trace-agent",
        "entrypoint": "cmd/trace-agent",
        "flavor": AgentFlavor.heroku,
        "platforms": ["linux/x64"],
    },
    "otel-agent": {
        "entrypoint": "cmd/otel-agent",
        "platforms": ["linux/x64", "linux/arm64"],
    },
}

METRIC_GO_DEPS_DIFF = "datadog.agent.go_dependencies.diff"


@task
def go_deps(
    ctx,
    baseline_ref=None,
    report_file=None,
    report_metrics: bool = False,
    git_ref: str | None = None,
):
    if check_uncommitted_changes(ctx):
        raise Exit(
            color_message(
                "There are uncomitted changes in your repository. Please commit or stash them before trying again.",
                "red",
            ),
            code=1,
        )

    if report_metrics and not os.environ.get("DD_API_KEY"):
        raise Exit(
            code=1,
            message=color_message(
                "DD_API_KEY environment variable not set, cannot send pipeline metrics to the backend", "red"
            ),
        )

    timestamp = int(datetime.datetime.now(datetime.UTC).timestamp())

    current_branch = get_current_branch(ctx)
    commit_sha = os.getenv("CI_COMMIT_SHA")
    if commit_sha is None:
        commit_sha = get_commit_sha(ctx)

    if not baseline_ref:
        base_branch = _get_release_json_value("base_branch")
        baseline_ref = ctx.run(f"git merge-base {commit_sha} origin/{base_branch}", hide=True).stdout.strip()

    diffs = {}
    dep_cmd = "go list -f '{{ range .Deps }}{{ printf \"%s\\n\" . }}{{end}}'"

    with tempfile.TemporaryDirectory() as tmpdir:
        try:
            # generate list of imports for each target+branch combo
            branches = {"current": None, "main": baseline_ref}
            for branch_name, branch_ref in branches.items():
                if branch_ref:
                    ctx.run(f"git checkout -q {branch_ref}")

                for binary, details in BINARIES.items():
                    with ctx.cd(details.get("entrypoint")):
                        for combo in details["platforms"]:
                            platform, arch = combo.split("/")
                            goos, goarch = GOOS_MAPPING.get(platform), GOARCH_MAPPING.get(arch)
                            target = f"{binary}-{goos}-{goarch}"

                            depsfile = os.path.join(tmpdir, f"{target}-{branch_name}")
                            flavor = details.get("flavor", AgentFlavor.base)
                            build = details.get("build", binary)
                            build_tags = get_default_build_tags(build=build, platform=platform, flavor=flavor)
                            # need to explicitly enable CGO to also include CGO-only deps when checking different platforms
                            env = {
                                "GOOS": goos,
                                "GOARCH": goarch,
                                "CGO_ENABLED": "1",
                                "GOTOOLCHAIN": f"go{dot_go_version(ctx)}",
                            }
                            ctx.run(f"{dep_cmd} -tags \"{' '.join(build_tags)}\" > {depsfile}", env=env)
        finally:
            ctx.run(f"git checkout -q {current_branch}")

        # compute diffs for each target
        for binary, details in BINARIES.items():
            for combo in details["platforms"]:
                platform, arch = combo.split("/")
                goos, goarch = GOOS_MAPPING.get(platform), GOARCH_MAPPING.get(arch)
                target = f"{binary}-{goos}-{goarch}"

                prdeps = os.path.join(tmpdir, f"{target}-current")
                maindeps = os.path.join(tmpdir, f"{target}-main")
                # filter out internal packages to avoid noise
                res = ctx.run(
                    f"diff -u0 {maindeps} {prdeps} | grep -v '^@@' | grep -v '^[+-][+-]' | grep -v -E '(^[+-]|/)internal/'",
                    hide=True,
                    warn=True,
                )
                if len(res.stdout) > 0:
                    diffs[target] = res.stdout.strip()

        # output, also to file if requested
        if len(diffs) > 0:
            pr_comment = [
                f"Baseline: {baseline_ref}",
                f"Comparison: {commit_sha}\n",
                "<table><thead><tr><th>binary</th><th>os</th><th>arch</th><th>change</th></tr></thead><tbody>",
            ]
            for binary, details in BINARIES.items():
                for combo in details["platforms"]:
                    flavor = details.get("flavor", AgentFlavor.base)
                    build = details.get("build", binary)
                    platform, arch = combo.split("/")
                    goos, goarch = GOOS_MAPPING.get(platform), GOARCH_MAPPING.get(arch)
                    target = f"{binary}-{goos}-{goarch}"
                    prettytarget = f"{binary} {goos}/{goarch}"

                    if target in diffs:
                        targetdiffs = diffs[target]
                        add, remove = patch_summary(targetdiffs)

                        if report_metrics:
                            tags = [
                                f"build:{build}",
                                f"flavor:{flavor.name}",
                                f"os:{goos}",
                                f"arch:{goarch}",
                                f"git_sha:{commit_sha}",
                                f"git_ref:{git_ref}",
                            ]

                            if git_ref:
                                tags.append(f"git_ref:{git_ref}")

                            dependency_diff = create_count(METRIC_GO_DEPS_DIFF, timestamp, (add - remove), tags)
                            send_metrics([dependency_diff])

                        color_add = color_message(f"+{add}", "green")
                        color_remove = color_message(f"-{remove}", "red")
                        print(f"== {prettytarget} {color_add}, {color_remove} ==")
                        print(f"{color_patch(targetdiffs)}\n")

                        summary = f"<summary>+{add}, -{remove}</summary>"
                        diff_block = f"<pre lang='diff'>\n{targetdiffs}\n</pre>"
                        pr_comment.append(
                            f"<tr><td>{binary}</td><td>{goos}</td><td>{goarch}</td><td><details>{summary}\n{diff_block}</details></td></tr>"
                        )
                    else:
                        print(f"== {prettytarget} ==\nno changes\n")

            pr_comment.append("</tbody></table>")
            if report_file:
                with open(report_file, 'w') as f:
                    f.write("\n".join(pr_comment))
        else:
            print("no changes for all binaries")
            if report_file:
                # touch file
                open(report_file, 'w').close()


def color_patch(diff):
    out = []
    for line in diff.splitlines():
        if line.startswith("+"):
            out.append(color_message(line, 'green'))
        elif line.startswith("-"):
            out.append(color_message(line, 'red'))
        else:
            out.append(line)
    return "\n".join(out)


def patch_summary(diff):
    add_count, remove_count = 0, 0
    for line in diff.splitlines():
        if line.startswith("+"):
            add_count += 1
        elif line.startswith("-"):
            remove_count += 1
    return add_count, remove_count


def _list_tasks_rec(collection, prefix='', res=None):
    res = res or {}

    if isinstance(collection, dict):
        newpref = prefix + collection['name']

        for task in collection['tasks']:
            res[newpref + '.' + task['name']] = task['help']

        for subtask in collection['collections']:
            _list_tasks_rec(subtask, newpref + '.', res)

    return res


def _list_invoke_tasks(ctx) -> dict[str, str]:
    """Returns a dictionary of invoke tasks and their descriptions."""

    tasks = json.loads(ctx.run('dda inv -- --list -F json', hide=True).stdout)

    # Remove 'tasks.' prefix
    return {name.removeprefix(tasks['name'] + '.'): desc for name, desc in _list_tasks_rec(tasks).items()}


@task
def invoke_tasks(ctx, diff_date: str | None = None):
    """Shows the added / removed invoke tasks since diff_date with their description.

    Args:
        diff_date: The date to compare the tasks to ('YYYY-MM-DD' format). Will be the last 30 days if not provided.
    """

    if not diff_date:
        diff_date = (datetime.datetime.now() - timedelta(days=30)).strftime('%Y-%m-%d')
    else:
        try:
            datetime.datetime.strptime(diff_date, '%Y-%m-%d')
        except ValueError as e:
            raise Exit('Invalid date format. Please use the format "YYYY-MM-DD".') from e

    old_commit = ctx.run(f"git rev-list -n 1 --before='{diff_date} 23:59' HEAD", hide=True).stdout.strip()
    assert old_commit, f"No commit found before {diff_date}"

    with agent_context(ctx, commit=old_commit):
        old_tasks = _list_invoke_tasks(ctx)
    current_tasks = _list_invoke_tasks(ctx)

    all_tasks = set(old_tasks.keys()).union(current_tasks.keys())
    removed_tasks = {task for task in all_tasks if task not in current_tasks}
    added_tasks = {task for task in all_tasks if task not in old_tasks}

    if removed_tasks:
        print(f'* {color_message("Removed tasks", Color.BOLD)}:')
        print('\n'.join(sorted(f'- {name}' for name in removed_tasks)))
    else:
        print('No task removed')

    if added_tasks:
        print(f'\n* {color_message("Added tasks", Color.BOLD)}:')
        for name, description in sorted((name, current_tasks[name]) for name in added_tasks):
            line = '+ ' + name
            if description:
                line += ': ' + description
            print(line)
    else:
        print('No task added')
