"""Release helper tasks

Notes about Agent6:
    Release tasks should be run from the main branch.
    To make a task compatible with agent 6, it is possible to use agent_context such that
    the task will be run in the agent6 branch.
"""

import json
import os
import sys
import tempfile
import time
from collections import defaultdict
from datetime import date
from time import sleep

from gitlab import GitlabError
from invoke import Failure, task
from invoke.exceptions import Exit

from tasks.libs.ciproviders.github_api import GithubAPI, create_release_pr
from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.constants import (
    GITHUB_REPO_NAME,
)
from tasks.libs.common.datadog_api import get_ci_pipeline_events
from tasks.libs.common.git import (
    check_base_branch,
    check_clean_branch_state,
    create_tree,
    get_default_branch,
    get_last_commit,
    get_last_release_tag,
    is_agent6,
    push_tags_in_batches,
    try_git_command,
)
from tasks.libs.common.gomodules import get_default_modules
from tasks.libs.common.user_interactions import yes_no_question
from tasks.libs.common.utils import running_in_ci, set_gitconfig_in_ci
from tasks.libs.common.worktree import agent_context
from tasks.libs.pipeline.notifications import (
    DEFAULT_JIRA_PROJECT,
    DEFAULT_SLACK_CHANNEL,
    load_and_validate,
    warn_new_commits,
    warn_new_tags,
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
    get_current_milestone,
    load_release_json,
    set_current_milestone,
    set_new_release_branch,
    update_release_json,
)
from tasks.libs.releasing.notes import _add_dca_prelude, _add_prelude
from tasks.libs.releasing.version import (
    FINAL_VERSION_RE,
    MINOR_RC_VERSION_RE,
    RC_VERSION_RE,
    RELEASE_JSON_DEPENDENCIES,
    VERSION_RE,
    _create_version_from_match,
    current_version,
    deduce_version,
    get_version_major,
    next_final_version,
    next_rc_version,
)
from tasks.notify import post_message
from tasks.pipeline import edit_schedule, run
from tasks.release_metrics.metrics import get_prs_metrics, get_release_lead_time

BACKPORT_LABEL_COLOR = "5319e7"
QUALIFICATION_TAG = "qualification"


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
        $ dda inv -e release.update-modules 7.27.x
    """

    assert release_branch or version

    agent_version = version or deduce_version(ctx, release_branch, trust=trust)

    with agent_context(ctx, release_branch, skip_checkout=release_branch is None):
        modules = get_default_modules()
        for module in modules.values():
            for dependency in module.dependencies:
                dependency_mod = modules[dependency]
                if (
                    agent_version.startswith('6')
                    and 'pkg/util/optional' in dependency_mod.dependency_path(agent_version)
                    and 'test/new-e2e' in module.go_mod_path()
                ):
                    # Skip this dependency update in new-e2e for Agent 6, as it's incompatible.
                    continue
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


def __tag_single_module(ctx, module, tag_name, commit, force_option, devel):
    """Tag a given module."""
    tags = []
    tags_to_commit = module.tag(tag_name) if VERSION_RE.match(tag_name) else [tag_name]
    for tag in tags_to_commit:
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
    ctx,
    release_branch=None,
    commit="HEAD",
    push=True,
    force=False,
    devel=False,
    version=None,
    trust=False,
    skip_agent_context=False,
):
    """Create tags for Go nested modules for a given Datadog Agent version.

    Args:
        commit: Will tag `commit` with the tags (default HEAD).
        verify: Checks for correctness on the Agent version (on by default).
        push: Will push the tags to the origin remote (on by default).
        force: Will allow the task to overwrite existing tags. Needed to move existing tags (off by default).
        devel: Will create -devel tags (used after creation of the release branch).
        skip_agent_context: Won't do context change if set.

    Examples:
        $ dda inv -e release.tag-modules 7.27.x                 # Create tags and push them to origin
        $ dda inv -e release.tag-modules 7.27.x --no-push       # Create tags locally; don't push them
        $ dda inv -e release.tag-modules 7.29.x --force         # Create tags (overwriting existing tags with the same name), force-push them to origin
    """

    assert release_branch or version

    agent_version = version or deduce_version(ctx, release_branch, trust=trust)

    def _tag_modules():
        tags = []
        force_option = __get_force_option(force)
        for module in get_default_modules().values():
            # Skip main module; this is tagged at tag_version via __tag_single_module.
            if module.should_tag and module.path != ".":
                new_tags = __tag_single_module(ctx, module, agent_version, commit, force_option, devel)
                tags.extend(new_tags)

        if push:
            set_gitconfig_in_ci(ctx)
            push_tags_in_batches(ctx, tags, force_option)
        print(f"Created module tags for version {agent_version}")

    if skip_agent_context:
        _tag_modules()
    else:
        with agent_context(ctx, release_branch, skip_checkout=release_branch is None):
            _tag_modules()


@task
def tag_version(
    ctx,
    release_branch=None,
    commit="HEAD",
    push=True,
    force=False,
    devel=False,
    version=None,
    trust=False,
    start_qual=False,
    skip_agent_context=False,
):
    """Create tags for a given Datadog Agent version.

    Args:
        commit: Will tag `commit` with the tags (default HEAD).
        verify: Checks for correctness on the Agent version (on by default).
        push: Will push the tags to the origin remote (on by default).
        force: Will allow the task to overwrite existing tags. Needed to move existing tags (off by default).
        devel: Will create -devel tags (used after creation of the release branch).
        start_qual: Will start the qualification phase for agent 6 release candidate by adding a qualification tag.
        skip_agent_context: Won't do context change if set.

    Examples:
        $ dda inv -e release.tag-version -r 7.27.x            # Create tags and push them to origin
        $ dda inv -e release.tag-version -r 7.27.x --no-push  # Create tags locally; don't push them
        $ dda inv -e release.tag-version -r 7.29.x --force    # Create tags (overwriting existing tags with the same name), force-push them to origin
    """

    assert release_branch or version

    agent_version = version or deduce_version(ctx, release_branch, trust=trust)

    def _tag_version():
        # Always tag the main module
        force_option = __get_force_option(force)
        tags = __tag_single_module(ctx, get_default_modules()["."], agent_version, commit, force_option, devel)

        set_gitconfig_in_ci(ctx)
        if is_agent6(ctx) and (start_qual or is_qualification(ctx, "6.53.x")):
            # remove all the qualification tags if it is the final version
            if FINAL_VERSION_RE.match(agent_version):
                qualification_tags = [tag for _, tag in get_qualification_tags(ctx, release_branch)]
                push_tags_in_batches(ctx, qualification_tags, delete=True)
            # create or update the qualification tag on the current commit
            else:
                tags += __tag_single_module(
                    ctx, get_default_modules()["."], f"{QUALIFICATION_TAG}-{int(time.time())}", commit, "", False
                )

        if push:
            push_tags_in_batches(ctx, tags, force_option)
            print(f"Created tags for version {agent_version}")

    if skip_agent_context:
        _tag_version()
    else:
        with agent_context(ctx, release_branch, skip_checkout=release_branch is None):
            _tag_version()


@task
def tag_devel(ctx, release_branch, commit="HEAD", push=True, force=False):
    with agent_context(ctx, get_default_branch(major=get_version_major(release_branch))):
        tag_version(ctx, release_branch, commit, push, force, devel=True, skip_agent_context=True)
        tag_modules(ctx, release_branch, commit, push, force, devel=True, trust=True, skip_agent_context=True)


@task
def finish(ctx, release_branch, upstream="origin"):
    """Updates the release.json file for the new version.

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
        new_version = next_final_version(ctx, release_branch, False)
        if not yes_no_question(
            f'Do you want to finish the release with version {new_version}?', color="bold", default=False
        ):
            raise Exit(color_message("Aborting.", "red"), code=1)
        update_release_json(new_version, new_version)

        next_milestone = next_final_version(ctx, release_branch, True)
        next_milestone = next_milestone.next_version(bump_patch=True)
        previous_milestone = get_current_milestone()
        print(f"Creating the {next_milestone} milestone...")

        gh = GithubAPI()
        gh.create_milestone(str(next_milestone), exist_ok=True)
        set_current_milestone(str(next_milestone))

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

        set_gitconfig_in_ci(ctx)
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
            milestone=previous_milestone,
        )


@task(help={'upstream': "Remote repository name (default 'origin')"})
def create_rc(ctx, release_branch, patch_version=False, upstream="origin"):
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
        If the latest tag on the branch is 7.31.0, and dda inv release.create-rc --patch-version
        is run, then the task will prepare the release entries for 7.31.1-rc.1, and therefore
        will only use 7.31.X tags on the dependency repositories that follow the Agent version scheme.

        If the latest tag on the branch is 7.32.0-devel or 7.31.0, and dda inv release.create-rc
        is run, then the task will prepare the release entries for 7.32.0-rc.1, and therefore
        will only use 7.32.X tags on the dependency repositories that follow the Agent version scheme.

        Updates internal module dependencies with the new RC.

        Commits the above changes, and then creates a PR on the upstream repository with the change.

    Notes:
        This requires a Github token (either in the GITHUB_TOKEN environment variable, or in the MacOS keychain),
        with 'repo' permissions.
        This also requires that there are no local uncommitted changes, that the current branch is 'main' or the
        release branch, and that no branch named 'release/<new rc version>' already exists locally or upstream.
    """
    major_version = get_version_major(release_branch)

    with agent_context(ctx, release_branch):
        github = GithubAPI(repository=GITHUB_REPO_NAME)

        # Get the version of the highest major: useful for some logging & to get
        # the version to use for Go submodules updates
        new_highest_version = next_rc_version(ctx, release_branch, patch_version)
        # Get the next final version of the highest major: useful to know which
        # milestone to target, as well as decide which tags from dependency repositories
        # can be used.
        new_final_version = next_final_version(ctx, release_branch, patch_version)
        print(color_message(f"Preparing RC for agent version {major_version}", "bold"))

        # Step 0: checks

        print(color_message("Checking repository state", "bold"))
        ctx.run("git fetch")

        # Check that the current and update branches are valid
        update_branch = f"release/{new_highest_version}-{int(time.time())}"

        check_clean_branch_state(ctx, github, update_branch)
        active_releases = [branch.name for branch in github.latest_unreleased_release_branches()]
        # Bypass if we want to cut a patch release, in that case the branch is not considered "active"
        if not patch_version and not any(
            check_base_branch(release_branch, unreleased_branch) for unreleased_branch in active_releases
        ):
            raise Exit(
                color_message(
                    f"The branch you are on is neither {get_default_branch()} or amongst the active release branches ({active_releases}). Aborting.",
                    "red",
                ),
                code=1,
            )

        # Step 1: Update release entries
        print(color_message("Updating release entries", "bold"))
        new_version = next_rc_version(ctx, release_branch, patch_version)

        update_release_json(new_version, new_final_version)

        # Step 2: Update internal module dependencies

        print(color_message("Updating Go modules", "bold"))
        update_modules(ctx, version=str(new_highest_version))

        # Step 3: branch out, push branch, then add, and create signed commit with Github API

        print(color_message(f"Branching out to {update_branch}", "bold"))
        ctx.run(f"git checkout -b {update_branch}")
        ctx.run(f"git push --set-upstream {upstream} {update_branch}")
        print(color_message("Committing release.json and Go modules updates", "bold"))
        print(
            color_message(
                "If commit signing is enabled, you will have to make sure the commit gets properly signed.", "bold"
            )
        )
        ctx.run("git add release.json")
        ctx.run("git ls-files . | grep 'go.mod$' | xargs git add")

        commit_message = f"Update release.json and Go modules for {new_highest_version}"
        if running_in_ci():
            print("Creating signed commits using Github API")
            tree = create_tree(ctx, release_branch)
            github.commit_and_push_signed(update_branch, commit_message, tree)
        else:
            print("Creating commits using your local git configuration, please make sure to sign them")
            ok = try_git_command(ctx, f"git commit --no-verify -m '{commit_message}'")
            if not ok:
                raise Exit(
                    color_message(
                        f"Could not create commit. Please commit manually, push the {update_branch} branch and then open a PR against {release_branch}.",
                        "red",
                    ),
                    code=1,
                )
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

        # Step 4 - Send a slack message
        message = f":alert_party: New Agent RC <{pr_url}/s|PR> has been created {new_highest_version}."
        channel = 'agent-release-sync'
        if major_version == 6:
            channel = 'agent-ci-on-call'
            message += "\nCan you please merge this PR and trigger a build pipeline according to <https://datadoghq.atlassian.net/wiki/x/cgEaCgE|this document>?"
        post_message(ctx, channel, message)


@task
def is_qualification(ctx, release_branch, output=False):
    if qualification_tag_query(ctx, release_branch):
        if output:
            print('true')
        return True
    if output:
        print("false")
    return False


def qualification_tag_query(ctx, release_branch, sort=False):
    with agent_context(ctx, release_branch):
        sort_option = " --sort=-refname" if sort else ""
        res = ctx.run(f"git ls-remote --tags{sort_option} origin '{QUALIFICATION_TAG}-*^{{}}'", hide=True)
        if res.stdout:
            return res.stdout.splitlines()
        return None


@task
def get_qualification_tags(ctx, release_branch, latest_tag=False):
    """Get the qualification tags in remote repository

    Args:
        latest_tag: if True, only return the latest commit and tag
    """
    qualification_tags = qualification_tag_query(ctx, release_branch, sort=True)
    if latest_tag:
        qualification_tags = [qualification_tags[0]]

    return [ref.replace("^{}", "").split("\t") for ref in qualification_tags]


@task
def build_rc(ctx, release_branch, patch_version=False, k8s_deployments=False, start_qual=False):
    """To be done after the PR created by release.create-rc is merged, with the same options
    as release.create-rc.

    Tags the new RC versions on the current commit, and creates the build pipeline for these
    new tags.

    Args:
        k8s_deployments: When set to True the child pipeline deploying to subset of k8s staging clusters will be triggered.
        start_qual: Start the qualification phase for agent 6 release candidates.
    """

    with agent_context(ctx, release_branch):
        datadog_agent = get_gitlab_repo()

        # Get the version of the highest major: needed for tag_version and to know
        # which tag to target when creating the pipeline.
        new_version = next_rc_version(ctx, release_branch, patch_version)

        # Get a string representation of the RC, eg. "6/7.32.0-rc.1"
        versions_string = str(new_version)

        # Step 0: checks

        print(color_message("Checking repository state", "bold"))

        # Check that the base branch is valid
        if not check_base_branch(release_branch, new_version.branch()):
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
        tag_version(ctx, version=str(new_version), force=False, start_qual=start_qual)
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
        run_rc_pipeline(ctx, gitlab_tag.name, k8s_deployments)


def get_qualification_rc_tag(ctx, release_branch):
    with agent_context(ctx, release_branch):
        err_msg = "Error: Expected exactly one release candidate tag associated with the qualification tag commit. Tags found:"
        try:
            latest_commit, _ = get_qualification_tags(ctx, release_branch, latest_tag=True)[0]
            res = ctx.run(f"git tag --points-at {latest_commit} | grep 6.53")
        except Failure as err:
            raise Exit(message=f"{err_msg} []", code=1) from err

        tags = [tag for tag in res.stdout.split("\n") if tag.strip()]
        if len(tags) > 1:
            raise Exit(message=f"{err_msg} {tags}", code=1)
        if not RC_VERSION_RE.match(tags[0]):
            raise Exit(message=f"Error: The tag '{tags[0]}' does not match expected release candidate pattern", code=1)

        return tags[0]


@task
def run_rc_pipeline(ctx, gitlab_tag, k8s_deployments=False):
    run(
        ctx,
        git_ref=gitlab_tag,
        repo_branch="beta",
        deploy=True,
        rc_build=True,
        rc_k8s_deployments=k8s_deployments,
    )


@task
def alert_ci_on_call(ctx, release_branch):
    gitlab_tag = get_qualification_rc_tag(ctx, release_branch)
    message = f":loudspeaker: Agent 6 Update:\nThere is an ongoing Agent 6 release and since there are no new changes there will be no RC bump this week.\n\nPlease rerun the previous build pipeline:\ndda inv release.run-rc-pipeline --gitlab-tag {gitlab_tag}"
    post_message(ctx, "agent-ci-on-call", message)


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
        print(color_message(f"Branching out to {release_branch}", "bold"))
        ctx.run(f"git checkout -b {release_branch}")

        # Step 2 - Push newly created release branch to the remote repository

        print(color_message("Pushing new branch to the upstream repository", "bold"))
        set_gitconfig_in_ci(ctx)
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
        _main()
    else:
        with ctx.cd(f"{base_directory}/{repo}"):
            # Step 1 - Create a local branch out from the default branch
            main_branch = (
                base_branch
                or ctx.run(f"git remote show {upstream} | grep \"HEAD branch\" | sed 's/.*: //'").stdout.strip()
            )
            ctx.run(f"git checkout {main_branch}")
            ctx.run("git pull", warn=True)

            _main()


# TODO: unfreeze is the former name of this task, kept for backward compatibility. Remove in a few weeks.
@task(help={'upstream': "Remote repository name (default 'origin')"}, aliases=["unfreeze"])
def create_release_branches(
    ctx, commit, base_directory="~/dd", major_version: int = 7, upstream="origin", check_state=True
):
    """Create and push release branches in Agent repositories and update them.

    That includes:
        - creates a release branch in datadog-agent, datadog-agent-macos, and omnibus-ruby repositories,
        - updates release.json on new datadog-agent branch to point to newly created release branches
        - updates entries in .gitlab-ci.yml and .gitlab/notify/notify.yml which depend on local branch name

    Args:
        commit: the commit on which the branch should be created (usually the one before the milestone bump)
        base_directory: Path to the directory where dd repos are cloned, defaults to ~/dd, but can be overwritten.
        use_worktree: If True, will go to datadog-agent-worktree instead of datadog-agent.

    Notes:
        This requires a GitHub token (either in the GITHUB_TOKEN environment variable, or in the MacOS keychain),
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

    with agent_context(ctx, commit=commit):
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

        for repo in UNFREEZE_REPOS:
            base_branch = get_default_branch() if major_version == 6 else DEFAULT_BRANCHES[repo]
            create_and_update_release_branch(
                ctx, repo, release_branch, base_branch=base_branch, base_directory=base_directory, upstream=upstream
            )

        # create the backport label in the Agent repo
        print(color_message("Creating backport label in the Agent repository", Color.BOLD))
        github.create_label(
            f'backport/{release_branch}',
            BACKPORT_LABEL_COLOR,
            f'Automatically create a backport PR to {release_branch}',
            exist_ok=True,
        )

        # Step 2 - Create PRs with new settings in datadog-agent repository
        # Step 2.0 - Update release.json
        update_branch = f"{release_branch}-updates"

        ctx.run(f"git checkout {release_branch}")
        ctx.run(f"git checkout -b {update_branch}")

        set_new_release_branch(release_branch)

        # Step 1.2 - In datadog-agent repo update gitlab-ci.yaml
        with open(".gitlab-ci.yml") as f:
            content = f.read()
        with open(".gitlab-ci.yml", "w") as f:
            f.write(
                content.replace(f'COMPARE_TO_BRANCH: {get_default_branch()}', f'COMPARE_TO_BRANCH: {release_branch}')
            )

        # Step 1.3 - Commit new changes
        ctx.run("git add release.json .gitlab-ci.yml")
        ok = try_git_command(ctx, f"git commit -m 'Update release.json, .gitlab-ci.yml with {release_branch}'")
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
            f"[release] Update release.json and .gitlab-ci.yml files for {release_branch} branch",
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
        set_gitconfig_in_ci(ctx)
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
        else:
            omnibus_ruby_branch = base_branch

        def _check_commit_in_repo(repo_name, branch, release_json_field):
            with tempfile.TemporaryDirectory() as tmpdir:
                ctx.run(
                    f'git clone --depth=50 https://github.com/DataDog/{repo_name} --branch {branch} {tmpdir}/{repo_name}',
                    hide='stdout',
                )
                commit = _get_release_json_value(f'{RELEASE_JSON_DEPENDENCIES}::{release_json_field}')
                if ctx.run(f'git -C {tmpdir}/{repo_name} branch --contains {commit}', warn=True, hide=True).exited != 0:
                    raise Exit(
                        code=1,
                        message=f'{repo_name} commit ({commit}) is not in the expected branch ({branch}). The PR is not mergeable',
                    )
                else:
                    print(f'[nightly] Commit {commit} was found in {repo_name} branch {branch}')

        _check_commit_in_repo('omnibus-ruby', omnibus_ruby_branch, 'OMNIBUS_RUBY_VERSION')

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
                "No Atlassian credentials provided. Run dda inv --help update-build-links for more details.",
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
def generate_release_metrics(ctx, milestone, cutoff_date, release_date):
    """Task to run after the release is done to generate release metrics.

    Args:
        milestone: Github milestone number for the release. Expected format like '7.54.0'
        cutoff_date: Date when the code cutoff was started. Expected format YYYY-MM-DD, like '2022-02-01'
        release_date: Date when the release was done. Expected format YYYY-MM-DD, like '2022-09-15'

    Notes:
        Results are formatted in a way that can be easily copied to https://docs.google.com/spreadsheets/d/1r39CtyuvoznIDx1JhhLHQeAzmJB182n7ln8nToiWQ8s/edit#gid=1490566519
        Copy paste numbers to the respective sheets and select 'Split text to columns'.
    """

    # Step 1: Lead Time for Changes data
    lead_time = get_release_lead_time(cutoff_date, release_date)
    print("Lead Time for Changes data")
    print("--------------------------")
    print(lead_time)

    # Step 2: Agent stability data
    prs = get_prs_metrics(milestone, cutoff_date)
    print("\n")
    print("Agent stability data: Pull Requests")
    print("--------------------")

    print(
        f"total: {prs['total']}, before_cutoff: {prs['before_cutoff']}, on_cutoff: {prs['on_cutoff']}, after_cutoff: {prs['after_cutoff']}"
    )

    # Step 3: Code changes
    code_stats = ctx.run(
        f"git log --shortstat {milestone}-devel..{milestone} | grep \"files changed\" | awk '{{files+=$1; inserted+=$4; deleted+=$6}} END {{print files,\",\", inserted,\",\", deleted}}'",
        hide=True,
    ).stdout.strip()
    print("\n")
    print("Code changes")
    print("------------")
    print(code_stats)


@task
def create_schedule(_, version, cutoff_date):
    """Create confluence pages for the release schedule.

    Args:
        cutoff_date: Date when the code cut-off happened. Expected format YYYY-MM-DD, like '2022-02-01'
    """

    required_environment_variables = ["ATLASSIAN_USERNAME", "ATLASSIAN_PASSWORD"]
    if not all(key in os.environ for key in required_environment_variables):
        raise Exit(f"You must set {required_environment_variables} environment variables to use this task.", code=1)
    release_page = create_release_page(version, date.fromisoformat(cutoff_date))
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

    client = WebClient(os.environ["SLACK_DATADOG_AGENT_BOT_TOKEN"])
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
    client = WebClient(os.environ["SLACK_DATADOG_AGENT_BOT_TOKEN"])
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
        next_version = next_rc_version(ctx, release_branch)
        repo_data = generate_repo_data(ctx, warning_mode, next_version, release_branch)
        changes = 'false'
        message = [f":warning: Please add the `{next_version}` tag on the head of `{release_branch}` for:\n"]
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
                        message.append(
                            f" - <https://github.com/DataDog/{repo_name}/commits/{release_branch}/|{repo_name}>\n"
                        )
                # This repo has changes, the next check is not needed
                continue
            if repo_name != "datadog-agent" and last_tag_name != repo['previous_tag']:
                changes = 'true'
                print(
                    f"{repo_name} has a new tag {last_tag_name} since last release candidate (was {repo['previous_tag']})",
                    file=sys.stderr,
                )
        # Notify the release manager if there are changes
        if len(message) > 1:
            message.append("Make sure to tag them before merging the next RC PR.")
            warn_new_tags("".join(message))
        # Send a value for the create_rc_pr.yml workflow
        print(changes)


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

    gh = GithubAPI()

    current = current_version(ctx, major_version)
    next = current.next_version(bump_minor=True)
    next.devel = False

    print(f"Creating the {next} milestone...")
    gh.create_milestone(str(next), exist_ok=True)

    with agent_context(ctx, get_default_branch(major=major_version)):
        milestone_branch = f"release_milestone-{int(time.time())}"
        ctx.run(f"git switch -c {milestone_branch}")
        set_current_milestone(str(next))
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


@task
def check_previous_agent6_rc(ctx):
    """
    Validates that there are no existing Agent 6 release candidate pull requests
    and checks if an Agent 6 build pipeline has been run in the past week
    """
    err_msg = ""
    agent6_prs = ""
    github = GithubAPI()
    prs = github.get_pr_for_branch(None, "6.53.x")
    for pr in prs:
        if "Update release.json and Go modules for 6.53" in pr.title and not pr.draft:
            agent6_prs += f"\n- {pr.title}: https://github.com/DataDog/datadog-agent/pull/{pr.number}"
    if agent6_prs:
        err_msg += "AGENT 6 ERROR: The following Agent 6 release candidate PRs already exist. Please address these PRs before creating a new release candidate"
        err_msg += agent6_prs

    response = get_ci_pipeline_events(
        'ci_level:pipeline @ci.pipeline.name:"DataDog/datadog-agent" @git.tag:6.53.* -@ci.pipeline.downstream:true -@ci.partial_pipeline:retry',
        7,
    )
    if not response.data:
        err_msg += "\nAGENT 6 ERROR: No Agent 6 build pipelines have run in the past week. Please trigger a build pipeline for the next agent 6 release candidate."

    if err_msg:
        post_message(ctx, "agent-ci-on-call", err_msg)
        raise Exit(message=err_msg, code=1)
