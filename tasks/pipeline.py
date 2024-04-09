import os
import pprint
import re
import sys
import time
from datetime import datetime, timedelta, timezone

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.ciproviders.gitlab import Gitlab, get_gitlab_bot_token, get_gitlab_token
from tasks.libs.common.color import color_message
from tasks.libs.common.utils import (
    DEFAULT_BRANCH,
    GITHUB_REPO_NAME,
    check_clean_branch_state,
    get_all_allowed_repo_branches,
    is_allowed_repo_branch,
    nightly_entry_for,
    release_entry_for,
)
from tasks.libs.owners.parsing import read_owners
from tasks.libs.pipeline.notifications import send_slack_message
from tasks.libs.pipeline.tools import (
    FilteredOutException,
    cancel_pipelines_with_confirmation,
    get_running_pipelines_on_same_ref,
    gracefully_cancel_pipeline,
    trigger_agent_pipeline,
    wait_for_pipeline,
)


class GitlabReference(yaml.YAMLObject):
    def __init__(self, refs):
        self.refs = refs

    def __repr__(self):
        return f'{self.__class__.__name__}=(refs={self.refs}'


def reference_constructor(loader, node):
    return GitlabReference(loader.construct_sequence(node))


def GitlabYamlLoader():
    loader = yaml.SafeLoader
    loader.add_constructor('!reference', reference_constructor)
    return loader


# Tasks to trigger pipelines


def check_deploy_pipeline(gitlab, git_ref, release_version_6, release_version_7, repo_branch):
    """
    Run checks to verify a deploy pipeline is valid:
    - it targets a valid repo branch
    - it has matching Agent 6 and Agent 7 tags (depending on release_version_* values)
    """

    # Check that the target repo branch is valid
    if not is_allowed_repo_branch(repo_branch):
        print(
            f"--repo-branch argument '{repo_branch}' is not in the list of allowed repository branches: {get_all_allowed_repo_branches()}"
        )
        raise Exit(code=1)

    #
    # If git_ref matches v7 pattern and release_version_6 is not empty, make sure Gitlab has v6 tag.
    # If git_ref matches v6 pattern and release_version_7 is not empty, make sure Gitlab has v7 tag.
    # v7 version pattern should be able to match 7.12.24-rc2 and 7.12.34
    #
    v7_pattern = r'^7\.(\d+\.\d+)(-.+|)$'
    v6_pattern = r'^6\.(\d+\.\d+)(-.+|)$'

    match = re.match(v7_pattern, git_ref)

    if release_version_6 and match:
        # release_version_6 is not empty and git_ref matches v7 pattern, construct v6 tag and check.
        tag_name = "6." + "".join(match.groups())
        gitlab_tag = gitlab.find_tag(tag_name)

        if ("name" not in gitlab_tag) or gitlab_tag["name"] != tag_name:
            print(f"Cannot find GitLab v6 tag {tag_name} while trying to build git ref {git_ref}")
            raise Exit(code=1)

        print(f"Successfully cross checked v6 tag {tag_name} and git ref {git_ref}")
    else:
        match = re.match(v6_pattern, git_ref)

        if release_version_7 and match:
            # release_version_7 is not empty and git_ref matches v6 pattern, construct v7 tag and check.
            tag_name = "7." + "".join(match.groups())
            gitlab_tag = gitlab.find_tag(tag_name)

            if ("name" not in gitlab_tag) or gitlab_tag["name"] != tag_name:
                print(f"Cannot find GitLab v7 tag {tag_name} while trying to build git ref {git_ref}")
                raise Exit(code=1)

            print(f"Successfully cross checked v7 tag {tag_name} and git ref {git_ref}")


@task
def clean_running_pipelines(ctx, git_ref=DEFAULT_BRANCH, here=False, use_latest_sha=False, sha=None):
    """
    Fetch running pipelines on a target ref (+ optionally a git sha), and ask the user if they
    should be cancelled.
    """

    gitlab = Gitlab(api_token=get_gitlab_token())
    gitlab.test_project_found()

    if here:
        git_ref = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()

    print(f"Fetching running pipelines on {git_ref}")

    if not sha and use_latest_sha:
        sha = ctx.run(f"git rev-parse {git_ref}", hide=True).stdout.strip()
        print(f"Git sha not provided, using the one {git_ref} currently points to: {sha}")
    elif not sha:
        print(f"Git sha not provided, fetching all running pipelines on {git_ref}")

    pipelines = get_running_pipelines_on_same_ref(gitlab, git_ref, sha)

    print(
        f"Found {len(pipelines)} running pipeline(s) matching the request.",
        "They are ordered from the newest one to the oldest one.\n",
        sep='\n',
    )
    cancel_pipelines_with_confirmation(gitlab, pipelines)


def workflow_rules(gitlab_file=".gitlab-ci.yml"):
    """Get Gitlab workflow rules list in a YAML-formatted string."""
    with open(gitlab_file, 'r') as f:
        return yaml.dump(yaml.safe_load(f.read())["workflow"]["rules"])


@task
def trigger(_, git_ref=DEFAULT_BRANCH, release_version_6="dev", release_version_7="dev-a7", repo_branch="dev"):
    """
    OBSOLETE: Trigger a deploy pipeline on the given git ref. Use pipeline.run with the --deploy option instead.
    """

    use_release_entries = ""
    major_versions = []

    if release_version_6 != "nightly" and release_version_7 != "nightly-a7":
        use_release_entries = "--use-release-entries "

    if release_version_6 != "":
        major_versions.append("6")

    if release_version_7 != "":
        major_versions.append("7")

    raise Exit(
        f"""The pipeline.trigger task is obsolete. Use:
    pipeline.run --git-ref {git_ref} --deploy --major-versions "{','.join(major_versions)}" --repo-branch {repo_branch} {use_release_entries}
instead.""",
        1,
    )


@task
def auto_cancel_previous_pipelines(ctx):
    """
    Automatically cancel previous pipelines running on the same ref
    """

    if not os.environ.get('GITLAB_TOKEN'):
        raise Exit("GITLAB_TOKEN variable needed to cancel pipelines on the same ref.", 1)

    gitlab = Gitlab(api_token=get_gitlab_token())
    gitlab.test_project_found()

    git_ref = os.getenv("CI_COMMIT_REF_NAME")
    git_sha = os.getenv("CI_COMMIT_SHA")

    pipelines = get_running_pipelines_on_same_ref(gitlab, git_ref)
    pipelines_without_current = [p for p in pipelines if p["sha"] != git_sha]

    for pipeline in pipelines_without_current:
        # We cancel pipeline only if it correspond to a commit that is an ancestor of the current commit
        is_ancestor = ctx.run(f'git merge-base --is-ancestor {pipeline["sha"]} {git_sha}', warn=True, hide="both")
        if is_ancestor.exited == 0:
            print(
                f'Gracefully canceling jobs that are not canceled on pipeline {pipeline["id"]} ({pipeline["web_url"]})'
            )
            gracefully_cancel_pipeline(gitlab, pipeline, force_cancel_stages=["package_build"])
        elif is_ancestor.exited == 1:
            print(f'{pipeline["sha"]} is not an ancestor of {git_sha}, not cancelling pipeline {pipeline["id"]}')
        elif is_ancestor.exited == 128:
            min_time_before_cancel = 5
            print(
                f'Could not determine if {pipeline["sha"]} is an ancestor of {git_sha}, probably because it has been deleted from the history because of force push'
            )
            if datetime.strptime(pipeline["created_at"], "%Y-%m-%dT%H:%M:%S.%fZ") < datetime.now() - timedelta(
                minutes=min_time_before_cancel
            ):
                print(
                    f'Pipeline started earlier than {min_time_before_cancel} minutes ago, gracefully canceling pipeline {pipeline["id"]}'
                )
                gracefully_cancel_pipeline(gitlab, pipeline, force_cancel_stages=["package_build"])
        else:
            print(is_ancestor.stderr)
            raise Exit(1)


@task
def run(
    ctx,
    git_ref=None,
    here=False,
    use_release_entries=False,
    major_versions=None,
    repo_branch="dev",
    deploy=False,
    all_builds=True,
    e2e_tests=True,
    rc_build=False,
    rc_k8s_deployments=False,
):
    """
    Run a pipeline on the given git ref (--git-ref <git ref>), or on the current branch if --here is given.
    By default, this pipeline will run all builds & tests, including all kitchen tests, but is not a deploy pipeline.
    Use --deploy to make this pipeline a deploy pipeline, which will upload artifacts to the staging repositories.
    Use --no-all-builds to not run builds for all architectures (only a subset of jobs will run. No effect on pipelines on the default branch).
    Use --no-kitchen-tests to not run all kitchen tests on the pipeline.
    Use --e2e-tests to run all e2e tests on the pipeline.

    Release Candidate related flags:
    Use --rc-build to mark the build as Release Candidate.
    Use --rc-k8s-deployments to trigger a child pipeline that will deploy Release Candidate build to staging k8s clusters.

    By default, the nightly release.json entries (nightly and nightly-a7) are used.
    Use the --use-release-entries option to use the release-a6 and release-a7 release.json entries instead.

    By default, the pipeline builds both Agent 6 and Agent 7.
    Use the --major-versions option to specify a comma-separated string of the major Agent versions to build
    (eg. '6' to build Agent 6 only, '6,7' to build both Agent 6 and Agent 7).

    The --repo-branch option indicates which branch of the staging repository the packages will be deployed to (useful only on deploy pipelines).

    If other pipelines are already running on the git ref, the script will prompt the user to confirm if these previous
    pipelines should be cancelled.

    Examples
    Run a pipeline on my-branch:
      inv pipeline.run --git-ref my-branch

    Run a pipeline on the current branch:
      inv pipeline.run --here

    Run a pipeline without kitchen tests on the current branch:
      inv pipeline.run --here --no-kitchen-tests

    Run a pipeline with e2e tets on the current branch:
      inv pipeline.run --here --e2e-tests

    Run a deploy pipeline on the 7.32.0 tag, uploading the artifacts to the stable branch of the staging repositories:
      inv pipeline.run --deploy --use-release-entries --major-versions "6,7" --git-ref "7.32.0" --repo-branch "stable"
    """

    gitlab = Gitlab(api_token=get_gitlab_token())
    gitlab.test_project_found()

    if (not git_ref and not here) or (git_ref and here):
        raise Exit("ERROR: Exactly one of --here or --git-ref <git ref> must be specified.", code=1)

    if use_release_entries:
        release_version_6 = release_entry_for(6)
        release_version_7 = release_entry_for(7)
    else:
        release_version_6 = nightly_entry_for(6)
        release_version_7 = nightly_entry_for(7)

    if major_versions:
        print(
            "[WARNING] --major-versions option will be deprecated soon. Both Agent 6 & 7 will be run everytime.",
            file=sys.stderr,
        )

    if here:
        git_ref = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()

    if deploy:
        # Check the validity of the deploy pipeline
        check_deploy_pipeline(gitlab, git_ref, release_version_6, release_version_7, repo_branch)
        # Force all builds and kitchen tests to be run
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

    pipelines = get_running_pipelines_on_same_ref(gitlab, git_ref)

    if pipelines:
        print(
            f"There are already {len(pipelines)} pipeline(s) running on the target git ref.",
            "For each of them, you'll be asked whether you want to cancel them or not.",
            "If you don't need these pipelines, please cancel them to save CI resources.",
            "They are ordered from the newest one to the oldest one.\n",
            sep='\n',
        )
        cancel_pipelines_with_confirmation(gitlab, pipelines)

    try:
        pipeline_id = trigger_agent_pipeline(
            gitlab,
            git_ref,
            release_version_6,
            release_version_7,
            repo_branch,
            deploy=deploy,
            all_builds=all_builds,
            e2e_tests=e2e_tests,
            rc_build=rc_build,
            rc_k8s_deployments=rc_k8s_deployments,
        )
    except FilteredOutException:
        print(color_message(f"ERROR: pipeline does not match any workflow rule. Rules:\n{workflow_rules()}", "red"))
        return

    wait_for_pipeline(gitlab, pipeline_id)


@task
def follow(ctx, id=None, git_ref=None, here=False, project_name="DataDog/datadog-agent"):
    """
    Follow a pipeline's progress in the CLI.
    Use --here to follow the latest pipeline on your current branch.
    Use --git-ref to follow the latest pipeline on a given tag or branch.
    Use --id to follow a specific pipeline.
    Use --project-name to specify a repo other than DataDog/datadog-agent (default)

    Examples:
    inv pipeline.follow --git-ref my-branch
    inv pipeline.follow --here
    inv pipeline.follow --id 1234567
    """

    gitlab = Gitlab(project_name=project_name, api_token=get_gitlab_token())
    gitlab.test_project_found()

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
        wait_for_pipeline(gitlab, id)
    elif git_ref is not None:
        wait_for_pipeline_from_ref(gitlab, git_ref)
    elif here:
        git_ref = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()
        wait_for_pipeline_from_ref(gitlab, git_ref)


def wait_for_pipeline_from_ref(gitlab, ref):
    pipeline = gitlab.last_pipeline_for_ref(ref)
    if pipeline is not None:
        wait_for_pipeline(gitlab, pipeline['id'])
    else:
        print(f"No pipelines found for {ref}")
        raise Exit(code=1)


@task(iterable=['variable'])
def trigger_child_pipeline(_, git_ref, project_name, variable=None, follow=True):
    """
    Trigger a child pipeline on a target repository and git ref.
    Used in CI jobs only (requires CI_JOB_TOKEN).

    Use --variable to specify the environment variables that should be passed to the child pipeline.
    You can pass the argument multiple times for each new variable you wish to forward

    Use --follow to make this task wait for the pipeline to finish, and return 1 if it fails. (requires GITLAB_TOKEN).

    Examples:
    inv pipeline.trigger-child-pipeline --git-ref "master" --project-name "DataDog/agent-release-management" --variables "RELEASE_VERSION"

    inv pipeline.trigger-child-pipeline --git-ref "master" --project-name "DataDog/agent-release-management" --variables "VAR1,VAR2,VAR3"
    """

    if not os.environ.get('CI_JOB_TOKEN'):
        raise Exit("CI_JOB_TOKEN variable needed to create child pipelines.", 1)

    if not os.environ.get('GITLAB_TOKEN'):
        if follow:
            raise Exit("GITLAB_TOKEN variable needed to follow child pipelines.", 1)
        else:
            # The Gitlab lib requires `GITLAB_TOKEN` to be
            # set, but trigger_pipeline doesn't use it
            os.environ["GITLAB_TOKEN"] = os.environ['CI_JOB_TOKEN']

    gitlab = Gitlab(project_name=project_name, api_token=get_gitlab_token())

    data = {"token": os.environ['CI_JOB_TOKEN'], "ref": git_ref, "variables": {}}

    # Fill the environment variables to pass to the child pipeline.
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
            data['variables'][v] = os.environ[v]

    print(
        f"Creating child pipeline in repo {project_name}, on git ref {git_ref} with params: {data['variables']}",
        flush=True,
    )

    res = gitlab.trigger_pipeline(data)

    if 'id' not in res:
        raise Exit(f"Failed to create child pipeline: {res}", code=1)

    pipeline_id = res['id']
    pipeline_url = res['web_url']
    print(f"Created a child pipeline with id={pipeline_id}, url={pipeline_url}", flush=True)

    if follow:
        print("Waiting for child pipeline to finish...", flush=True)

        wait_for_pipeline(gitlab, pipeline_id)

        # Check pipeline status
        pipeline = gitlab.pipeline(pipeline_id)
        pipestatus = pipeline["status"].lower().strip()

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
        ("TEAM", "@DataDog/Networks"),
        ("TEAM", "@DataDog/universal-service-monitoring"),
        ("TEAM", "@DataDog/ebpf-platform"),
        ("TEAM", "@DataDog/agent-security"),
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
    "leeavital@gmail.com": "UDG2223C1",
}


@task
def changelog(ctx, new_commit_sha):
    # Environment variable to deal with both local and CI environments
    if "CI_PROJECT_DIR" in os.environ:
        parent_dir = os.environ["CI_PROJECT_DIR"]
    else:
        parent_dir = os.getcwd()
    old_commit_sha = ctx.run(
        f"{parent_dir}/tools/ci/aws_ssm_get_wrapper.sh {os.environ['CHANGELOG_COMMIT_SHA_SSM_NAME']}",
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
        "The nightly deployment is rolling out to Staging :siren: \n"
        + f"Changelog for <{commit_range_link}|commit range>: `{old_commit_sha}` to `{new_commit_sha}`:\n"
    )

    if old_commit_sha == new_commit_sha:
        print("No new commits found, exiting")
        slack_message += no_commits_msg
        send_slack_message("system-probe-ops", slack_message)
        return

    print(f"Generating changelog for commit range {old_commit_sha} to {new_commit_sha}")
    commits = ctx.run(f"git log {old_commit_sha}..{new_commit_sha} --pretty=format:%h", hide=True).stdout.split("\n")
    owners = read_owners(".github/CODEOWNERS")
    messages = []

    for commit in commits:
        # see https://git-scm.com/docs/pretty-formats for format string
        commit_str = ctx.run(f"git show --name-only --pretty=format:%s%n%aN%n%aE {commit}", hide=True).stdout
        title, author, author_email, files, url = parse(commit_str)
        if not is_system_probe(owners, files):
            continue
        message_link = f"• <{url}|{title}>" if url else f"• {title}"
        if "dependabot" in author_email or "github-actions" in author_email:
            messages.append(f"{message_link}")
            continue
        if author_email in EMAIL_SLACK_ID_MAP:
            author_handle = EMAIL_SLACK_ID_MAP[author_email]
        else:
            author_handle = ctx.run(f"email2slackid {author_email.strip()}", hide=True).stdout.strip()
        if author_handle:
            author_handle = f"<@{author_handle}>"
        else:
            author_handle = author_email
        time.sleep(1)  # necessary to prevent slack/sdm API rate limits
        messages.append(f"{message_link} {author_handle}")

    if messages:
        slack_message += (
            "\n".join(messages) + "\n:wave: Authors, please check the "
            "<https://ddstaging.datadoghq.com/dashboard/kfn-zy2-t98?tpl_var_cluster_name%5B0%5D=stripe"
            "&tpl_var_cluster_name%5B1%5D=muk&tpl_var_cluster_name%5B2%5D=snowver"
            "&tpl_var_cluster_name%5B3%5D=chillpenguin&tpl_var_cluster_name%5B4%5D=diglet"
            "&tpl_var_cluster_name%5B5%5D=lagaffe&tpl_var_datacenter%5B0%5D=%2A|dashboard> for issues"
        )
    else:
        slack_message += empty_changelog_msg

    print(f"Posting message to slack: \n {slack_message}")
    send_slack_message("system-probe-ops", slack_message)
    print(f"Writing new commit sha: {new_commit_sha} to SSM")
    ctx.run(
        f"aws ssm put-parameter --name ci.datadog-agent.gitlab_changelog_commit_sha --value {new_commit_sha} "
        "--type \"SecureString\" --region us-east-1 --overwrite",
        hide=True,
    )


def _init_pipeline_schedule_task():
    gitlab = Gitlab(api_token=get_gitlab_bot_token())
    gitlab.test_project_found()
    return gitlab


@task
def get_schedules(_):
    """
    Pretty-print all pipeline schedules on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    for ps in gitlab.all_pipeline_schedules():
        pprint.pprint(ps)


@task
def get_schedule(_, schedule_id):
    """
    Pretty-print a single pipeline schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.pipeline_schedule(schedule_id)
    pprint.pprint(result)


@task
def create_schedule(_, description, ref, cron, cron_timezone=None, active=False):
    """
    Create a new pipeline schedule on the repository.

    Note that unless you explicitly specify the --active flag, the schedule will be created as inactive.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.create_pipeline_schedule(description, ref, cron, cron_timezone, active)
    pprint.pprint(result)


@task
def edit_schedule(_, schedule_id, description=None, ref=None, cron=None, cron_timezone=None):
    """
    Edit an existing pipeline schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.edit_pipeline_schedule(schedule_id, description, ref, cron, cron_timezone)
    pprint.pprint(result)


@task
def activate_schedule(_, schedule_id):
    """
    Activate an existing pipeline schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.edit_pipeline_schedule(schedule_id, active=True)
    pprint.pprint(result)


@task
def deactivate_schedule(_, schedule_id):
    """
    Deactivate an existing pipeline schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.edit_pipeline_schedule(schedule_id, active=False)
    pprint.pprint(result)


@task
def delete_schedule(_, schedule_id):
    """
    Delete an existing pipeline schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.delete_pipeline_schedule(schedule_id)
    pprint.pprint(result)


@task
def create_schedule_variable(_, schedule_id, key, value):
    """
    Create a variable for an existing schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.create_pipeline_schedule_variable(schedule_id, key, value)
    pprint.pprint(result)


@task
def edit_schedule_variable(_, schedule_id, key, value):
    """
    Edit an existing variable for a schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.edit_pipeline_schedule_variable(schedule_id, key, value)
    pprint.pprint(result)


@task
def delete_schedule_variable(_, schedule_id, key):
    """
    Delete an existing variable for a schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.delete_pipeline_schedule_variable(schedule_id, key)
    pprint.pprint(result)


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
    create_branch = branch_name is None
    branch_name = verify_workspace(ctx, branch_name=branch_name)
    update_gitlab_config(".gitlab-ci.yml", image_tag, test_version=test_version)
    update_circleci_config(".circleci/config.yml", image_tag, test_version=test_version)
    trigger_build(ctx, branch_name=branch_name, create_branch=create_branch)


def verify_workspace(ctx, branch_name=None):
    """
    Assess we can modify files and commit without risk of local or upstream conflicts
    """
    if branch_name is None:
        user_name = ctx.run("whoami", hide="out")
        branch_name = f"{user_name.stdout.rstrip()}/test_buildimages"
        github = GithubAPI(repository=GITHUB_REPO_NAME)
        check_clean_branch_state(ctx, github, branch_name)
    return branch_name


def update_test_infra_def(file_path, image_tag):
    """
    Override TEST_INFRA_DEFINITIONS_BUILDIMAGES in `.gitlab/common/test_infra_version.yml` file
    """
    with open(file_path, "r") as gl:
        file_content = gl.readlines()
    with open(file_path, "w") as gl:
        for line in file_content:
            test_infra_def = re.search(r"TEST_INFRA_DEFINITIONS_BUILDIMAGES:\s*(\w+)", line)
            if test_infra_def:
                gl.write(line.replace(test_infra_def.group(1), image_tag))
            else:
                gl.write(line)


def update_gitlab_config(file_path, image_tag, test_version):
    """
    Override variables in .gitlab-ci.yml file
    """
    with open(file_path, "r") as gl:
        file_content = gl.readlines()
    gitlab_ci = yaml.load("".join(file_content), Loader=GitlabYamlLoader())
    # TEST_INFRA_DEFINITION_BUILDIMAGE label format differs from other buildimages
    suffixes = [
        name
        for name in gitlab_ci["variables"]
        if name.endswith("SUFFIX") and not name.startswith("TEST_INFRA_DEFINITION")
    ]
    images = [name.replace("_SUFFIX", "") for name in suffixes]
    with open(file_path, "w") as gl:
        for line in file_content:
            if any(re.search(fr"{suffix}:", line) for suffix in suffixes):
                if test_version:
                    gl.write(line.replace('""', '"_test_only"'))
                else:
                    gl.write(line.replace('"_test_only"', '""'))
            elif any(re.search(fr"{image}:", line) for image in images):
                current_version = re.search(r"v\d+-\w+", line)
                if current_version:
                    gl.write(line.replace(current_version.group(0), image_tag))
                else:
                    raise RuntimeError(
                        f"Unable to find a version matching the v<pipelineId>-<commitId> pattern in line {line}"
                    )
            else:
                gl.write(line)


def update_circleci_config(file_path, image_tag, test_version):
    """
    Override variables in .gitlab-ci.yml file
    """
    image_name = "gcr.io/datadoghq/agent-circleci-runner"
    with open(file_path, "r") as circle:
        circle_ci = circle.read()
    match = re.search(rf"({image_name}(_test_only)?):([a-zA-Z0-9_-]+)\n", circle_ci)
    if not match:
        raise RuntimeError(f"Impossible to find the version of image {image_name} in circleci configuration file")
    image = f"{image_name}_test_only" if test_version else image_name
    with open(file_path, "w") as circle:
        circle.write(circle_ci.replace(f"{match.group(0)}", f"{image}:{image_tag}\n"))


def trigger_build(ctx, branch_name=None, create_branch=False):
    """
    Trigger a pipeline from current branch on-demand (useful for test image)
    """
    if create_branch:
        ctx.run(f"git checkout -b {branch_name}")
    answer = input("Do you want to trigger a pipeline (will also commit and push)? [Y/n]\n")
    if len(answer) == 0 or answer.casefold() == "y":
        ctx.run("git add .gitlab-ci.yml .circleci/config.yml")
        ctx.run("git commit -m 'Update buildimages version'")
        ctx.run(f"git push origin {branch_name}")
        print("Wait 10s to let Gitlab create the first events before triggering a new pipeline")
        time.sleep(10)
        run(ctx, here=True)


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
    # Verify parameters
    owner_branch_name = owner_branch_name.lower()

    assert (
        owner_branch_name.count('/') == 1
    ), f'owner_branch_name should be "<owner-name>/<branch-name>" but is {owner_branch_name}'
    assert "'" not in owner_branch_name

    owner, branch = owner_branch_name.split('/')
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

    # Commands to push the branch
    commands = [
        # Fetch
        f"git remote add {owner} git@github.com:{owner}/datadog-agent.git",
        f"git fetch '{owner}'",
        # Create branch
        f"git checkout '{owner}/{branch}'",  # This first checkout puts us in a detached head state, thus the second checkout below
        f"git checkout -b '{owner}/{branch}'",
        # Push
        f"git push --set-upstream origin '{owner}/{branch}'{no_verify_flag}",
    ] + restore_commands

    # Run commands then restore commands
    ret_code = 0
    for command in commands:
        ret_code = ctx.run(command, warn=True, echo=True).exited
        if ret_code != 0:
            print('The last command exited with code', ret_code)
            print('You might want to run these commands to restore the current state:')
            print('\n'.join(restore_commands))

            exit(1)

    # Show links
    repo = f'https://github.com/DataDog/datadog-agent/tree/{owner}/{branch}'
    pipeline = f'https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Apipeline%20%40ci.provider.name%3Agitlab%20%40git.repository.name%3A%22DataDog%2Fdatadog-agent%22%20%40git.branch%3A%22{owner}%2F{branch}%22&colorBy=meta%5B%27ci.stage.name%27%5D&colorByAttr=meta%5B%27ci.stage.name%27%5D&currentTab=json&fromUser=false&index=cipipeline&sort=time&spanViewType=logs'

    print(f'\nBranch {owner}/{branch} pushed to repo: {repo}')
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
    test_main = f"mq/test_{timestamp}"
    current_branch = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()
    ctx.run("git checkout main", hide=True)
    ctx.run("git pull", hide=True)
    ctx.run(f"git checkout -b {test_main}", hide=True)
    ctx.run(f"git push origin {test_main}", hide=True)
    # Create a PR towards this new branch and adds it to the merge queue
    print("Creating a PR and adding it to the merge queue")
    gh = GithubAPI()
    pr = gh.create_pr(f"Test MQ for {current_branch}", "", test_main, current_branch)
    pr.create_issue_comment("/merge")
    # Search for the generated pipeline
    print(f"PR {pr.html_url} is waiting for MQ pipeline generation")
    gitlab = Gitlab(api_token=get_gitlab_token())
    max_attempts = 5
    for attempt in range(max_attempts):
        time.sleep(30)
        pipelines = gitlab.last_pipelines()
        try:
            pipeline = next(p for p in pipelines if p["ref"].startswith(f"mq-working-branch-{test_main}"))
            print(f"Pipeline found: {pipeline['web_url']}")
            break
        except StopIteration:
            if attempt == max_attempts - 1:
                raise RuntimeError("No pipeline found for the merge queue")
            continue
    success = pipeline["status"] == "running"
    if success:
        print("Pipeline correctly created, congrats")
    else:
        print(f"[ERROR] Impossible to generate a pipeline for the merge queue, please check {pipeline['web_url']}")
    # Clean up
    print("Cleaning up")
    if success:
        gitlab.cancel_pipeline(pipeline["id"])
    pr.edit(state="closed")
    ctx.run(f"git checkout {current_branch}", hide=True)
    ctx.run(f"git branch -D {test_main}", hide=True)
    ctx.run(f"git push origin :{test_main}", hide=True)
    if not success:
        raise Exit(message="Merge queue test failed", code=1)
