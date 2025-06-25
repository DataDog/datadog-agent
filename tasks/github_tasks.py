from __future__ import annotations

import json
import os
import time
from collections import Counter

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.libs.ciproviders.github_actions_tools import (
    download_artifacts,
    download_with_retry,
    follow_workflow_run,
    print_failed_jobs_logs,
    print_workflow_conclusion,
    trigger_windows_bump_workflow,
)
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.datadog_api import create_gauge, send_event, send_metrics
from tasks.libs.owners.linter import codeowner_has_orphans, directory_has_packages_without_owner
from tasks.libs.owners.parsing import read_owners
from tasks.libs.pipeline.notifications import GITHUB_SLACK_MAP
from tasks.libs.releasing.version import current_version
from tasks.libs.types.types import PermissionCheck

ALL_TEAMS = '@datadog/agent-all'


def _update_windows_runner_version(new_version=None, repo="ci-platform-machine-images"):
    if new_version is None:
        raise Exit(message="workflow needs the 'new_version' field value to be not None")
    args_per_repo = {
        "ci-platform-machine-images": {
            "workflow_name": "windows-runner-agent-bump.yml",
            "github_action_ref": "main",
        },
    }

    run = trigger_windows_bump_workflow(
        repo=repo,
        workflow_name=args_per_repo[repo]["workflow_name"],
        github_action_ref=args_per_repo[repo]["github_action_ref"],
        new_version=new_version,
    )
    full_repo = f"DataDog/{repo}"
    workflow_conclusion, workflow_url = follow_workflow_run(run, full_repo, 0.5)

    if workflow_conclusion != "success":
        if workflow_conclusion == "failure":
            print_failed_jobs_logs(run)
        return workflow_conclusion

    print_workflow_conclusion(workflow_conclusion, workflow_url)

    download_with_retry(download_artifacts, run, ".", 3, 5, full_repo)

    with open("PR_URL_ARTIFACT") as f:
        PR_URL = f.read().strip()

    if not PR_URL:
        raise Exit(message="Failed to fetch artifact from the workflow. (Empty artifact)")

    message = f":robobits: A new windows-runner bump PR to {new_version} has been generated. Please take a look :frog-review:\n:pr: {PR_URL} :ty:"

    from slack_sdk import WebClient

    client = WebClient(token=os.environ["SLACK_DATADOG_AGENT_BOT_TOKEN"])
    client.chat_postMessage(channel="ci-infra-support", text=message)
    return workflow_conclusion


@task
def update_windows_runner_version(
    ctx,
    new_version=None,
):
    """
    Trigger a workflow on the ci-platform-machine-images repository to bump windows gitlab runner
    """
    if new_version is None:
        new_version = str(current_version(ctx, "7"))

    repo = "ci-platform-machine-images"
    conclusion = _update_windows_runner_version(new_version, repo)
    if conclusion != "success":
        raise Exit(message=f"Windows runner bump workflow {conclusion} for {repo}", code=1)


@task
def lint_codeowner(_, owners_file=".github/CODEOWNERS"):
    """
    Run multiple checks on the provided CODEOWNERS file
    """

    base = os.path.dirname(os.path.abspath(__file__))
    root_folder = os.path.join(base, "..")
    os.chdir(root_folder)

    exit_code = 0

    # Getting GitHub CODEOWNER file content
    owners = read_owners(owners_file)

    # Define linters
    linters = [directory_has_packages_without_owner, codeowner_has_orphans]

    # Execute linters
    for linter in linters:
        if linter(owners):
            exit_code = 1

    raise Exit(code=exit_code)


@task
def get_milestone_id(_, milestone):
    # Local import as github isn't part of our default set of installed
    # dependencies, and we don't want to propagate it to files importing this one
    from tasks.libs.ciproviders.github_api import GithubAPI

    gh = GithubAPI()
    m = gh.get_milestone_by_name(milestone)
    if not m:
        raise Exit(f'Milestone {milestone} wasn\'t found in the repo', code=1)
    print(m.number)


@task
def send_rate_limit_info_datadog(_, pipeline_id, app_instance):
    from tasks.libs.ciproviders.github_api import GithubAPI

    gh = GithubAPI()
    rate_limit_info = gh.get_rate_limit_info()
    print(f"Remaining rate limit for app instance {app_instance}: {rate_limit_info[0]}/{rate_limit_info[1]}")
    metric = create_gauge(
        metric_name='github.rate_limit.remaining',
        timestamp=int(time.time()),
        value=rate_limit_info[0],
        tags=[
            'source:github',
            'repository:datadog-agent',
            f'app_instance:{app_instance}',
        ],
    )
    send_metrics([metric])


@task
def get_token_from_app(_, app_id_env='GITHUB_APP_ID', pkey_env='GITHUB_KEY_B64'):
    from .libs.ciproviders.github_api import GithubAPI

    GithubAPI.get_token_from_app(app_id_env, pkey_env)


def _get_teams(changed_files, owners_file='.github/CODEOWNERS') -> list[str]:
    codeowners = read_owners(owners_file)

    team_counter = Counter()
    for file in changed_files:
        owners = [name for (kind, name) in codeowners.of(file) if kind == 'TEAM']
        team_counter.update(owners)

    team_count = team_counter.most_common()
    if team_count == []:
        return []

    _, best_count = team_count[0]
    best_teams = [team.casefold() for (team, count) in team_count if count == best_count]

    return best_teams


def _get_team_labels():
    import toml

    with open('.ddqa/config.toml') as f:
        data = toml.loads(f.read())

    labels = []
    for team in data['teams'].values():
        labels.extend(team.get('github_labels', []))
    return labels


@task
def assign_team_label(_, pr_id=-1):
    """
    Assigns the github team label name if teams can
    be deduced from the changed files
    """
    from tasks.libs.ciproviders.github_api import GithubAPI

    gh = GithubAPI('DataDog/datadog-agent')

    labels = gh.get_pr_labels(pr_id)

    # Skip if necessary
    if 'qa/done' in labels or 'qa/no-code-change' in labels:
        print('Qa done or no code change, skipping')
        return

    if any(label.startswith('team/') for label in labels):
        print('This PR already has a team label, skipping')
        return

    # Find team
    teams = _get_teams(gh.get_pr_files(pr_id))
    if teams == []:
        print('No team found')
        return

    _assign_pr_team_labels(gh, pr_id, teams)


def _assign_pr_team_labels(gh, pr_id, teams):
    """
    Assign team labels (team/team-name) for each team (@datadog/team-name)
    """
    import github

    # Get labels
    all_team_labels = _get_team_labels()
    team_labels = [f"team{team.removeprefix('@datadog')}" for team in teams]

    # Assign label
    for label_name in team_labels:
        if label_name not in all_team_labels:
            print(label_name, 'cannot be found in .ddqa/config.toml, skipping')
        else:
            try:
                gh.add_pr_label(pr_id, label_name)
                print(label_name, 'label assigned to the pull request')
            except github.GithubException:
                print(f'Failed to assign label {label_name}')


@task
def handle_community_pr(_, repo='', pr_id=-1, labels=''):
    """
    Will set labels and notify teams about a newly opened community PR
    """
    from slack_sdk import WebClient

    from tasks.libs.ciproviders.github_api import GithubAPI

    # Get review teams / channels
    gh = GithubAPI()

    # Find teams corresponding to file changes
    teams = _get_teams(gh.get_pr_files(pr_id)) or [ALL_TEAMS]
    channels = [GITHUB_SLACK_MAP[team.lower()] for team in teams if team if team.lower() in GITHUB_SLACK_MAP]

    # Remove duplicates
    channels = list(set(channels))

    # Update labels
    for label in labels.split(','):
        if label:
            gh.add_pr_label(pr_id, label)

    if teams != [ALL_TEAMS]:
        _assign_pr_team_labels(gh, pr_id, teams)

    # Create message
    pr = gh.get_pr(pr_id)
    title = pr.title.strip()
    message = f':pr: *New Community PR*\n{title} <{pr.html_url}|{repo}#{pr_id}>'

    # Post message
    client = WebClient(os.environ['SLACK_DATADOG_AGENT_BOT_TOKEN'])
    for channel in channels:
        client.chat_postMessage(channel=channel, text=message)


@task
def milestone_pr_team_stats(_: Context, milestone: str, team: str):
    """
    This task prints statistics about the PRs opened by a given team and
    merged in the given milestone.
    """
    from tasks.libs.ciproviders.github_api import GithubAPI

    gh = GithubAPI()
    team_members = gh.get_team_members(team)
    authors = ' '.join("author:" + member.login for member in team_members)
    common_query = f'repo:DataDog/datadog-agent is:pr is:merged milestone:{milestone} {authors}'

    no_code_changes_query = common_query + ' label:qa/no-code-change'
    no_code_changes = gh.search_issues(no_code_changes_query).totalCount

    qa_done_query = common_query + ' -label:qa/no-code-change label:qa/done'
    qa_done = gh.search_issues(qa_done_query).totalCount

    with_qa_query = common_query + ' -label:qa/no-code-change -label:qa/done'
    with_qa = gh.search_issues(with_qa_query).totalCount

    print("no code changes :", no_code_changes)
    print("qa done :", qa_done)
    print("with qa :", with_qa)
    print("proportion of PRs with code changes and QA done :", 100 * qa_done / (qa_done + with_qa), "%")


@task
def pr_commenter(
    _,
    title: str,
    body: str = '',
    pr_id: int | None = None,
    verbose: bool = True,
    delete: bool = False,
    force_delete: bool = False,
    echo: bool = False,
    fail_on_pr_missing: bool = False,
):
    """
    Will comment or update current comment posted on the PR with the new data.
    The title is used to identify the comment to update.

    - pr_id: If None, will use $CI_COMMIT_BRANCH to identify which PR to comment on.
    - delete: If True and the body is empty, will delete the comment.
    - force_delete: Won't throw error if the comment to delete is not found.
    - echo: Print comment content to stdout.
    - fail_on_pr_missing: If True, will raise an error if the PR is not found. Only a warning is printed otherwise.

    Inspired by the pr-commenter binary from <https://github.com/DataDog/devtools>
    """

    from tasks.libs.ciproviders.github_api import GithubAPI

    if not body and not delete:
        return

    assert not delete or not body, "Use delete with an empty body to delete the comment"

    github = GithubAPI()

    if pr_id is None:
        branch = os.environ["CI_COMMIT_BRANCH"]
        prs = list(github.get_pr_for_branch(branch))
        if len(prs) == 0 and not fail_on_pr_missing:
            print(f'{color_message("Warning", Color.ORANGE)}: No PR found for branch {branch}, skipping PR comment')
            return
        assert len(prs) == 1, f"Expected 1 PR for branch {branch}, found {len(prs)} PRs"
        pr = prs[0]
    else:
        pr = github.get_pr(pr_id)

    # Created / updated / deleted comment
    action = ''
    header = f'## {title}'
    content = f'{header}\n\n{body}'

    comment = github.find_comment(pr, header)

    if comment:
        if delete:
            comment.delete()
            action = 'Deleted'
        else:
            comment.edit(content)
            action = 'Updated'
    else:
        if delete and force_delete:
            if verbose:
                print('Comment to delete not found, skipping')
            return
        else:
            assert not delete, 'Comment to delete not found'

        github.publish_comment(pr, content)
        action = 'Created'

    if echo:
        if verbose:
            print('Content:\n')
        print(content)
        print()

    if verbose:
        print(f"{action} comment on PR #{pr.number} - {pr.title}")


@task
def pr_merge_dd_event_sender(
    _,
    pr_id: int | None = None,
    dry_run: bool = False,
):
    """
    Sends a PR merged event to Datadog with the following tags:
    - repo:datadog-agent
    - pr:<pr_number>
    - author:<pr_author>
    - qa_label:missing if the PR doesn't have the qa/done or qa/no-code-change label
    - qa_description:missing if the PR doesn't have a test/QA description section

    - pr_id: If None, will use $CI_COMMIT_BRANCH to identify which PR
    """

    from tasks.libs.ciproviders.github_api import GithubAPI

    github = GithubAPI()

    if pr_id is None:
        branch = os.environ["CI_COMMIT_BRANCH"]
        prs = list(github.get_pr_for_branch(branch))
        assert len(prs) == 1, f"Expected 1 PR for branch {branch}, found {len(prs)} PRs"
        pr = prs[0]
    else:
        pr = github.get_pr(int(pr_id))

    if not pr.merged:
        raise Exit(f"PR #{pr.number} is not merged yet", code=1)

    tags = [f'repo:{pr.base.repo.full_name}', f'pr_id:{pr.number}', f'author:{pr.user.login}']
    labels = set(github.get_pr_labels(pr.number))
    all_qa_labels = {'qa/done', 'qa/no-code-change', 'qa/rc-required'}
    qa_labels = all_qa_labels.intersection(labels)
    if len(qa_labels) == 0:
        tags.append('qa_label:missing')
    else:
        tags.extend([f"qa_label:{label}" for label in qa_labels])

    qa_description = extract_test_qa_description(pr.body)
    if qa_description == '':
        tags.append('qa_description:missing')

    tags.extend([f"team:{label.removeprefix('team/')}" for label in labels if label.startswith('team/')])
    title = "PR merged"
    text = f"PR #{pr.number} merged to {pr.base.ref} at {pr.base.repo.full_name} by {pr.user.login} with QA description [{qa_description}]"

    if dry_run:
        print(f'''I would send the following event to Datadog:

title: {title}
text: {text}
tags: {tags}''')
        return

    send_event(
        title=title,
        text=text,
        tags=tags,
    )

    print(f"Event sent to Datadog for PR #{pr.number}")


def extract_test_qa_description(pr_body: str) -> str:
    """
    Extract the test/QA description section from the PR body
    """
    # Extract the test/QA description section from the PR body
    # Based on PULL_REQUEST_TEMPLATE.md
    pr_body_lines = pr_body.splitlines()
    index_of_test_qa_section = -1
    for i, line in enumerate(pr_body_lines):
        if line.startswith('### Describe how you validated your changes'):
            index_of_test_qa_section = i
            break
    if index_of_test_qa_section == -1:
        return ''
    index_of_next_section = len(pr_body_lines)
    for i in range(index_of_test_qa_section + 1, len(pr_body_lines)):
        if pr_body_lines[i].startswith('### '):
            index_of_next_section = i
            break
    if index_of_next_section == -1:
        return ''
    return '\n'.join(pr_body_lines[index_of_test_qa_section + 1 : index_of_next_section]).strip()


@task
def assign_codereview_label(_, pr_id=-1):
    """
    Assigns a code review complexity label based on PR attributes (files changed, additions, deletions, comments)
    """
    from tasks.libs.ciproviders.github_api import GithubAPI

    gh = GithubAPI('DataDog/datadog-agent')
    complexity = gh.get_codereview_complexity(pr_id)
    gh.update_review_complexity_labels(pr_id, complexity)


@task
def agenttelemetry_list_change_ack_check(_, pr_id=-1):
    """
    Change to `comp/core/agenttelemetry/impl/config.go` file requires to acknowledge
    potential changes to Agent Telemetry metrics. If Agent Telemetry metric list has been changed,
    the PR should be labeled with `need-change/agenttelemetry-governance` and follow
    `Agent Telemetry Governance` instructions to potentially perform additional changes. See
    https://datadoghq.atlassian.net/wiki/spaces/ASUP/pages/4340679635/Agent+Telemetry+Governance
    for details.
    """
    from tasks.libs.ciproviders.github_api import GithubAPI

    gh = GithubAPI('DataDog/datadog-agent')

    labels = gh.get_pr_labels(pr_id)
    files = gh.get_pr_files(pr_id)
    if "comp/core/agenttelemetry/impl/config.go" in files:
        if "need-change/agenttelemetry-governance" not in labels:
            message = f"{color_message('Error', 'red')}: If you change the `comp/core/agenttelemetry/impl/config.go` file, you need to add `need-change/agenttelemetry-governance` label. If you have access, pleas follow the instructions specified in https://datadoghq.atlassian.net/wiki/spaces/ASUP/pages/4340679635/Agent+Telemetry+Governance"
            raise Exit(message, code=1)
        else:
            print(
                "'need-change/agenttelemetry-governance' label found on the PR: potential change to Agent Telemetry metrics is acknowledged and the governance instructions are followed."
            )


@task
def get_required_checks(_, branch: str = "main"):
    """
    For this task to work:
        - A Personal Access Token (PAT) needs the "repo" permissions.
        - A fine-grained token needs the "Administration" repository permissions (read).
    """
    from tasks.libs.ciproviders.github_api import GithubAPI

    gh = GithubAPI()
    required_checks = gh.get_branch_required_checks(branch)
    print(required_checks)


@task(iterable=['check'])
def add_required_checks(_, branch: str, check: str, force: bool = False):
    """
    For this task to work:
        - A Personal Access Token (PAT) needs the "repo" permissions.
        - A fine-grained token needs the "Administration" repository permissions (write).

    Use it like this:
    dda inv github.add-required-checks --branch=main --check="dd-gitlab/lint_codeowners" --check="dd-gitlab/lint_components"
    """
    from tasks.libs.ciproviders.github_api import GithubAPI

    if not check:
        raise Exit(color_message("No check name provided, exiting", Color.RED), code=1)

    gh = GithubAPI()
    gh.add_branch_required_check(branch, check, force)


@task
def check_qa_labels(_, labels: str):
    """
    Check if the PR has one of qa/[done|no-code-change|rc-required] label
    """
    labels = set(labels.split(" "))
    all_qa_labels = {'qa/done', 'qa/no-code-change', 'qa/rc-required'}
    qa_labels = all_qa_labels.intersection(labels)
    docs = "\n".join(
        [
            "You must set one of:",
            "- 'qa/no-code-change' if your PR does not contain changes to the agent code or has no impact to the agent functionalities",
            "  Examples: code owner changes, e2e test framework changes, documentation changes",
            "- 'qa/done' if your PR contains changes impacting the Agent binary code that are validated through automated tests, double checked through manual validation if needed.",
            "  If you want additional validation by a second person, you can ask reviewers to do it. Describe how to set up an environment for manual tests in the PR description. Manual validation is expected to happen on every commit before merge.",
            "  Any manual validation step should then map to an automated test. Manual validation should not substitute automation, minus exceptions not supported by test tooling yet.",
            "- 'qa/rc-required' if your PR changes require validation on the Release Candidate. Examples are changes that need workloads that we cannot emulate, or changes that require validation on prod during RC deployment",
            "",
            "See https://datadoghq.atlassian.net/wiki/spaces/agent/pages/3341649081/QA+Best+Practices for more details.",
        ]
    )
    if len(qa_labels) == 0:
        raise Exit(f"No QA label set.\n{docs}", code=1)
    if len(qa_labels) > 1:
        raise Exit(f"More than one QA label set.\n{docs}", code=1)
    print("QA label set correctly")


@task
def print_pr_state(_, id):
    """Print the PR merge state if the PR is stuck within the merge queue."""

    from tasks.libs.ciproviders.github_api import GithubAPI

    query = """
query {
  repository (owner: "DataDog", name: "datadog-agent") {
    pullRequest(number: ID) {
      reviewDecision
      state
      statusCheckRollup {
        state
      }
      mergeable
      mergeStateStatus
      locked
    }
  }
}
""".replace("ID", id)  # Use replace to avoid formatting issues with curly braces

    gh = GithubAPI()
    res = gh.graphql(query)

    print(json.dumps(res, indent=2))


@task
def check_permissions(_, name: str, check: PermissionCheck = PermissionCheck.REPO, channel: str = "agent-devx-ops"):
    """
    Check the permissions on a given repository or team.
      - list contributing teams on the repository or subteams
      - list teams with not any contributors
      - list members without any contribution in the last period

    """
    from tasks.libs.ciproviders.github_api import GithubAPI
    from tasks.libs.common.slack import format_teams, header_block, markdown_block
    from tasks.libs.notify.permissions import list_permissions

    if check == PermissionCheck.TEAM:
        gh = GithubAPI()
        root = gh.get_team(name)
        depth = None
        admins = root.get_members(role='maintainer')
    else:
        gh = GithubAPI(f"datadog/{name}")
        root = gh._repository
        depth = 1
        admins = [
            c for c in root.get_collaborators(affiliation='direct') if root.get_collaborator_permission(c) == 'admin'
        ]

    all_teams = gh.find_teams(
        root,
        depth=depth,
    )

    # List information of teams in <name>
    blocks = [header_block(f":github: {name} permissions check\n")]
    blocks.extend(format_teams(name, check, all_teams))

    # Add admins
    if len(admins) > 0:
        admins = [f" - <{admin.html_url}|{admin.login}>\n" for admin in admins]
        block = f"Admins:\n{''.join(admins)}"
        blocks.append(markdown_block(block))

    if check == PermissionCheck.TEAM:
        # For agent-all, list non contributors (team or members) to remove or look at.
        contributors = 'agent-contributors'
        non_contributing_teams, contributors_to_remove, membership = list_permissions(gh, name, all_teams, contributors)

        if len(non_contributing_teams) > 0:
            teams = [f" - {team}\n" for team in non_contributing_teams]
            block = f"Non contributing teams:\n{''.join(teams)}"
            blocks.append(markdown_block(block))

        if len(contributors_to_remove) > 0:
            block = f"Non contributors to remove from <https://github.com/orgs/DataDog/teams/{contributors}|{contributors}> team:\n{','.join(contributors_to_remove)}"
            blocks.append(markdown_block(block))

        if len(membership) > 0:
            block = "Non contributors to assess:\n"
            blocks.append(markdown_block(block))
            for member, teams in membership.items():
                block = f" - {member}: [{', '.join(teams)}]\n"
                blocks.append(markdown_block(block))
    # Send message to slack
    from slack_sdk import WebClient

    client = WebClient(token=os.environ['SLACK_DATADOG_AGENT_BOT_TOKEN'])
    message = ''.join(b['text']['text'] for b in blocks)
    print(message)
    MAX_BLOCKS = 50
    for idx in range(0, len(blocks), MAX_BLOCKS):
        client.chat_postMessage(channel=channel, blocks=blocks[idx : idx + MAX_BLOCKS], text=message)
