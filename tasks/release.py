"""Release helper tasks

Notes about Agent6:
    Release tasks should be run from the main branch.
    To make a task compatible with agent 6, it is possible to use agent_context such that
    the task will be run in the agent6 branch.
"""

import os
import sys
import tempfile
import time
from collections import defaultdict
from datetime import datetime
from time import sleep

from gitlab import GitlabError
from invoke import Failure, task
from invoke.exceptions import Exit

from tasks.go import tidy
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
    list_not_closed_qa_cards,
)
from tasks.libs.releasing.json import (
    DEFAULT_BRANCHES,
    DEFAULT_BRANCHES_AGENT6,
    _get_release_json_value,
    _save_release_json,
    generate_repo_data,
    get_current_milestone,
    load_release_json,
    set_current_milestone,
    update_release_json,
)
from tasks.libs.releasing.notes import _add_dca_prelude, _add_prelude
from tasks.libs.releasing.version import (
    FINAL_VERSION_RE,
    RC_VERSION_RE,
    RELEASE_JSON_DEPENDENCIES,
    VERSION_RE,
    deduce_version,
    get_version_major,
    next_final_version,
    next_rc_version,
)
from tasks.notify import post_message
from tasks.pipeline import run
from tasks.release_metrics.metrics import get_prs_metrics, get_release_lead_time

QUALIFICATION_TAG = "qualification"


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
            for dependency in module.dependencies(ctx):
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
def finish(ctx, release_branch, upstream="origin", release_date=None):
    """Updates the release.json file for the new version.

    Args:
        release_branch: The Git branch from which the release is being finalized.
            This branch should correspond to the release line you are finishing
            (for example, "7.69.x"). It is used to determine the major version,
            update module dependencies, and generate the release artifacts.
        release_date: Date when the release was done. Expected format YYYY-MM-DD,
            like '2025-09-03'. (default: today's date)
        upstream: The name of the remote repository to push the finalized release
            branch to. This is typically "origin", but can be changed if working
            with a fork or a differently named remote. (default: "origin")

    Updates internal module dependencies with the new version.
    """
    # Step 1: Preparation

    # Validate release_date (if provided)
    if release_date:
        try:
            datetime.strptime(release_date, "%Y-%m-%d")
        except ValueError as err:
            raise Exit(
                color_message(f"Invalid date `{release_date}`. Date should be valid and in format YYYY-MM-DD.", "red"),
                code=1,
            ) from err

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
        _add_prelude(ctx, str(new_version), release_date)

        print(color_message("Adding DCA release changelog prelude", "bold"))
        _add_dca_prelude(ctx, str(new_version), release_date)

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

        # Step 3: Run tidy task
        print(color_message("Running `dda inv tidy`", "bold"))
        tidy(ctx)

        # Step 4: branch out, push branch, then add, and create signed commit with Github API
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

        # Step 5 - Send a slack message
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
def build_rc(ctx, release_branch, patch_version=False, start_qual=False):
    """To be done after the PR created by release.create-rc is merged, with the same options
    as release.create-rc.

    Tags the new RC versions on the current commit, and creates the build pipeline for these
    new tags.
    Staging k8s deployment PR will be created during the build pipeline.

    Args:
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

        # tag_version only takes the highest version (Agent 7 currently), and creates
        # the tags for all supported versions
        # TODO(team:agent-delivery): make it possible to do Agent 6-only or Agent 7-only tags?
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
        run_rc_pipeline(ctx, gitlab_tag.name)


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
def run_rc_pipeline(ctx, gitlab_tag):
    run(
        ctx,
        git_ref=gitlab_tag,
        repo_branch="beta",
        deploy=True,
        rc_build=True,
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
        for idx, key in enumerate(path):
            if key not in current_node:
                raise Exit(code=1, message=f"Couldn't find '{key}' in release.json")
            if idx == len(path) - 1:
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
        try:
            team = next(team for team, jira_project in GITHUB_JIRA_MAP.items() if project == jira_project)
        except StopIteration:
            client.chat_postMessage(
                channel="#agent-devx-ops",
                text=f"Issue in qa_card chase, no team found for project {project} for cards {', '.join([card['key'] for card in cards])}",
            )
            print(f"No team found for project {project}")
            continue
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
        return_code = 0  # no changes
        message = [f":warning: Please add the `{next_version}` tag on the head of `{release_branch}` for:\n"]
        for repo_name, repo in repo_data.items():
            head_commit = get_last_commit(ctx, repo_name, repo['branch'])
            last_tag_commit, last_tag_name = get_last_release_tag(ctx, repo_name, next_version.tag_pattern())
            if last_tag_commit != "" and last_tag_commit != head_commit:
                return_code = 69
                print(f"{repo_name} has new commits since {last_tag_name}", file=sys.stderr)
                if warning_mode:
                    team = "agent-integrations"
                    warn_new_commits(team, repo['branch'], next_version)
                else:
                    if repo_name not in ["datadog-agent", "integrations-core"]:
                        message.append(
                            f" - <https://github.com/DataDog/{repo_name}/commits/{release_branch}/|{repo_name}>\n"
                        )
                # This repo has changes, the next check is not needed
                continue
            if repo_name != "datadog-agent" and last_tag_name != repo['previous_tag']:
                return_code = 69
                print(
                    f"{repo_name} has a new tag {last_tag_name} since last release candidate (was {repo['previous_tag']})",
                    file=sys.stderr,
                )
        # Notify the release manager if there are changes
        if len(message) > 1:
            message.append("Make sure to tag them before merging the next RC PR.")
            warn_new_tags("".join(message))
        # Send a value for the create_rc_pr.yml workflow
        sys.exit(return_code)


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

    with agent_context(ctx, release_branch):
        # Fetch tags in the worktree so deduce_version can find them
        ctx.run("git fetch origin --tags", hide=True)
        version = deduce_version(ctx, release_branch, next_version=False)
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
