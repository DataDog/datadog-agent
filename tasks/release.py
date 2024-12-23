"""Release helper tasks

Notes about Agent6:
    Release tasks should be run from the main branch.
    To make a task compatible with agent 6, it is possible to use agent_context such that
    the task will be run in the agent6 branch.
"""

import json
import os
import re
import sys
import tempfile
import time
from collections import defaultdict
from datetime import date
from time import sleep

from gitlab import GitlabError
from invoke import task
from invoke.exceptions import Exit

from tasks.libs.ciproviders.github_api import GithubAPI, create_release_pr
from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.constants import (
    GITHUB_REPO_NAME,
)
from tasks.libs.common.git import (
    check_base_branch,
    check_clean_branch_state,
    clone,
    get_default_branch,
    get_last_commit,
    get_last_release_tag,
    is_agent6,
    set_git_config,
    try_git_command,
)
from tasks.libs.common.gomodules import get_default_modules
from tasks.libs.common.user_interactions import yes_no_question
from tasks.libs.common.worktree import agent_context
from tasks.libs.pipeline.notifications import (
    DEFAULT_JIRA_PROJECT,
    DEFAULT_SLACK_CHANNEL,
    load_and_validate,
    warn_new_commits,
)
from tasks.libs.releasing.documentation import (
    create_release_page,
    get_release_page_info,
    list_not_closed_qa_cards,
    release_manager,
)
from tasks.libs.releasing.json import (
    DEFAULT_BRANCHES,
    DEFAULT_BRANCHES_AGENT6,
    UNFREEZE_REPOS,
    _get_release_json_value,
    _save_release_json,
    generate_repo_data,
    load_release_json,
    set_new_release_branch,
    update_release_json,
)
from tasks.libs.releasing.notes import _add_dca_prelude, _add_prelude
from tasks.libs.releasing.version import (
    MINOR_RC_VERSION_RE,
    RC_VERSION_RE,
    VERSION_RE,
    _create_version_from_match,
    current_version,
    deduce_version,
    get_version_major,
    next_final_version,
    next_rc_version,
)
from tasks.pipeline import edit_schedule, run
from tasks.release_metrics.metrics import get_prs_metrics, get_release_lead_time

GITLAB_FILES_TO_UPDATE = [
    ".gitlab-ci.yml",
    ".gitlab/notify/notify.yml",
]

BACKPORT_LABEL_COLOR = "5319e7"


@task
def list_major_change(_, milestone):
    """List all PR labeled "major_changed" for this release."""

    gh = GithubAPI()
    pull_requests = gh.get_pulls(milestone=milestone, labels=['major_change'])
    if pull_requests is None:
        return
    if len(pull_requests) == 0:
        print(f"no major change for {milestone}")
        return

    for pr in pull_requests:
        print(f"#{pr.number}: {pr.title} ({pr.html_url})")


@task
def update_modules(ctx, release_branch=None, version=None, trust=False):
    """Update internal dependencies between the different Agent modules.

    Args:
        verify: Checks for correctness on the Agent Version (on by default).

    Examples:
        $ inv -e release.update-modules 7.27.x
    """

    assert release_branch or version

    agent_version = version or deduce_version(ctx, release_branch, trust=trust)

    with agent_context(ctx, release_branch, skip_checkout=release_branch is None):
        modules = get_default_modules()
        for module in modules.values():
            for dependency in module.dependencies:
                dependency_mod = modules[dependency]
                ctx.run(f"go mod edit -require={dependency_mod.dependency_path(agent_version)} {module.go_mod_path()}")


def __get_force_option(force: bool) -> str:
    """Get flag to pass to git tag depending on if we want forcing or not."""

    force_option = ""
    if force:
        print(color_message("--force option enabled. This will allow the task to overwrite existing tags.", "orange"))
        result = yes_no_question("Please confirm the use of the --force option.", color="orange", default=False)
        if result:
            print("Continuing with the --force option.")
            force_option = " --force"
        else:
            print("Continuing without the --force option.")
    return force_option


def __tag_single_module(ctx, module, agent_version, commit, force_option, devel):
    """Tag a given module."""
    tags = []
    for tag in module.tag(agent_version):
        if devel:
            tag += "-devel"

        ok = try_git_command(
            ctx,
            f"git tag -m {tag} {tag} {commit}{force_option}",
        )
        if not ok:
            message = f"Could not create tag {tag}. Please rerun the task to retry creating the tags (you may need the --force option)"
            raise Exit(color_message(message, "red"), code=1)
        print(f"Created tag {tag}")
        tags.append(tag)
    return tags


@task
def tag_modules(
    ctx, release_branch=None, commit="HEAD", push=True, force=False, devel=False, version=None, trust=False
):
    """Create tags for Go nested modules for a given Datadog Agent version.

    Args:
        commit: Will tag `commit` with the tags (default HEAD).
        verify: Checks for correctness on the Agent version (on by default).
        push: Will push the tags to the origin remote (on by default).
        force: Will allow the task to overwrite existing tags. Needed to move existing tags (off by default).
        devel: Will create -devel tags (used after creation of the release branch).

    Examples:
        $ inv -e release.tag-modules 7.27.x                 # Create tags and push them to origin
        $ inv -e release.tag-modules 7.27.x --no-push       # Create tags locally; don't push them
        $ inv -e release.tag-modules 7.29.x --force         # Create tags (overwriting existing tags with the same name), force-push them to origin
    """

    assert release_branch or version

    agent_version = version or deduce_version(ctx, release_branch, trust=trust)

    tags = []
    with agent_context(ctx, release_branch, skip_checkout=release_branch is None):
        force_option = __get_force_option(force)
        for module in get_default_modules().values():
            # Skip main module; this is tagged at tag_version via __tag_single_module.
            if module.should_tag and module.path != ".":
                new_tags = __tag_single_module(ctx, module, agent_version, commit, force_option, devel)
                tags.extend(new_tags)

        if push:
            tags_list = ' '.join(tags)
            ctx.run(f"git push origin {tags_list}{force_option}")
            print(f"Pushed tag {tags_list}")
        print(f"Created module tags for version {agent_version}")


@task
def tag_version(
    ctx, release_branch=None, commit="HEAD", push=True, force=False, devel=False, version=None, trust=False
):
    """Create tags for a given Datadog Agent version.

    Args:
        commit: Will tag `commit` with the tags (default HEAD)
        verify: Checks for correctness on the Agent version (on by default).
        push: Will push the tags to the origin remote (on by default).
        force: Will allow the task to overwrite existing tags. Needed to move existing tags (off by default).
        devel: Will create -devel tags (used after creation of the release branch)

    Examples:
        $ inv -e release.tag-version 7.27.x            # Create tags and push them to origin
        $ inv -e release.tag-version 7.27.x --no-push  # Create tags locally; don't push them
        $ inv -e release.tag-version 7.29.x --force    # Create tags (overwriting existing tags with the same name), force-push them to origin
    """

    assert release_branch or version

    agent_version = version or deduce_version(ctx, release_branch, trust=trust)

    # Always tag the main module
    force_option = __get_force_option(force)
    with agent_context(ctx, release_branch, skip_checkout=release_branch is None):
        tags = __tag_single_module(ctx, get_default_modules()["."], agent_version, commit, force_option, devel)

    if push:
        tags_list = ' '.join(tags)
        ctx.run(f"git push origin {tags_list}{force_option}")
        print(f"Pushed tag {tags_list}")
    print(f"Created tags for version {agent_version}")


@task
def tag_devel(ctx, release_branch, commit="HEAD", push=True, force=False):
    tag_version(ctx, release_branch, commit, push, force, devel=True)
    tag_modules(ctx, release_branch, commit, push, force, devel=True, trust=True)


@task
def finish(ctx, release_branch, upstream="origin"):
    """Updates the release entry in the release.json file for the new version.

    Updates internal module dependencies with the new version.
    """

    # Step 1: Preparation

    major_version = get_version_major(release_branch)
    print(f"Finishing release for major version {major_version}")

    with agent_context(ctx, release_branch):
        # NOTE: the release process assumes that at least one RC
        # was built before release.finish is used. It doesn't support
        # doing final version -> final version updates (eg. 7.32.0 -> 7.32.1
        # without doing at least 7.32.1-rc.1), as next_final_version won't
        # find the correct new version.
        # To support this, we'd have to support a --patch-version param in
        # release.finish
        new_version = next_final_version(ctx, major_version, False)
        if not yes_no_question(
            f'Do you want to finish the release with version {new_version}?', color="bold", default=False
        ):
            raise Exit(color_message("Aborting.", "red"), code=1)
        update_release_json(new_version, new_version)

        # Step 2: Update internal module dependencies

        update_modules(ctx, version=str(new_version))

        # Step 3: Branch out, commit change, push branch

        final_branch = f"{new_version}-final"

        print(color_message(f"Branching out to {final_branch}", "bold"))
        ctx.run(f"git checkout -b {final_branch}")

        print(color_message("Committing release.json and Go modules updates", "bold"))
        print(
            color_message(
                "If commit signing is enabled, you will have to make sure the commit gets properly signed.", "bold"
            )
        )
        ctx.run("git add release.json")
        ctx.run("git ls-files . | grep 'go.mod$' | xargs git add")

        commit_message = f"'Final updates for release.json and Go modules for {new_version} release'"

        ok = try_git_command(ctx, f"git commit -m {commit_message}")
        if not ok:
            raise Exit(
                color_message(
                    f"Could not create commit. Please commit manually with:\ngit commit -m {commit_message}\n, push the {final_branch} branch and then open a PR against {final_branch}.",
                    "red",
                ),
                code=1,
            )

        # Step 4: Add release changelog preludes
        print(color_message("Adding Agent release changelog prelude", "bold"))
        _add_prelude(ctx, str(new_version))

        print(color_message("Adding DCA release changelog prelude", "bold"))
        _add_dca_prelude(ctx, str(new_version))

        ok = try_git_command(ctx, f"git commit -m 'Add preludes for {new_version} release'")
        if not ok:
            raise Exit(
                color_message(
                    f"Could not create commit. Please commit manually, push the {final_branch} branch and then open a PR against {final_branch}.",
                    "red",
                ),
                code=1,
            )

        # Step 5: Push branch and create PR
        print(color_message("Pushing new branch to the upstream repository", "bold"))
        res = ctx.run(f"git push --set-upstream {upstream} {final_branch}", warn=True)
        if res.exited is None or res.exited > 0:
            raise Exit(
                color_message(
                    f"Could not push branch {final_branch} to the upstream '{upstream}'. Please push it manually and then open a PR against {final_branch}.",
                    "red",
                ),
                code=1,
            )

        create_release_pr(
            f"Final updates for release.json and Go modules for {new_version} release + preludes",
            release_branch,
            final_branch,
            new_version,
        )


@task(help={'upstream': "Remote repository name (default 'origin')"})
def create_rc(ctx, release_branch, patch_version=False, upstream="origin", slack_webhook=None):
    """Updates the release entries in release.json to prepare the next RC build.

    If the previous version of the Agent (determined as the latest tag on the
    current branch) is not an RC:
    - by default, updates the release entries for the next minor version of
      the Agent.
    - if --patch-version is specified, updates the release entries for the next
      patch version of the Agent.

    This changes which tags will be considered on the dependency repositories (only
    tags that match the same major and minor version as the Agent).

    If the previous version of the Agent was an RC, updates the release entries for RC + 1.

    Examples:
        If the latest tag on the branch is 7.31.0, and invoke release.create-rc --patch-version
        is run, then the task will prepare the release entries for 7.31.1-rc.1, and therefore
        will only use 7.31.X tags on the dependency repositories that follow the Agent version scheme.

        If the latest tag on the branch is 7.32.0-devel or 7.31.0, and invoke release.create-rc
        is run, then the task will prepare the release entries for 7.32.0-rc.1, and therefore
        will only use 7.32.X tags on the dependency repositories that follow the Agent version scheme.

        Updates internal module dependencies with the new RC.

        Commits the above changes, and then creates a PR on the upstream repository with the change.

        If slack_webhook is provided, it tries to send the PR URL to the provided webhook. This is meant to be used mainly in automation.

    Notes:
        This requires a Github token (either in the GITHUB_TOKEN environment variable, or in the MacOS keychain),
        with 'repo' permissions.
        This also requires that there are no local uncommitted changes, that the current branch is 'main' or the
        release branch, and that no branch named 'release/<new rc version>' already exists locally or upstream.
    """
    major_version = get_version_major(release_branch)

    with agent_context(ctx, release_branch):
        github = GithubAPI(repository=GITHUB_REPO_NAME)
        github_action = os.environ.get("GITHUB_ACTIONS")

        if github_action:
            set_git_config('user.name', 'github-actions[bot]')
            set_git_config('user.email', 'github-actions[bot]@users.noreply.github.com')
            upstream = f"https://x-access-token:{os.environ.get('GITHUB_TOKEN')}@github.com/{GITHUB_REPO_NAME}.git"

        # Get the version of the highest major: useful for some logging & to get
        # the version to use for Go submodules updates
        new_highest_version = next_rc_version(ctx, major_version, patch_version)
        # Get the next final version of the highest major: useful to know which
        # milestone to target, as well as decide which tags from dependency repositories
        # can be used.
        new_final_version = next_final_version(ctx, major_version, patch_version)
        print(color_message(f"Preparing RC for agent version {major_version}", "bold"))

        # Step 0: checks

        print(color_message("Checking repository state", "bold"))
        ctx.run("git fetch")

        # Check that the current and update branches are valid
        update_branch = f"release/{new_highest_version}-{int(time.time())}"

        check_clean_branch_state(ctx, github, update_branch)
        if not check_base_branch(release_branch, new_highest_version):
            raise Exit(
                color_message(
                    f"The branch you are on is neither {get_default_branch()} or the correct release branch ({new_highest_version.branch()}). Aborting.",
                    "red",
                ),
                code=1,
            )

        # Step 1: Update release entries
        print(color_message("Updating release entries", "bold"))
        new_version = next_rc_version(ctx, major_version, patch_version)

        update_release_json(new_version, new_final_version)

        # Step 2: Update internal module dependencies

        print(color_message("Updating Go modules", "bold"))
        update_modules(ctx, version=str(new_highest_version))

        # Step 3: branch out, commit change, push branch

        print(color_message(f"Branching out to {update_branch}", "bold"))
        ctx.run(f"git checkout -b {update_branch}")

        print(color_message("Committing release.json and Go modules updates", "bold"))
        print(
            color_message(
                "If commit signing is enabled, you will have to make sure the commit gets properly signed.", "bold"
            )
        )
        ctx.run("git add release.json")
        ctx.run("git ls-files . | grep 'go.mod$' | xargs git add")

        ok = try_git_command(
            ctx,
            f"git commit --no-verify -m 'Update release.json and Go modules for {new_highest_version}'",
            github_action,
        )
        if not ok:
            raise Exit(
                color_message(
                    f"Could not create commit. Please commit manually, push the {update_branch} branch and then open a PR against {release_branch}.",
                    "red",
                ),
                code=1,
            )

        print(color_message("Pushing new branch to the upstream repository", "bold"))
        res = ctx.run(f"git push --no-verify --set-upstream {upstream} {update_branch}", warn=True)
        if res.exited is None or res.exited > 0:
            raise Exit(
                color_message(
                    f"Could not push branch {update_branch} to the upstream '{upstream}'. Please push it manually and then open a PR against {release_branch}.",
                    "red",
                ),
                code=1,
            )

        pr_url = create_release_pr(
            f"[release] Update release.json and Go modules for {new_highest_version}",
            release_branch,
            update_branch,
            new_final_version,
        )

        # Step 4 - If slack workflow webhook is provided, send a slack message
        if slack_webhook:
            print(color_message("Sending slack notification", "bold"))
            payload = {
                "pr_url": pr_url,
                "version": str(new_highest_version),
            }

            ctx.run(f"curl -X POST -H 'Content-Type: application/json' --data '{json.dumps(payload)}' {slack_webhook}")


@task
def build_rc(ctx, release_branch, patch_version=False, k8s_deployments=False):
    """To be done after the PR created by release.create-rc is merged, with the same options
    as release.create-rc.

    Tags the new RC versions on the current commit, and creates the build pipeline for these
    new tags.

    Args:
        k8s_deployments: When set to True the child pipeline deploying to subset of k8s staging clusters will be triggered.
    """

    major_version = get_version_major(release_branch)

    with agent_context(ctx, release_branch):
        datadog_agent = get_gitlab_repo()

        # Get the version of the highest major: needed for tag_version and to know
        # which tag to target when creating the pipeline.
        new_version = next_rc_version(ctx, major_version, patch_version)

        # Get a string representation of the RC, eg. "6/7.32.0-rc.1"
        versions_string = str(new_version)

        # Step 0: checks

        print(color_message("Checking repository state", "bold"))

        # Check that the base branch is valid
        if not check_base_branch(release_branch, new_version):
            raise Exit(
                color_message(
                    f"The branch you are on is neither {get_default_branch()} or the correct release branch ({new_version.branch()}). Aborting.",
                    "red",
                ),
                code=1,
            )

        latest_commit = ctx.run("git --no-pager log --no-color -1 --oneline").stdout.strip()

        if not yes_no_question(
            f"This task will create tags for {versions_string} on the current commit: {latest_commit}. Is this OK?",
            color="orange",
            default=False,
        ):
            raise Exit(color_message("Aborting.", "red"), code=1)

        # Step 1: Tag versions

        print(color_message(f"Tagging RC for agent version {versions_string}", "bold"))
        print(
            color_message(
                "If commit signing is enabled, you will have to make sure each tag gets properly signed.", "bold"
            )
        )

        # tag_version only takes the highest version (Agent 7 currently), and creates
        # the tags for all supported versions
        # TODO: make it possible to do Agent 6-only or Agent 7-only tags?
        tag_version(ctx, version=str(new_version), force=False)
        tag_modules(ctx, version=str(new_version), force=False)

        print(color_message(f"Waiting until the {new_version} tag appears in Gitlab", "bold"))
        gitlab_tag = None
        while not gitlab_tag:
            try:
                gitlab_tag = datadog_agent.tags.get(str(new_version))
            except GitlabError:
                continue

            sleep(5)

        print(color_message("Creating RC pipeline", "bold"))

        # Step 2: Run the RC pipeline

        run(
            ctx,
            git_ref=gitlab_tag.name,
            use_release_entries=True,
            repo_branch="beta",
            deploy=True,
            rc_build=True,
            rc_k8s_deployments=k8s_deployments,
        )


@task(help={'key': "Path to an existing release.json key, separated with double colons, eg. 'last_stable::6'"})
def set_release_json(ctx, key, value, release_branch=None, skip_checkout=False, worktree=False):
    def _main():
        nonlocal key

        release_json = load_release_json()
        path = key.split('::')
        current_node = release_json
        for key_idx in range(len(path)):
            key = path[key_idx]
            if key not in current_node:
                raise Exit(code=1, message=f"Couldn't find '{key}' in release.json")
            if key_idx == len(path) - 1:
                current_node[key] = value
                break
            else:
                current_node = current_node[key]
        _save_release_json(release_json)

    if worktree:
        with agent_context(ctx, release_branch, skip_checkout=skip_checkout):
            _main()
    else:
        _main()


@task(help={'key': "Path to the release.json key, separated with double colons, eg. 'last_stable::6'"})
def get_release_json_value(ctx, key, release_branch=None, skip_checkout=False, worktree=True):
    if worktree:
        with agent_context(ctx, release_branch, skip_checkout=skip_checkout):
            release_json = _get_release_json_value(key)
    else:
        release_json = _get_release_json_value(key)

    print(release_json)


def create_and_update_release_branch(
    ctx, repo, release_branch, base_branch: str | None = None, base_directory="~/dd", upstream="origin"
):
    """Create and push a release branch to `repo`.

    Args:
        base_branch: Branch from which we create the release branch. Default branch if `None`.
    """

    def _main():
        ctx.run("git pull")
        print(color_message(f"Branching out to {release_branch}", "bold"))
        ctx.run(f"git checkout -b {release_branch}")

        # Step 2 - Push newly created release branch to the remote repository

        print(color_message("Pushing new branch to the upstream repository", "bold"))
        res = ctx.run(f"git push --set-upstream {upstream} {release_branch}", warn=True)
        if res.exited is None or res.exited > 0:
            raise Exit(
                color_message(
                    f"Could not push branch {release_branch} to the upstream '{upstream}'. Please push it manually.",
                    "red",
                ),
                code=1,
            )

    # Perform branch out in all required repositories
    print(color_message(f"Working repository: {repo}", "bold"))
    if repo == 'datadog-agent':
        with agent_context(ctx, base_branch or get_default_branch(major=get_version_major(release_branch))):
            _main()
    else:
        with ctx.cd(f"{base_directory}/{repo}"):
            # Step 1 - Create a local branch out from the default branch
            main_branch = (
                base_branch
                or ctx.run(f"git remote show {upstream} | grep \"HEAD branch\" | sed 's/.*: //'").stdout.strip()
            )
            ctx.run(f"git checkout {main_branch}")

            _main()


# TODO: unfreeze is the former name of this task, kept for backward compatibility. Remove in a few weeks.
@task(help={'upstream': "Remote repository name (default 'origin')"}, aliases=["unfreeze"])
def create_release_branches(ctx, base_directory="~/dd", major_version: int = 7, upstream="origin", check_state=True):
    """Create and push release branches in Agent repositories and update them.

    That includes:
        - creates a release branch in datadog-agent, datadog-agent-macos, omnibus-ruby and omnibus-software repositories,
        - updates release.json on new datadog-agent branch to point to newly created release branches in nightly section
        - updates entries in .gitlab-ci.yml and .gitlab/notify/notify.yml which depend on local branch name

    Args:
        base_directory: Path to the directory where dd repos are cloned, defaults to ~/dd, but can be overwritten.
        use_worktree: If True, will go to datadog-agent-worktree instead of datadog-agent.

    Notes:
        This requires a Github token (either in the GITHUB_TOKEN environment variable, or in the MacOS keychain),
        with 'repo' permissions.
        This also requires that there are no local uncommitted changes, that the current branch is 'main' or the
        release branch, and that no branch named 'release/<new rc version>' already exists locally or upstream.
    """

    github = GithubAPI(repository=GITHUB_REPO_NAME)

    current = current_version(ctx, major_version)
    current.rc = False
    current.devel = False

    # Strings with proper branch/tag names
    release_branch = current.branch()

    with agent_context(ctx, get_default_branch(major=major_version)):
        # Step 0: checks
        ctx.run("git fetch")

        if check_state:
            print(color_message("Checking repository state", "bold"))
            check_clean_branch_state(ctx, github, release_branch)

        if not yes_no_question(
            f"This task will create new branches with the name '{release_branch}' in repositories: {', '.join(UNFREEZE_REPOS)}. Is this OK?",
            color="orange",
            default=False,
        ):
            raise Exit(color_message("Aborting.", "red"), code=1)

        # Step 1 - Create release branches in all required repositories

        base_branch = get_default_branch() if major_version == 6 else None

        for repo in UNFREEZE_REPOS:
            create_and_update_release_branch(
                ctx, repo, release_branch, base_branch=base_branch, base_directory=base_directory, upstream=upstream
            )

        # create the backport label in the Agent repo
        print(color_message("Creating backport label in the Agent repository", Color.BOLD))
        github.create_label(
            f'backport/{release_branch}',
            BACKPORT_LABEL_COLOR,
            f'Automatically create a backport PR to {release_branch}',
        )

        # Step 2 - Create PRs with new settings in datadog-agent repository
        # Step 2.0 - Update release.json
        update_branch = f"{release_branch}-updates"

        ctx.run(f"git checkout {release_branch}")
        ctx.run(f"git checkout -b {update_branch}")

        set_new_release_branch(release_branch)

        # Step 1.2 - In datadog-agent repo update gitlab-ci.yaml and notify.yml jobs
        for file in GITLAB_FILES_TO_UPDATE:
            with open(file) as gl:
                file_content = gl.readlines()

            with open(file, "w") as gl:
                for line in file_content:
                    if re.search(rf"compare_to: {get_default_branch()}", line):
                        gl.write(line.replace(get_default_branch(), f"{release_branch}"))
                    else:
                        gl.write(line)

        # Step 1.3 - Commit new changes
        ctx.run("git add release.json .gitlab-ci.yml .gitlab/notify/notify.yml")
        ok = try_git_command(
            ctx, f"git commit -m 'Update release.json, .gitlab-ci.yml and notify.yml with {release_branch}'"
        )
        if not ok:
            raise Exit(
                color_message(
                    f"Could not create commit. Please commit manually and push the commit to the {release_branch} branch.",
                    "red",
                ),
                code=1,
            )

        # Step 1.4 - Push branch and create PR
        print(color_message("Pushing new branch to the upstream repository", "bold"))
        res = ctx.run(f"git push --set-upstream {upstream} {update_branch}", warn=True)
        if res.exited is None or res.exited > 0:
            raise Exit(
                color_message(
                    f"Could not push branch {update_branch} to the upstream '{upstream}'. Please push it manually and then open a PR against {release_branch}.",
                    "red",
                ),
                code=1,
            )

        create_release_pr(
            f"[release] Update release.json and gitlab files for {release_branch} branch",
            release_branch,
            update_branch,
            current,
        )


def _update_last_stable(_, version, major_version: int = 7):
    """
    Updates the last_release field(s) of release.json and returns the current milestone
    """
    release_json = load_release_json()
    # If the release isn't a RC, update the last stable release field
    version.major = major_version
    release_json['last_stable'][str(major_version)] = str(version)
    _save_release_json(release_json)

    return release_json["current_milestone"]


@task
def cleanup(ctx, release_branch):
    """Perform the post release cleanup steps

    Currently this:
      - Updates the scheduled nightly pipeline to target the new stable branch
      - Updates the release.json last_stable fields
    """

    # This task will create a PR to update the last_stable field in release.json
    # It must create the PR against the default branch (6 or 7), so setting the context on it
    main_branch = get_default_branch()
    with agent_context(ctx, main_branch):
        gh = GithubAPI()
        major_version = get_version_major(release_branch)
        latest_release = gh.latest_release(major_version)
        match = VERSION_RE.search(latest_release)
        if not match:
            raise Exit(f'Unexpected version fetched from github {latest_release}', code=1)

        version = _create_version_from_match(match)
        current_milestone = _update_last_stable(ctx, version, major_version=major_version)

        # create pull request to update last stable version
        cleanup_branch = f"release/{version}-cleanup"
        ctx.run(f"git checkout -b {cleanup_branch}")
        ctx.run("git add release.json")

        commit_message = f"Update last_stable to {version}"
        ok = try_git_command(ctx, f"git commit -m '{commit_message}'")
        if not ok:
            raise Exit(
                color_message(
                    f"Could not create commit. Please commit manually with:\ngit commit -m {commit_message}\n, push the {cleanup_branch} branch and then open a PR against {main_branch}.",
                    "red",
                ),
                code=1,
            )

        if not ctx.run(f"git push --set-upstream origin {cleanup_branch}", warn=True):
            raise Exit(
                color_message(
                    f"Could not push branch {cleanup_branch} to the upstream 'origin'. Please push it manually and then open a PR against {main_branch}.",
                    "red",
                ),
                code=1,
            )

        create_release_pr(commit_message, main_branch, cleanup_branch, version, milestone=current_milestone)

    if major_version != 6:
        edit_schedule(ctx, 2555, ref=version.branch())


@task
def check_omnibus_branches(ctx, release_branch=None, worktree=True):
    def _main():
        base_branch = _get_release_json_value('base_branch')
        if base_branch == get_default_branch():
            default_branches = DEFAULT_BRANCHES_AGENT6 if is_agent6(ctx) else DEFAULT_BRANCHES
            omnibus_ruby_branch = default_branches['omnibus-ruby']
            omnibus_software_branch = default_branches['omnibus-software']
        else:
            omnibus_ruby_branch = base_branch
            omnibus_software_branch = base_branch

        def _check_commit_in_repo(repo_name, branch, release_json_field):
            with tempfile.TemporaryDirectory() as tmpdir:
                ctx.run(
                    f'git clone --depth=50 https://github.com/DataDog/{repo_name} --branch {branch} {tmpdir}/{repo_name}',
                    hide='stdout',
                )
                for version in ['nightly', 'nightly-a7']:
                    commit = _get_release_json_value(f'{version}::{release_json_field}')
                    if (
                        ctx.run(f'git -C {tmpdir}/{repo_name} branch --contains {commit}', warn=True, hide=True).exited
                        != 0
                    ):
                        raise Exit(
                            code=1,
                            message=f'{repo_name} commit ({commit}) is not in the expected branch ({branch}). The PR is not mergeable',
                        )
                    else:
                        print(f'[{version}] Commit {commit} was found in {repo_name} branch {branch}')

        _check_commit_in_repo('omnibus-ruby', omnibus_ruby_branch, 'OMNIBUS_RUBY_VERSION')
        _check_commit_in_repo('omnibus-software', omnibus_software_branch, 'OMNIBUS_SOFTWARE_VERSION')

        return True

    if worktree:
        with agent_context(ctx, release_branch):
            return _main()
    else:
        return _main()


@task
def update_build_links(_, new_version, patch_version=False):
    """Updates Agent release candidates build links on https://datadoghq.atlassian.net/wiki/spaces/agent/pages/2889876360/Build+links

    Args:
        new_version: Should be given as an Agent 7 RC version, ie. '7.50.0-rc.1' format. Does not support patch version unless patch_version is set to True.
        patch_version: If set to True, then task can be used for patch releases (3 digits), ie. '7.50.1-rc.1' format. Otherwise patch release number will be considered as invalid.

    Notes:
        Attlasian credentials are required to be available as ATLASSIAN_USERNAME and ATLASSIAN_PASSWORD as environment variables.
        ATLASSIAN_USERNAME is typically an email address.
        ATLASSIAN_PASSWORD is a token. See: https://id.atlassian.com/manage-profile/security/api-tokens
    """

    from atlassian import Confluence
    from atlassian.confluence import ApiError

    BUILD_LINKS_PAGE_ID = 2889876360

    match = RC_VERSION_RE.match(new_version) if patch_version else MINOR_RC_VERSION_RE.match(new_version)
    if not match:
        raise Exit(
            color_message(
                f"{new_version} is not a valid {'patch' if patch_version else 'minor'} Agent RC version number/tag.\nCorrect example: 7.50{'.1' if patch_version else '.0'}-rc.1",
                "red",
            ),
            code=1,
        )

    username = os.getenv("ATLASSIAN_USERNAME")
    password = os.getenv("ATLASSIAN_PASSWORD")

    if username is None or password is None:
        raise Exit(
            color_message(
                "No Atlassian credentials provided. Run inv --help update-build-links for more details.",
                "red",
            ),
            code=1,
        )

    confluence = Confluence(url="https://datadoghq.atlassian.net/", username=username, password=password)

    content = confluence.get_page_by_id(page_id=BUILD_LINKS_PAGE_ID, expand="body.storage")

    title = content["title"]
    current_version = title.split()[-1].strip()
    body = content["body"]["storage"]["value"]

    title = title.replace(current_version, new_version)

    patterns = _create_build_links_patterns(current_version, new_version)

    for key in patterns:
        body = body.replace(key, patterns[key])

    print(color_message(f"Updating QA Build links page with {new_version}", "bold"))

    try:
        confluence.update_page(BUILD_LINKS_PAGE_ID, title, body=body)
    except ApiError as e:
        raise Exit(
            color_message(
                f"Failed to update confluence page. Reason: {e.reason}",
                "red",
            ),
            code=1,
        ) from e
    print(color_message("Build links page updated", "green"))


def _create_build_links_patterns(current_version, new_version):
    patterns = {}

    current_minor_version = current_version[1:]
    new_minor_version = new_version[1:]

    patterns[current_minor_version] = new_minor_version
    patterns[current_minor_version.replace("rc.", "rc-")] = new_minor_version.replace("rc.", "rc-")
    patterns[current_minor_version.replace("-rc", "~rc")] = new_minor_version.replace("-rc", "~rc")
    patterns[current_minor_version[1:].replace("-rc", "~rc")] = new_minor_version[1:].replace("-rc", "~rc")

    return patterns


@task
def get_active_release_branch(ctx, release_branch):
    """Determine what is the current active release branch for the Agent within the release worktree.

    If release started and code freeze is in place - main branch is considered active.
    If release started and code freeze is over - release branch is considered active.
    """

    with agent_context(ctx, branch=release_branch):
        gh = GithubAPI()
        next_version = get_next_version(gh, latest_release=gh.latest_release(6) if is_agent6(ctx) else None)
        release_branch = gh.get_branch(next_version.branch())
        if release_branch:
            print(f"{release_branch.name}")
        else:
            print(get_default_branch())


@task
def get_unreleased_release_branches(_):
    """
    Determine what are the current active release branches for the Agent.
    """
    gh = GithubAPI()
    print(json.dumps([branch.name for branch in gh.latest_unreleased_release_branches()]))


def get_next_version(gh, latest_release=None):
    latest_release = latest_release or gh.latest_release()
    current_version = _create_version_from_match(VERSION_RE.search(latest_release))
    return current_version.next_version(bump_minor=True)


@task
def generate_release_metrics(ctx, milestone, freeze_date, release_date):
    """Task to run after the release is done to generate release metrics.

    Args:
        milestone: Github milestone number for the release. Expected format like '7.54.0'
        freeze_date: Date when the code freeze was started. Expected format YYYY-MM-DD, like '2022-02-01'
        release_date: Date when the release was done. Expected format YYYY-MM-DD, like '2022-09-15'

    Notes:
        Results are formatted in a way that can be easily copied to https://docs.google.com/spreadsheets/d/1r39CtyuvoznIDx1JhhLHQeAzmJB182n7ln8nToiWQ8s/edit#gid=1490566519
        Copy paste numbers to the respective sheets and select 'Split text to columns'.
    """

    # Step 1: Lead Time for Changes data
    lead_time = get_release_lead_time(freeze_date, release_date)
    print("Lead Time for Changes data")
    print("--------------------------")
    print(lead_time)

    # Step 2: Agent stability data
    prs = get_prs_metrics(milestone, freeze_date)
    print("\n")
    print("Agent stability data")
    print("--------------------")
    print(f"{prs['total']}, {prs['before_freeze']}, {prs['on_freeze']}, {prs['after_freeze']}")

    # Step 3: Code changes
    code_stats = ctx.run(
        f"git log --shortstat {milestone}-devel..{milestone} | grep \"files changed\" | awk '{{files+=$1; inserted+=$4; deleted+=$6}} END {{print files,\",\", inserted,\",\", deleted}}'",
        hide=True,
    ).stdout.strip()
    print("\n")
    print("Code changes")
    print("------------")
    print(code_stats)


# TODO rename to freeze_date to cutoff_date
@task
def create_schedule(_, version, freeze_date):
    """Create confluence pages for the release schedule.

    Args:
        freeze_date: Date when the code cut-off happened. Expected format YYYY-MM-DD, like '2022-02-01'
    """

    required_environment_variables = ["ATLASSIAN_USERNAME", "ATLASSIAN_PASSWORD"]
    if not all(key in os.environ for key in required_environment_variables):
        raise Exit(f"You must set {required_environment_variables} environment variables to use this task.", code=1)
    release_page = create_release_page(version, date.fromisoformat(freeze_date))
    print(f"Release schedule pages {release_page['url']} {color_message('successfully created', 'green')}")


@task
def chase_release_managers(_, version):
    url, missing_teams = get_release_page_info(version)
    github_slack_map = load_and_validate("github_slack_map.yaml", "DEFAULT_SLACK_CHANNEL", DEFAULT_SLACK_CHANNEL)
    channels = set()

    for team in missing_teams:
        channel = github_slack_map.get(f"@datadog/{team}")
        if channel:
            channels.add(channel)
        else:
            print(color_message(f"Missing slack channel for {team}", Color.RED))

    message = f"Hello :wave:\nCould you please update the `datadog-agent` <{url}|release coordination page> with the RM for your team?\nThanks in advance"

    from slack_sdk import WebClient

    client = WebClient(os.environ["SLACK_API_TOKEN"])
    for channel in sorted(channels):
        print(f"Sending message to {channel}")
        client.chat_postMessage(channel=channel, text=message)


@task
def chase_for_qa_cards(_, version):
    from slack_sdk import WebClient

    cards = list_not_closed_qa_cards(version)
    if not cards:
        print(f"[{color_message('OK', Color.GREEN)}] No QA cards to chase")
        return
    grouped_cards = defaultdict(list)
    for card in cards:
        grouped_cards[card["fields"]["project"]["key"]].append(card)
    GITHUB_SLACK_MAP = load_and_validate("github_slack_map.yaml", "DEFAULT_SLACK_CHANNEL", DEFAULT_SLACK_CHANNEL)
    GITHUB_JIRA_MAP = load_and_validate("github_jira_map.yaml", "DEFAULT_JIRA_PROJECT", DEFAULT_JIRA_PROJECT)
    client = WebClient(os.environ["SLACK_API_TOKEN"])
    print(f"Found {len(cards)} QA cards to chase")
    for project, cards in grouped_cards.items():
        team = next(team for team, jira_project in GITHUB_JIRA_MAP.items() if project == jira_project)
        channel = GITHUB_SLACK_MAP[team]
        print(f" - {channel} for {[card['key'] for card in cards]}")
        card_links = ", ".join(
            [f"<https://datadoghq.atlassian.net/browse/{card['key']}|{card['key']}>" for card in cards]
        )
        message = f"Hello :wave:\nCould you please update the QA cards {card_links} for the {version} release?\nThanks in advance"
        client.chat_postMessage(channel=channel, text=message)


@task
def check_for_changes(ctx, release_branch, warning_mode=False):
    """
    Check if there was any modification on the release repositories since last release candidate.
    """
    with agent_context(ctx, release_branch):
        major_version = get_version_major(release_branch)
        next_version = next_rc_version(ctx, major_version)
        repo_data = generate_repo_data(ctx, warning_mode, next_version, release_branch)
        changes = 'false'
        for repo_name, repo in repo_data.items():
            head_commit = get_last_commit(ctx, repo_name, repo['branch'])
            last_tag_commit, last_tag_name = get_last_release_tag(ctx, repo_name, next_version.tag_pattern())
            if last_tag_commit != "" and last_tag_commit != head_commit:
                changes = 'true'
                print(f"{repo_name} has new commits since {last_tag_name}", file=sys.stderr)
                if warning_mode:
                    team = "agent-integrations"
                    emails = release_manager(next_version.clone(), team)
                    warn_new_commits(emails, team, repo['branch'], next_version)
                else:
                    if repo_name not in ["datadog-agent", "integrations-core"]:
                        with clone(ctx, repo_name, repo['branch'], options="--filter=blob:none --no-checkout"):
                            # We can add the new commit now to be used by release candidate creation
                            print(f"Creating new tag {next_version} on {repo_name}", file=sys.stderr)
                            ctx.run(f"git tag {next_version}")
                            ctx.run(f"git push origin tag {next_version}")
                # This repo has changes, the next check is not needed
                continue
            if repo_name != "datadog-agent" and last_tag_name != repo['previous_tag']:
                changes = 'true'
                print(
                    f"{repo_name} has a new tag {last_tag_name} since last release candidate (was {repo['previous_tag']})",
                    file=sys.stderr,
                )
        # Send a value for the create_rc_pr.yml workflow
        print(changes)


@task
def create_qa_cards(ctx, tag):
    """
    Automate the call to ddqa
    """
    from tasks.libs.releasing.qa import get_labels, setup_ddqa

    version = _create_version_from_match(VERSION_RE.match(tag))
    if not version.rc:
        print(f"{tag} is not a release candidate, skipping")
        return
    setup_ddqa(ctx)
    ctx.run(f"ddqa --auto create {version.previous_rc_version()} {tag} {get_labels(version)}")


@task
def create_github_release(ctx, release_branch, draft=True):
    """
    Create a GitHub release for the given tag.
    """
    import pandoc

    sections = (
        ("Agent", "CHANGELOG.rst"),
        ("Datadog Cluster Agent", "CHANGELOG-DCA.rst"),
    )

    notes = []
    version = deduce_version(ctx, release_branch, next_version=False)

    with agent_context(ctx, release_branch):
        for section, filename in sections:
            text = pandoc.write(pandoc.read(file=filename), format="markdown_strict", options=["--wrap=none"])

            header_found = False
            lines = []

            # Extract the section for the given version
            for line in text.splitlines():
                # Move to the right section
                if line.startswith("## " + version):
                    header_found = True
                    continue

                if header_found:
                    # Next version found, stop
                    if line.startswith("## "):
                        break
                    lines.append(line)

            # if we found the header, add the section to the final release note
            if header_found:
                notes.append(f"# {section}")
                notes.extend(lines)

        if not notes:
            print(f"No release notes found for {version}")
            raise Exit(code=1)

        github = GithubAPI()
        release = github.create_release(
            version,
            "\n".join(notes),
            draft=draft,
        )

        print(f"Link to the release note: {release.html_url}")


@task
def update_current_milestone(ctx, major_version: int = 7, upstream="origin"):
    """
    Create a PR to bump the current_milestone in the release.json file
    """
    import github

    gh = GithubAPI()

    current = current_version(ctx, major_version)
    next = current.next_version(bump_minor=True)
    next.devel = False

    print(f"Creating the {next} milestone...")

    try:
        gh.create_milestone(str(next))
    except github.GithubException as e:
        if e.status == 422:
            print(f"Milestone {next} already exists")
        else:
            raise e

    with agent_context(ctx, get_default_branch(major=major_version)):
        milestone_branch = f"release_milestone-{int(time.time())}"
        ctx.run(f"git switch -c {milestone_branch}")
        rj = load_release_json()
        rj["current_milestone"] = f"{next}"
        _save_release_json(rj)
        # Commit release.json
        ctx.run("git add release.json")
        ok = try_git_command(ctx, f"git commit -m 'Update release.json with current milestone to {next}'")

        if not ok:
            raise Exit(
                color_message(
                    f"Could not create commit. Please commit manually and push the commit to the {milestone_branch} branch.",
                    Color.RED,
                ),
                code=1,
            )

        res = ctx.run(f"git push --set-upstream {upstream} {milestone_branch}", warn=True)
        if res.exited is None or res.exited > 0:
            raise Exit(
                color_message(
                    f"Could not push branch {milestone_branch} to the upstream '{upstream}'. Please push it manually and then open a PR against main.",
                    Color.RED,
                ),
                code=1,
            )

        create_release_pr(
            f"[release] Update current milestone to {next}",
            get_default_branch(),
            milestone_branch,
            next,
        )
