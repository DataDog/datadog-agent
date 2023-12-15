"""
Diffing tasks
"""

import os
import tempfile

from invoke import task
from invoke.exceptions import Exit

from .build_tags import get_default_build_tags
from .flavor import AgentFlavor
from .go import GOARCH_MAPPING, GOOS_MAPPING
from .libs.common.color import color_message
from .release import _get_release_json_value
from .utils import check_uncommitted_changes


@task
def go_deps(ctx, baseline_ref=None, report_file=None):
    if check_uncommitted_changes(ctx):
        raise Exit(
            color_message(
                "There are uncomitted changes in your repository. Please commit or stash them before trying again.",
                "red",
            ),
            code=1,
        )
    current_branch = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()
    commit_sha = os.getenv("CI_COMMIT_SHA")
    if commit_sha is None:
        commit_sha = ctx.run("git rev-parse HEAD", hide=True).stdout.strip()

    if not baseline_ref:
        base_branch = _get_release_json_value("base_branch")
        baseline_ref = ctx.run(f"git merge-base {commit_sha} origin/{base_branch}", hide=True).stdout.strip()

    # platforms are the agent task recognized os/platform and arch values, not Go-specific values
    binaries = {
        "agent": {
            "entrypoint": "cmd/agent",
            "platforms": ["linux/x64", "linux/arm64", "win32/x64", "win32/x86", "darwin/x64", "darwin/arm64"],
        },
        "iot-agent": {
            "build": "agent",
            "entrypoint": "cmd/agent",
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
            "platforms": ["linux/x64", "linux/arm64"],
        },
        "serverless": {"entrypoint": "cmd/serverless", "platforms": ["linux/x64", "linux/arm64"]},
        "system-probe": {"entrypoint": "cmd/system-probe", "platforms": ["linux/x64", "linux/arm64", "win32/x64"]},
        "trace-agent": {
            "entrypoint": "cmd/trace-agent",
            "platforms": ["linux/x64", "linux/arm64", "win32/x64", "win32/x86", "darwin/x64", "darwin/arm64"],
        },
        "heroku-trace-agent": {
            "build": "trace-agent",
            "entrypoint": "cmd/trace-agent",
            "flavor": AgentFlavor.heroku,
            "platforms": ["linux/x64"],
        },
    }
    diffs = {}
    dep_cmd = "go list -f '{{ range .Deps }}{{ printf \"%s\\n\" . }}{{end}}'"

    with tempfile.TemporaryDirectory() as tmpdir:
        try:
            # generate list of imports for each target+branch combo
            branches = {"current": None, "main": baseline_ref}
            for branch_name, branch_ref in branches.items():
                if branch_ref:
                    ctx.run(f"git checkout -q {branch_ref}")

                for binary, details in binaries.items():
                    with ctx.cd(details.get("entrypoint")):
                        for combo in details.get("platforms"):
                            platform, arch = combo.split("/")
                            goos, goarch = GOOS_MAPPING.get(platform), GOARCH_MAPPING.get(arch)
                            target = f"{binary}-{goos}-{goarch}"

                            depsfile = os.path.join(tmpdir, f"{target}-{branch_name}")
                            flavor = details.get("flavor", AgentFlavor.base)
                            build = details.get("build", binary)
                            build_tags = get_default_build_tags(
                                build=build, arch=arch, platform=platform, flavor=flavor
                            )
                            env = {"GOOS": goos, "GOARCH": goarch}
                            ctx.run(f"{dep_cmd} -tags \"{' '.join(build_tags)}\" > {depsfile}", env=env)
        finally:
            ctx.run(f"git checkout -q {current_branch}")

        # compute diffs for each target
        for binary, details in binaries.items():
            for combo in details.get("platforms"):
                platform, arch = combo.split("/")
                goos, goarch = GOOS_MAPPING.get(platform), GOARCH_MAPPING.get(arch)
                target = f"{binary}-{goos}-{goarch}"

                prdeps = os.path.join(tmpdir, f"{target}-current")
                maindeps = os.path.join(tmpdir, f"{target}-main")
                res = ctx.run(
                    f"diff -u0 {maindeps} {prdeps} | grep -v '^@@' | grep -v '^[+-][+-]'", hide=True, warn=True
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
            for binary, details in binaries.items():
                for combo in details.get("platforms"):
                    platform, arch = combo.split("/")
                    goos, goarch = GOOS_MAPPING.get(platform), GOARCH_MAPPING.get(arch)
                    target = f"{binary}-{goos}-{goarch}"
                    prettytarget = f"{binary} {goos}/{goarch}"

                    if target in diffs:
                        targetdiffs = diffs[target]
                        add, remove = patch_summary(targetdiffs)
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
