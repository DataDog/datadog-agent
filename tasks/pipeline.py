import os
import re
import sys
import time
from datetime import datetime, timedelta, timezone

import yaml
from gitlab import GitlabError
from gitlab.v4.objects import Project
from invoke import task
from invoke.exceptions import Exit

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.ciproviders.gitlab_api import (
    cancel_pipeline,
    get_gitlab_repo,
    gitlab_configuration_is_modified,
    refresh_pipeline,
)
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.git import get_commit_sha, get_current_branch, get_default_branch
from tasks.libs.common.utils import (
    get_all_allowed_repo_branches,
    is_allowed_repo_branch,
)
from tasks.libs.owners.parsing import read_owners
from tasks.libs.pipeline.tools import (
    FilteredOutException,
    cancel_pipelines_with_confirmation,
    get_running_pipelines_on_same_ref,
    gracefully_cancel_pipeline,
    trigger_agent_pipeline,
    wait_for_pipeline,
)

BOT_NAME = "github-actions[bot]"


# Tasks to trigger pipelines


def check_deploy_pipeline(repo_branch):
    """
    Run checks to verify a deploy pipeline is valid (it targets a valid repo branch)
    """

    # Check that the target repo branch is valid
    if not is_allowed_repo_branch(repo_branch):
        print(
            f"--repo-branch argument '{repo_branch}' is not in the list of allowed repository branches: {get_all_allowed_repo_branches()}"
        )
        raise Exit(code=1)


@task
def clean_running_pipelines(ctx, git_ref=None, here=False, use_latest_sha=False, sha=None):
    """
    Fetch running pipelines on a target ref (+ optionally a git sha), and ask the user if they
    should be cancelled.
    """

    agent = get_gitlab_repo()

    if here:
        git_ref = get_current_branch(ctx)
    else:
        git_ref = git_ref or get_default_branch()

    print(f"Fetching running pipelines on {git_ref}")

    if not sha and use_latest_sha:
        sha = get_commit_sha(ctx, git_ref)
        print(f"Git sha not provided, using the one {git_ref} currently points to: {sha}")
    elif not sha:
        print(f"Git sha not provided, fetching all running pipelines on {git_ref}")

    pipelines = get_running_pipelines_on_same_ref(agent, git_ref, sha)

    print(
        f"Found {len(pipelines)} running pipeline(s) matching the request.",
        "They are ordered from the newest one to the oldest one.\n",
        sep='\n',
    )
    cancel_pipelines_with_confirmation(agent, pipelines)


def workflow_rules(gitlab_file=".gitlab-ci.yml"):
    """Get Gitlab workflow rules list in a YAML-formatted string."""
    with open(gitlab_file) as f:
        return yaml.dump(yaml.safe_load(f.read())["workflow"]["rules"])


@task
def auto_cancel_previous_pipelines(ctx):
    """
    Automatically cancel previous pipelines running on the same ref
    """

    git_ref = os.environ["CI_COMMIT_REF_NAME"]
    if git_ref == "":
        raise Exit("CI_COMMIT_REF_NAME is empty, skipping pipeline cancellation", 0)

    git_sha = os.getenv("CI_COMMIT_SHA")

    repo = get_gitlab_repo()
    pipelines = get_running_pipelines_on_same_ref(repo, git_ref)
    pipelines_without_current = [p for p in pipelines if p.sha != git_sha]
    force_cancel_stages = [
        "package_build",
        # We want to cancel all KMT jobs to ensure proper cleanup of the KMT EC2 instances.
        # If some jobs are canceled, the cleanup jobs will not run automatically, which means
        # we must trigger the manual cleanup jobs (done in gracefully_cancel_pipeline) below.
        # But if we trigger the cleanup manually and there are test jobs still running, those
        # will be practically canceled (the instance they run on gets shut down) but will appear
        # as a fail in the pipeline.
        "kernel_matrix_testing_prepare",
        "kernel_matrix_testing_system_probe",
        "kernel_matrix_testing_security_agent",
    ]

    for pipeline in pipelines_without_current:
        # We cancel pipeline only if it correspond to a commit that is an ancestor of the current commit
        is_ancestor = ctx.run(f'git merge-base --is-ancestor {pipeline.sha} {git_sha}', warn=True, hide="both")
        if is_ancestor.exited == 0:
            print(f'Gracefully canceling jobs that are not canceled on pipeline {pipeline.id} ({pipeline.web_url})')
            gracefully_cancel_pipeline(repo, pipeline, force_cancel_stages=force_cancel_stages)
        elif is_ancestor.exited == 1:
            print(f'{pipeline.sha} is not an ancestor of {git_sha}, not cancelling pipeline {pipeline.id}')
        elif is_ancestor.exited == 128:
            min_time_before_cancel = 5
            print(
                f'Could not determine if {pipeline.sha} is an ancestor of {git_sha}, probably because it has been deleted from the history because of force push'
            )
            if datetime.strptime(pipeline.created_at, "%Y-%m-%dT%H:%M:%S.%fZ") < datetime.now() - timedelta(
                minutes=min_time_before_cancel
            ):
                print(
                    f'Pipeline started earlier than {min_time_before_cancel} minutes ago, gracefully canceling pipeline {pipeline.id}'
                )
                gracefully_cancel_pipeline(repo, pipeline, force_cancel_stages=force_cancel_stages)
        else:
            print(is_ancestor.stderr)
            raise Exit(code=1)


@task
def run(
    ctx,
    git_ref="",
    here=False,
    repo_branch="dev",
    deploy=False,
    deploy_installer=False,
    all_builds=True,
    e2e_tests=True,
    kmt_tests=True,
    rc_build=False,
    run_flaky_tests=False,
):
    """
    Run a pipeline on the given git ref (--git-ref <git ref>), or on the current branch if --here is given.
    By default, this pipeline will run all builds & tests, including all kmt and e2e tests, but is not a deploy pipeline.
    Use --deploy to make this pipeline a deploy pipeline for the agent, which will upload artifacts to the staging repositories.
    Use --deploy-installer to make this pipeline a deploy pipeline for the installer, which will upload artifacts to the staging repositories.
    Use --no-all-builds to not run builds for all architectures (only a subset of jobs will run. No effect on pipelines on the default branch).
    Use --no-kmt-tests to not run all Kernel Matrix Tests on the pipeline.
    Use --e2e-tests to run all e2e tests on the pipeline.
    Use --run-flaky-tests to run tests that are marked as flaky (by default, known flaky tests are skipped).

    Release Candidate related flags:
    Use --rc-build to mark the build as Release Candidate. Staging k8s deployment PR will be created during the build pipeline.

    By default, the pipeline builds both Agent 6 and Agent 7.
    Use the --major-versions option to specify a comma-separated string of the major Agent versions to build
    (eg. '6' to build Agent 6 only, '6,7' to build both Agent 6 and Agent 7).

    The --repo-branch option indicates which branch of the staging repository the packages will be deployed to (useful only on deploy pipelines).

    If other pipelines are already running on the git ref, the script will prompt the user to confirm if these previous
    pipelines should be cancelled.

    Examples
    Run a pipeline on my-branch:
      dda inv pipeline.run --git-ref my-branch

    Run a pipeline on the current branch:
      dda inv pipeline.run --here

    Run a pipeline without Kernel Matrix Tests on the current branch:
      dda inv pipeline.run --here --no-kmt-tests

    Run a pipeline with e2e tets on the current branch:
      dda inv pipeline.run --here --e2e-tests

    Run a pipeline that includes flaky tests on the current branch:
      dda inv pipeline.run --here --run-flaky-tests

    Run a deploy pipeline on the 7.32.0 tag, uploading the artifacts to the stable branch of the staging repositories:
      dda inv pipeline.run --deploy --major-versions "6,7" --git-ref "7.32.0" --repo-branch "stable"
    """

    repo = get_gitlab_repo()

    if (git_ref == "" and not here) or (git_ref != "" and here):
        raise Exit("ERROR: Exactly one of --here or --git-ref <git ref> must be specified.", code=1)

    if here:
        git_ref = get_current_branch(ctx)

    if deploy or deploy_installer:
        # Check the validity of the deploy pipeline
        check_deploy_pipeline(repo_branch)
        # Force all builds and e2e tests to be run
        if not all_builds:
            print(
                color_message(
                    "WARNING: ignoring --no-all-builds option, RUN_ALL_BUILDS is automatically set to true on deploy pipelines",
                    "orange",
                )
            )
            all_builds = True
        if not e2e_tests:
            print(
                color_message(
                    "WARNING: ignoring --no-e2e-tests option, RUN_E2E_TESTS is automatically set to true on deploy pipelines",
                    "orange",
                )
            )
            e2e_tests = True

    pipelines = get_running_pipelines_on_same_ref(repo, git_ref)

    if pipelines:
        print(
            f"There are already {len(pipelines)} pipeline(s) running on the target git ref.",
            "For each of them, you'll be asked whether you want to cancel them or not.",
            "If you don't need these pipelines, please cancel them to save CI resources.",
            "They are ordered from the newest one to the oldest one.\n",
            sep='\n',
        )
        cancel_pipelines_with_confirmation(repo, pipelines)

    try:
        pipeline = trigger_agent_pipeline(
            repo,
            git_ref,
            repo_branch,
            deploy=deploy,
            deploy_installer=deploy_installer,
            all_builds=all_builds,
            e2e_tests=e2e_tests,
            kmt_tests=kmt_tests,
            rc_build=rc_build,
            run_flaky_tests=run_flaky_tests,
        )
    except FilteredOutException:
        print(color_message(f"ERROR: pipeline does not match any workflow rule. Rules:\n{workflow_rules()}", "red"))
        return

    wait_for_pipeline(repo, pipeline)


@task
def follow(ctx, id=None, git_ref=None, here=False, project_name="DataDog/datadog-agent"):
    """
    Follow a pipeline's progress in the CLI.
    Use --here to follow the latest pipeline on your current branch.
    Use --git-ref to follow the latest pipeline on a given tag or branch.
    Use --id to follow a specific pipeline.
    Use --project-name to specify a repo other than DataDog/datadog-agent (default)

    Examples:
    dda inv pipeline.follow --git-ref my-branch
    dda inv pipeline.follow --here
    dda inv pipeline.follow --id 1234567
    """

    repo = get_gitlab_repo(project_name)

    args_given = 0
    if id is not None:
        args_given += 1
    if git_ref is not None:
        args_given += 1
    if here:
        args_given += 1
    if args_given != 1:
        raise Exit(
            "ERROR: Exactly one of --here, --git-ref or --id must be given.\nSee --help for an explanation of each.",
            code=1,
        )

    if id is not None:
        pipeline = repo.pipelines.get(id)
        wait_for_pipeline(repo, pipeline)
    elif git_ref is not None:
        wait_for_pipeline_from_ref(repo, git_ref)
    elif here:
        git_ref = get_current_branch(ctx)
        wait_for_pipeline_from_ref(repo, git_ref)


def wait_for_pipeline_from_ref(repo: Project, ref):
    # Get last updated pipeline
    pipelines = repo.pipelines.list(ref=ref, per_page=1, order_by='updated_at')
    if len(pipelines) == 0:
        print(f"No pipelines found for {ref}")
        raise Exit(code=1)

    pipeline = pipelines[0]
    wait_for_pipeline(repo, pipeline)


@task(iterable=['variable'])
def trigger_child_pipeline(_, git_ref, project_name, variable=None, follow=True, timeout=7200):
    """
    Trigger a child pipeline on a target repository and git ref.
    Used in CI jobs only (requires CI_JOB_TOKEN).

    Use --variable to specify the environment variables that should be passed to the child pipeline.
    You can pass the argument multiple times for each new variable you wish to forward

    Use --follow to make this task wait for the pipeline to finish, and return 1 if it fails.

    Use --timeout to set up a timeout shorter than the default 2 hours, to anticipate failures if any.

    Examples:
    dda inv pipeline.trigger-child-pipeline --git-ref "main" --project-name "DataDog/agent-release-management" --variable "RELEASE_VERSION"

    dda inv pipeline.trigger-child-pipeline --git-ref "main" --project-name "DataDog/agent-release-management" --variable "VAR1" --variable "VAR2" --variable "VAR3"
    """

    if not os.environ.get('CI_JOB_TOKEN'):
        raise Exit("CI_JOB_TOKEN variable needed to create child pipelines.", 1)

    # Use the CI_JOB_TOKEN which is passed from gitlab
    token = None if follow else os.environ['CI_JOB_TOKEN']
    repo = get_gitlab_repo(project_name, token=token)

    # Fill the environment variables to pass to the child pipeline.
    variables = {}
    if variable:
        for v in variable:
            # An empty string or a terminal ',' will yield an empty string which
            # we don't need to bother with
            if not v:
                continue
            if v not in os.environ:
                print(
                    color_message(
                        f"WARNING: attempting to pass undefined variable \"{v}\" to downstream pipeline", "orange"
                    )
                )
                continue
            variables[v] = os.environ[v]

    print(
        f"Creating child pipeline in repo {project_name}, on git ref {git_ref} with params: {variables}",
        flush=True,
    )

    try:
        pipeline = repo.trigger_pipeline(git_ref, os.environ['CI_JOB_TOKEN'], variables=variables)
    except GitlabError as e:
        raise Exit(f"Failed to create child pipeline: {e}", code=1) from e

    print(f"Created a child pipeline with id={pipeline.id}, url={pipeline.web_url}", flush=True)

    if follow:
        print("Waiting for child pipeline to finish...", flush=True)
        wait_for_pipeline(repo, pipeline, pipeline_finish_timeout_sec=timeout)

        # Check pipeline status
        refresh_pipeline(pipeline)
        pipestatus = pipeline.status.lower().strip()

        if pipestatus != "success":
            raise Exit(f"Error: child pipeline status {pipestatus.title()}", code=1)

        print("Child pipeline finished successfully", flush=True)


def parse(commit_str):
    lines = commit_str.split("\n")
    title = lines[0]
    url = ""
    pr_id_match = re.search(r".*\(#(\d+)\)", title)
    if pr_id_match is not None:
        url = f"https://github.com/DataDog/datadog-agent/pull/{pr_id_match.group(1)}"
    author = lines[1]
    author_email = lines[2]
    files = lines[3:]
    return title, author, author_email, files, url


def is_system_probe(owners, files):
    target = {
        ("TEAM", "@DataDog/universal-service-monitoring"),
        ("TEAM", "@DataDog/ebpf-platform"),
        ("TEAM", "@DataDog/agent-security"),
        ("TEAM", "@DataDog/cloud-network-monitoring"),
        ("TEAM", "@DataDog/network-path"),
        ("TEAM", "@DataDog/debugger-go"),
    }
    for f in files:
        match_teams = set(owners.of(f)) & target
        if len(match_teams) != 0:
            return True

    return False


EMAIL_SLACK_ID_MAP = {
    "guy20495@gmail.com": "U03LJSCAPK2",
    "safchain@gmail.com": "U01009CUG9X",
    "usamasaqib.96@live.com": "U03D807V94J",
}
GITHUB_SLACK_ID_MAP = {
    "gjulianm": "U069N8XCSQ0",
    "jmw51798": "U05FPPV9ASF",
}


@task
def changelog(ctx, new_commit_sha):
    from slack_sdk import WebClient
    from slack_sdk.errors import SlackApiError

    client = WebClient(token=os.environ["SLACK_DATADOG_AGENT_BOT_TOKEN"])
    # Environment variable to deal with both local and CI environments
    if "CI_PROJECT_DIR" in os.environ:
        parent_dir = os.environ["CI_PROJECT_DIR"]
    else:
        parent_dir = os.getcwd()
    old_commit_sha = ctx.run(
        f"{parent_dir}/tools/ci/fetch_secret.sh {os.environ['CHANGELOG_COMMIT_SHA']}",
        hide=True,
    ).stdout.strip()
    if not new_commit_sha:
        print("New commit sha not found, exiting")
        return
    if not old_commit_sha:
        print("Old commit sha not found, exiting")
        return

    commit_range_link = f"https://github.com/DataDog/datadog-agent/compare/{old_commit_sha}..{new_commit_sha}"
    empty_changelog_msg = "No new System Probe related commits in this release :cricket:"
    no_commits_msg = "No new commits in this release :cricket:"
    slack_message = (
        f"The nightly deployment is rolling out to Staging :siren: \n"
        f"Changelog for <{commit_range_link}|commit range>: `{old_commit_sha}` to `{new_commit_sha}`:\n"
    )

    if old_commit_sha == new_commit_sha:
        print("No new commits found, exiting")
        slack_message += no_commits_msg
        client.chat_postMessage(channel="system-probe-ops", text=slack_message)
        return

    print(f"Generating changelog for commit range {old_commit_sha} to {new_commit_sha}")
    commits = ctx.run(f"git log {old_commit_sha}..{new_commit_sha} --pretty=format:%h", hide=True).stdout.split("\n")
    owners = read_owners(".github/CODEOWNERS")
    messages = []

    for commit in commits:
        # see https://git-scm.com/docs/pretty-formats for format string
        commit_str = ctx.run(f"git show --name-only --pretty=format:%s%n%aN%n%aE {commit}", hide=True).stdout
        title, _, author_email, files, url = parse(commit_str)
        if not is_system_probe(owners, files):
            continue
        message_link = f"• <{url}|{title}>" if url else f"• {title}"
        if "dependabot" in author_email or "github-actions" in author_email:
            messages.append(f"{message_link}")
            continue
        author_handle = ""
        if author_email in EMAIL_SLACK_ID_MAP:
            author_handle = EMAIL_SLACK_ID_MAP[author_email]
        elif author_email.endswith("@users.noreply.github.com"):
            github_username = author_email.removesuffix("@users.noreply.github.com")
            if "+" in github_username:
                github_username = github_username.split("+", maxsplit=1)[1]
            if github_username in GITHUB_SLACK_ID_MAP:
                author_handle = GITHUB_SLACK_ID_MAP[github_username]
        else:
            try:
                recipient = client.users_lookupByEmail(email=author_email)
                author_handle = recipient.data["user"]["id"]
            except SlackApiError:
                # The email on the Github account is not a datadoghhq.com address, it cannot be decoded by slack.
                pass
        if author_handle:
            author_handle = f"<@{author_handle}>"
        else:
            author_handle = author_email
        time.sleep(1)  # necessary to prevent slack/sdm API rate limits
        messages.append(f"{message_link} {author_handle}")

    if messages:
        slack_message += "\n".join(messages)
        slack_message += "\n:wave: Authors, please check the <https://ddstaging.datadoghq.com/dashboard/kfn-zy2-t98?"
        clusters = ["chillpenguin", "diglet", "lagaffe", "muk", "oddish-b", "snowver", "stripe", "venomoth"]
        for i, cluster_name in enumerate(clusters):
            slack_message += f"{"&" if i > 0 else ""}tpl_var_kube_cluster_name%5B{i}%5D={cluster_name}"
        slack_message += "|dashboard> for issues"
    else:
        slack_message += empty_changelog_msg

    print(f"Posting message to slack: \n {slack_message}")
    client.chat_postMessage(channel="system-probe-ops", text=slack_message)
    print(f"Writing new commit sha: {new_commit_sha} to SSM")
    res = ctx.run(
        f"aws ssm put-parameter --name ci.datadog-agent.gitlab_changelog_commit_sha --value {new_commit_sha} "
        "--type \"SecureString\" --region us-east-1 --overwrite",
        hide=True,
    )
    if "unable to locate credentials" in res.stderr.casefold():
        raise Exit("Permanent error: unable to locate credentials, retry the job", code=42)


@task(
    help={
        "image_tag": "tag from build_image with format v<build_id>_<commit_id>",
        "test_version": "Is a test image or not",
        "branch_name": "If you already committed in a local branch",
    }
)
def update_buildimages(ctx, image_tag, test_version=True, branch_name=None):
    """
    Update local files to run with new image_tag from agent-buildimages and launch a full pipeline
    Use --no-test-version to commit without the _test_only suffixes
    """
    raise Exit(
        f"This invoke task is {color_message('deprecated', 'red')}, please use `dda inv buildimages.update` instead."
    )


@task(
    help={
        'owner-branch-name': 'Owner and branch names in the format <owner-name>/<branch-name>',
        'no-verify': 'Adds --no-verify flag when git push',
    }
)
def trigger_external(ctx, owner_branch_name: str, no_verify=False):
    """
    Trigger a pipeline from an external owner.
    """

    branch_re = re.compile(r'^(?P<owner>[a-zA-Z0-9_-]+):(?P<branch_name>[a-zA-Z0-9#_/-]+)$')
    match = branch_re.match(owner_branch_name)

    assert (
        match is not None
    ), f'owner_branch_name should be "<owner-name>:<prefix>/<branch-name>" or "<owner-name>:<branch-name>" but is {owner_branch_name}'
    assert "'" not in owner_branch_name

    owner, branch = match.group('owner'), match.group('branch_name')
    no_verify_flag = ' --no-verify' if no_verify else ''

    # Can checkout
    status_res = ctx.run('git status --porcelain')
    assert status_res.stdout.strip() == '', 'Cannot run this task if changes have not been committed'
    branch_res = ctx.run('git branch', hide='stdout')
    assert (
        re.findall(f'\\b{owner_branch_name}\\b', branch_res.stdout) == []
    ), f'{owner_branch_name} branch already exists'
    remote_res = ctx.run('git remote', hide='stdout')
    assert re.findall(f'\\b{owner}\\b', remote_res.stdout) == [], f'{owner} remote already exists'

    # Get current branch
    curr_branch_res = ctx.run('git branch --show-current', hide='stdout')
    curr_branch = curr_branch_res.stdout.strip()

    # Commands to restore current state
    restore_commands = [
        f"git remote remove '{owner}'",
        f"git checkout '{curr_branch}'",
        f"git branch -d '{owner}/{branch}'",
    ]

    # Get the correct fork name
    gh = GithubAPI()
    fork_name = gh.get_fork_name(owner)
    # Commands to push the branch
    commands = (
        [
            # Fetch
            f"git remote add {owner} git@github.com:{owner}/{fork_name}.git",
            f"git fetch '{owner}' '{branch}'",
            # Create branch
            f"git checkout '{owner}/{branch}'",  # This first checkout puts us in a detached head state, thus the second checkout below
            f"git checkout -b '{owner}/{branch}'",
            # Push
            f"git push --set-upstream origin '{owner}/{branch}'{no_verify_flag}",
        ]
        + restore_commands
    )

    # Run commands then restore commands
    ret_code = 0
    for command in commands:
        ret_code = ctx.run(command, warn=True, echo=True).exited
        if ret_code != 0:
            print('The last command exited with code', ret_code)
            print('You might want to run these commands to restore the current state:')
            print('\n'.join(restore_commands))

            raise Exit(code=1)

    # Show links
    repo = f'https://github.com/DataDog/datadog-agent/tree/{owner}/{branch}'
    pipeline = f'https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Apipeline%20%40ci.provider.name%3Agitlab%20%40git.repository.name%3A%22DataDog%2Fdatadog-agent%22%20%40git.branch%3A%22{owner}%2F{branch}%22&colorBy=meta%5B%27ci.stage.name%27%5D&colorByAttr=meta%5B%27ci.stage.name%27%5D&currentTab=json&fromUser=false&index=cipipeline&sort=time&spanViewType=logs'

    print(f'\nBranch {owner}/{branch} pushed to repo: {repo}')
    ctx.run(f"ddr devflow ddci trigger --owner DataDog --repository datadog-agent --branch {owner}/{branch}")
    print(f'CI-Visibility pipeline link: {pipeline}')


@task
def test_merge_queue(ctx):
    """
    Test the pipeline in merge-queue context:
      - Create a temporary copy of main branch
      - Create a PR of current branch against this copy
      - Trigger the merge queue
      - Check if the pipeline is correctly created
    """
    # Create a new main and push it
    print("Creating a new main branch")
    timestamp = int(datetime.now(timezone.utc).timestamp())
    test_default = f"mq/test_{timestamp}"
    current_branch = get_current_branch(ctx)
    ctx.run(f"git checkout {get_default_branch()}", hide=True)
    ctx.run("git pull", hide=True)
    ctx.run(f"git checkout -b {test_default}", hide=True)
    ctx.run(f"git push origin {test_default}", hide=True)
    # Create a PR towards this new branch and adds it to the merge queue
    print("Creating a PR and adding it to the merge queue")
    gh = GithubAPI()
    pr = gh.create_pr(f"Test MQ for {current_branch}", "", test_default, current_branch)
    pr.create_issue_comment("/merge")
    # Search for the generated pipeline
    print(f"PR {pr.html_url} is waiting for MQ pipeline generation")
    agent = get_gitlab_repo()
    max_attempts = 5
    for attempt in range(max_attempts):
        time.sleep(30)
        pipelines = agent.pipelines.list(per_page=100)
        try:
            pipeline = next(p for p in pipelines if p.ref.startswith(f"mq-working-branch-{test_default}"))
            print(f"Pipeline found: {pipeline.web_url}")
            break
        except StopIteration as e:
            if attempt == max_attempts - 1:
                raise RuntimeError("No pipeline found for the merge queue") from e
            continue
    success = pipeline.status == "running"
    if success:
        print("Pipeline correctly created, congrats")
    else:
        print(f"[ERROR] Impossible to generate a pipeline for the merge queue, please check {pipeline.web_url}")
    # Clean up
    print("Cleaning up")
    if success:
        cancel_pipeline(pipeline)
    pr.edit(state="closed")
    ctx.run(f"git checkout {current_branch}", hide=True)
    ctx.run(f"git branch -D {test_default}", hide=True)
    ctx.run(f"git push origin :{test_default}", hide=True)
    if not success:
        raise Exit(message="Merge queue test failed", code=1)


@task
def compare_to_itself(ctx):
    """
    Create a new branch with 'compare_to_itself' in gitlab-ci.yml and trigger a pipeline.
    This is used to verify that the gitlab ci is not broken by the changes when merged on the base branch.
    """
    if not gitlab_configuration_is_modified(ctx):
        print("No modification in the gitlab configuration, ignoring this test.")
        return
    agent = get_gitlab_repo()
    gh = GithubAPI()
    current_branch = os.environ["CI_COMMIT_REF_NAME"]
    if current_branch.startswith("compare/"):
        print("Branch already in compare_to_itself mode, ignoring this test to prevent infinite loop")
        return
    new_branch = f"compare/{current_branch}/{int(datetime.now(timezone.utc).timestamp())}"
    ctx.run(f"git checkout -b {new_branch}", hide=True)
    ctx.run(
        f"git remote set-url origin https://x-access-token:{gh._auth.token}@github.com/DataDog/datadog-agent.git",
        hide=True,
    )
    ctx.run(f"git config --global user.name '{BOT_NAME}'", hide=True)
    ctx.run("git config --global user.email 'github-app[bot]@users.noreply.github.com'", hide=True)
    # The branch must exist in gitlab to be able to "compare_to"
    # Push an empty commit to prevent linking this pipeline to the actual PR
    ctx.run("git commit -m 'Initial push of the compare/to branch' --allow-empty", hide=True)
    ctx.run(f"git push origin {new_branch}")

    from tasks.libs.releasing.json import load_release_json

    release_json = load_release_json()

    with open('.gitlab-ci.yml', 'r+') as f:
        content = f.read()
        f.write(
            content.replace(f'COMPARE_TO_BRANCH: {release_json["base_branch"]}', f'COMPARE_TO_BRANCH: {new_branch}')
        )

    ctx.run("git commit -am 'Commit to compare to itself'", hide=True)
    ctx.run(f"git push origin {new_branch}", hide=True)

    try:
        max_attempts = 18
        compare_to_pipeline = None
        for attempt in range(max_attempts):
            print(f"[{datetime.now()}] Waiting 10s for the branch to be created {attempt + 1}/{max_attempts}")
            time.sleep(10)
            if agent.branches.get(new_branch, raise_exception=False):
                break
        else:
            print(f"{color_message('ERROR', Color.RED)}: Branch {new_branch} not created", file=sys.stderr)
            raise RuntimeError(f"No branch found for {new_branch}")

        print('Branch created, triggering the pipeline')

        # Trigger the pipeline on the last commit of this branch
        compare_to_pipeline = agent.pipelines.create({'ref': new_branch})
        print(f"Pipeline created: {compare_to_pipeline.web_url}")

        if len(compare_to_pipeline.jobs.list(get_all=False)) == 0:
            print(
                f"[{color_message('ERROR', Color.RED)}] Failed to generate a pipeline for {new_branch}, please check {compare_to_pipeline.web_url}"
            )
            raise Exit(message="compare_to itself failed", code=1)
        else:
            print(f"Pipeline correctly created, {color_message('congrats', Color.GREEN)}")
    finally:
        pipelines = agent.pipelines.list(ref=new_branch, get_all=True)
        print(f"Cleaning up the pipelines: {' '.join([str(p.id) for p in pipelines])}")
        for pipeline in pipelines:
            cancel_pipeline(pipeline)
        print("Cleaning up git")
        ctx.run(f"git checkout {current_branch}", hide=True)
        ctx.run(f"git branch -D {new_branch}", hide=True)
        ctx.run(f"git push origin :{new_branch}", hide=True)


@task
def is_dev_branch(_):
    """
    Check if the current branch is not a dev branch.
    """
    # Mirror logic from .fast_on_dev_branch_only in .gitlab-ci.yml
    # Not a dev branch if any of the following is true:
    # - On main branch
    # - On a release branch (e.g., 7.42.x)
    # - On a tagged commit
    # - In a triggered pipeline

    current_branch = os.getenv("CI_COMMIT_BRANCH", "")

    # Main branch
    if current_branch == "main":
        print("false")
        return

    # Release branch: matches \d+.\d+.x
    if re.match(r"^\d+\.\d+\.x$", current_branch):
        print("false")
        return

    # Tagged commit (prefer CI variable if present)
    ci_commit_tag = os.getenv("CI_COMMIT_TAG", "")
    if ci_commit_tag is not None and ci_commit_tag != "":
        print("false")
        return

    # Triggered pipeline (CI context)
    ci_pipeline_source = os.getenv("CI_PIPELINE_SOURCE", "")
    if ci_pipeline_source in ("trigger", "pipeline"):
        print("false")
        return

    # Otherwise, consider it a dev branch
    print("true")
