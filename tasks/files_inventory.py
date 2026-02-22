import glob
import os
import sys
import tempfile

import yaml
from invoke import task

from tasks.git import get_ancestor
from tasks.github_tasks import pr_commenter
from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.color import color_message
from tasks.static_quality_gates.experimental_gates import (
    FileChange,
    FileInfo,
)
from tasks.static_quality_gates.experimental_gates import (
    measure_package_local as _measure_package_local,
)
from tasks.static_quality_gates.gates import byte_to_string


def _compare_inventories(
    previous_inventory: list[FileInfo], current_inventory: list[FileInfo]
) -> tuple[list[FileInfo], list[FileInfo], dict[str, FileChange]]:
    removed_files = []
    added_files = []
    changed_files = {}

    previous_files = {info.relative_path: info for info in previous_inventory}
    current_files = {info.relative_path: info for info in current_inventory}
    for path, previous in previous_files.items():
        if path not in current_files:
            removed_files.append(previous)
            continue
        current = current_files[path]
        size_change = previous.size_bytes - current.size_bytes
        changed_flags = FileChange.Flags(0)
        size_percent = None
        if size_change:
            size_percent = (current.size_bytes - previous.size_bytes) / previous.size_bytes * 100
            if size_percent > 10:
                changed_flags |= FileChange.Flags.Size
        if current.chmod != previous.chmod:
            changed_flags |= FileChange.Flags.Permissions
        if current.owner != previous.owner:
            changed_flags |= FileChange.Flags.Owner
        if current.group != previous.group:
            changed_flags |= FileChange.Flags.Group

        if changed_flags:
            changed_files[path] = FileChange(
                flags=changed_flags, previous=previous, current=current, size_percent=size_percent
            )
        # Remove entries that were present in both parent & current so that when we're done
        # the current list only contains new files
        del current_files[path]
    added_files = list(current_files.values())
    return added_files, removed_files, changed_files


@task
def compare_inventories(_, parent_inventory_report, current_inventory_report):
    with open(parent_inventory_report) as f:
        parent_inventory = yaml.safe_load(f)
    with open(current_inventory_report) as f:
        current_inventory = yaml.safe_load(f)
    parent_file_inventory = [FileInfo(**values) for values in parent_inventory['file_inventory']]
    current_file_inventory = [FileInfo(**values) for values in current_inventory['file_inventory']]
    added, removed, changed = _compare_inventories(parent_file_inventory, current_file_inventory)
    if len(added) == 0 and len(removed) == 0 and len(changed) == 0:
        print(color_message('✅ No change detected', 'green'))
        body = "No change detected"
    else:
        _print_inventory_diff(added, removed, changed)
        body = _inventory_changes_to_comment(added, removed, changed)
    return body


def _display_change_summary(change: FileChange):
    print(color_message(f'Summary of changes to {change.current.relative_path}', "orange"))
    if change.flags & FileChange.Flags.Permissions:
        print(f'    Permission changed: {oct(change.previous.chmod)} -> {oct(change.current.chmod)}')
    if change.flags & FileChange.Flags.Size:
        color = "red" if change.size_percent > 0 else "green"
        change_str = color_message(f'{change.size_percent:.2f}%', color)
        print(
            f'    Size changed by {change_str} ({byte_to_string(change.previous.size_bytes)} -> {byte_to_string(change.current.size_bytes)})'
        )
    if change.flags & (FileChange.Flags.Owner | FileChange.Flags.Group):
        print(
            f'    File owner/group changed: {change.previous.owner}:{change.previous.group} -> {change.current.owner}:{change.current.group}'
        )


def _print_inventory_diff(added, removed, changed):
    if len(added) > 0:
        print(color_message('➕ New files added:', "orange"))
        for f in added:
            print(color_message(f'    - {f.relative_path} ({byte_to_string(f.size_bytes)})', "orange"))
    if len(removed) > 0:
        print(color_message('❌ Old files removed:', "orange"))
        for f in removed:
            print(color_message(f'    - {f.relative_path} ({byte_to_string(f.size_bytes)})', "orange"))
    if len(changed) > 0:
        print(color_message('⚠️ Some files modifications need review:', "orange"))
        for change in changed.values():
            _display_change_summary(change)


def _inventory_changes_to_comment(added, removed, changed):
    body = "## Detected file changes:\n"
    if len(added):
        body += f"<details><summary>\n\n### {len(added)} Added files:\n\n</summary>\n\n"
        for f in added:
            body += f"* `{f.relative_path}` ({byte_to_string(f.size_bytes)})\n"
        body += "</details>"
    if len(removed):
        body += f"<details><summary>\n\n### {len(removed)} Removed files:\n\n</summary>\n\n"
        for f in removed:
            body += f"* `{f.relative_path}` ({byte_to_string(f.size_bytes)})\n"
        body += "</details>"
    if len(changed):
        body += f"<details><summary>\n\n### {len(changed)} Changed files:\n\n</summary>\n\n"
        for path, change in changed.items():
            change_str = f"* `{path}`:\n"
            if change.flags & FileChange.Flags.Permissions:
                change_str += f"  * Permission changed: {oct(change.previous.chmod)} -> {oct(change.current.chmod)}\n"
            if change.flags & FileChange.Flags.Size:
                change_str += f'  * Size changed: {change.size_percent:+.2f}% ({byte_to_string(change.previous.size_bytes)} -> {byte_to_string(change.current.size_bytes)})\n'
            if change.flags & (FileChange.Flags.Owner | FileChange.Flags.Group):
                change_str += f'  * File owner/group changed: {change.previous.owner}:{change.previous.group} -> {change.current.owner}:{change.current.group}\n'
            body += change_str
        body += "</details>"
    return body


def _get_parent_report(ctx, parent_sha, gate_name: str, output: str):
    """
    Fetch the quality gates report from the base commit.
    """
    aws_cmd = "aws.exe" if sys.platform == 'win32' else "aws"
    s3_url = f"s3://dd-ci-artefacts-build-stable/datadog-agent/static_quality_gates/GATE_REPORTS/{parent_sha}/{gate_name}_size_report_{parent_sha[:8]}.yml"
    ctx.run(f"{aws_cmd} s3 cp --only-show-errors {s3_url} {output}", warn=True)


def _filter_files(path: str) -> bool:
    pipeline_id = os.environ.get('CI_PIPELINE_ID')
    # Avoid files such as opt/datadog-packages/run/datadog-agent/7.76.0-devel.git.716.8d5ec09.pipeline.91823583
    if pipeline_id and f'pipeline.{pipeline_id}' in path:
        return False
    return path not in [
        # Installer symlinks which we don't want to include
        'opt/datadog-packages/datadog-agent/experiment',
        'opt/datadog-packages/datadog-agent/stable',
    ]


@task
def check(ctx, branch_name, reports_folder):
    package_types = ['deb', 'rpm']
    parent_sha = get_ancestor(ctx, branch_name)
    pr_comment = f"File checks results against ancestor [{parent_sha[:8]}](https://github.com/DataDog/datadog-agent/commit/{parent_sha}):\n\n"

    for package_type in package_types:
        for artifact in glob.glob(f'{reports_folder}/datadog-agent*.{package_type}'):
            # deb pattern is $packagename-$version
            # rpm pattern is $packagename_$version
            if '-dbg-' in artifact or '-dbg_' in artifact:
                continue
            pr_comment += f'### Results for {os.path.basename(artifact)}:\n'
            arch = "amd64"
            if 'aarch64' in artifact or 'arm64' in artifact:
                arch = "arm64"
            gate_short_name = f'agent_{package_type}_{arch}'
            report_filename = f'{gate_short_name}_{arch}_size_report_{os.environ["CI_COMMIT_SHORT_SHA"]}.yml'
            _measure_package_local(
                ctx=ctx,
                package_path=artifact,
                gate_name=f'static_quality_gate_{gate_short_name}',
                output_path=report_filename,
                build_job_name=os.environ['CI_JOB_NAME'],
                debug=True,
                filter=_filter_files,
            )
            # Upload the report to S3
            bucket_base_path = "s3://dd-ci-artefacts-build-stable/datadog-agent/static_quality_gates/GATE_REPORTS/"
            ctx.run(
                f'aws s3 cp --only-show-errors --region us-east-1 --sse AES256 {report_filename} {bucket_base_path}/{report_filename}'
            )

            parent_report_file = tempfile.NamedTemporaryFile()
            _get_parent_report(ctx, parent_sha, gate_short_name, parent_report_file.file.name)
            body = compare_inventories(ctx, parent_report_file.file.name, report_filename)
            pr_comment += body

    github = GithubAPI()
    prs = list(github.get_pr_for_branch(branch_name))
    if len(prs):
        pr_commenter(ctx, title='Files inventory check summary', body=pr_comment, pr=prs[0])
    else:
        print(color_message(f"Can't fetch PR assciated with branch {branch_name}", "red"))
