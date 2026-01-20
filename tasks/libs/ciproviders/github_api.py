from __future__ import annotations

import base64
import os
import re
from collections.abc import Iterable
from datetime import datetime, timedelta

import requests
from invoke import Context

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.constants import GITHUB_REPO_NAME
from tasks.libs.common.git import get_default_branch
from tasks.libs.common.user_interactions import yes_no_question

try:
    import semver
    from github import (
        Auth,
        Github,
        GithubException,
        GithubIntegration,
        GithubObject,
        InputGitTreeElement,
        PullRequest,
    )
    from github.NamedUser import NamedUser
except ImportError:
    # PyGithub isn't available on some build images, ignore it for now
    # and fail hard if it gets used.
    pass
from invoke.exceptions import Exit

__all__ = ["GithubAPI"]

RELEASE_BRANCH_PATTERN = re.compile(r"^\d+\.\d+\.x$")


class GithubAPI:
    """
    Helper class to perform API calls against the Github API, using a Github PAT.
    """

    BASE_URL = "https://api.github.com"

    def __init__(self, repository="DataDog/datadog-agent", public_repo=False):
        self._auth = self._chose_auth(public_repo)
        self._github = Github(auth=self._auth, per_page=100)
        org = repository.split("/")
        self._organization = org[0] if len(org) > 1 else None
        self._repository = self._github.get_repo(repository)

    @property
    def repo(self):
        """
        Gets the repo info.
        """

        return self._repository

    def graphql(self, query):
        """
        Perform a GraphQL query against the Github API.
        """

        headers = {"Authorization": "Bearer " + self._auth.token, "Content-Type": "application/json"}
        res = requests.post(
            "https://api.github.com/graphql",
            headers=headers,
            json={"query": query},
            timeout=10,
        )
        if res.status_code == 200:
            return res.json()
        raise RuntimeError(f"Failed to query Github: {res.text}")

    def get_branch(self, branch_name):
        """
        Gets info on a given branch in the given Github repository.
        """
        try:
            return self._repository.get_branch(branch_name)
        except GithubException as e:
            if e.status == 404:
                return None
            raise e

    def create_pr(self, pr_title, pr_body, base_branch, target_branch, draft=False):
        """
        Creates a PR in the given Github repository.
        """
        return self._repository.create_pull(
            title=pr_title, body=pr_body, base=base_branch, head=target_branch, draft=draft
        )

    def update_pr(self, pull_number, milestone_number, labels):
        """
        Updates a given PR with the provided milestone number and labels.
        """
        pr = self._repository.get_pull(pull_number)
        milestone = self._repository.get_milestone(milestone_number)
        issue = pr.as_issue()
        issue.edit(milestone=milestone, labels=labels)
        return pr

    def get_milestone_by_name(self, milestone_name):
        """
        Searches for a milestone in the given repository that matches the provided name,
        and returns data about it.
        """
        milestones = self._repository.get_milestones()
        for milestone in milestones:
            if milestone.title == milestone_name:
                return milestone
        return None

    def get_branch_protection(self, branch_name: str):
        """
        Get the protection of a given branch
        """
        branch = self.get_branch(branch_name)
        if not branch:
            raise Exit(color_message(f"Branch {branch_name} not found", Color.RED), code=1)
        elif not branch.protected:
            raise Exit(color_message(f"Branch {branch_name} doesn't have protection", Color.RED), code=1)
        try:
            protection = branch.get_protection()
        except GithubException as e:
            if e.status == 403:
                error_msg = f"""Can't access {branch_name} branch protection, probably due to missing permissions. You need either:
    - A Personal Access Token (PAT) needs the "repo" permissions.
    - Or a fine-grained token needs the "Administration" repository permissions.
"""
                raise PermissionError(error_msg) from e
            raise
        return protection

    def protection_to_payload(self, protection_raw_data: dict) -> dict:
        """
        Convert the protection object to a payload.
        See https://docs.github.com/en/rest/branches/branch-protection?apiVersion=2022-11-28#update-branch-protection

        The following seems to be defined at the Org scale, so we're not resending them here:
        - required_pull_request_reviews > dismissal_restrictions
        - required_pull_request_reviews > bypass_pull_request_allowances
        """
        prot = protection_raw_data
        return {
            "required_status_checks": {
                "strict": prot["required_status_checks"]["strict"],
                "checks": [
                    {"context": check["context"], "app_id": -1 if check["app_id"] is None else check["app_id"]}
                    for check in prot["required_status_checks"]["checks"]
                ],
            },
            "enforce_admins": prot["enforce_admins"]["enabled"],
            "required_pull_request_reviews": {
                "dismiss_stale_reviews": prot["required_pull_request_reviews"]["dismiss_stale_reviews"],
                "require_code_owner_reviews": prot["required_pull_request_reviews"]["require_code_owner_reviews"],
                "required_approving_review_count": prot["required_pull_request_reviews"][
                    "required_approving_review_count"
                ],
                "require_last_push_approval": prot["required_pull_request_reviews"]["require_last_push_approval"],
            },
            "restrictions": {
                "users": prot["restrictions"]["users"],
                "teams": prot["restrictions"]["teams"],
                "apps": [app["slug"] for app in prot["restrictions"]["apps"]],
            },
            "required_linear_history": prot["required_linear_history"]["enabled"],
            "allow_force_pushes": prot["allow_force_pushes"]["enabled"],
            "allow_deletions": prot["allow_deletions"]["enabled"],
            "block_creations": prot["block_creations"]["enabled"],
            "required_conversation_resolution": prot["required_conversation_resolution"]["enabled"],
            "lock_branch": prot["lock_branch"]["enabled"],
            "allow_fork_syncing": prot["allow_fork_syncing"]["enabled"],
        }

    def get_branch_required_checks(self, branch_name: str) -> list[str]:
        """
        Get the required checks for a given branch
        """
        return self.get_branch_protection(branch_name).required_status_checks.contexts

    def add_branch_required_check(self, branch_name: str, checks: list[str], force: bool = False) -> None:
        """
        Add required checks to a given branch

        It uses the Github API directly to add the required checks to the branch.
        Using the "checks" argument is not supported by PyGithub.
        :calls: `PUT /repos/{owner}/{repo}/branches/{branch}/protection

        """
        current_protection = self.get_branch_protection(branch_name)
        current_required_checks = current_protection.required_status_checks.contexts
        new_required_checks = []
        for check in checks:
            if check in current_required_checks:
                print(
                    color_message(
                        f"Ignoring the '{check}' check as it is already required on the {branch_name} branch",
                        Color.ORANGE,
                    )
                )
            else:
                new_required_checks.append(check)
        if not new_required_checks:
            print(color_message("No new checks to add", Color.GREEN))
            return
        print(
            color_message(
                f"Warning: You are about to add the following checks to the {branch_name} branch:\n{new_required_checks}",
                Color.ORANGE,
            )
        )
        print(color_message(f"Current required checks: {sorted(current_required_checks)}", Color.GREY))
        if force or yes_no_question("Are you sure?", default=False):
            # We're crafting the request and not using PyGithub because it doesn't support passing the checks variable instead of contexts.
            protection_url = f"{self.repo.url}/branches/{branch_name}/protection"
            headers = {
                "Accept": "application/vnd.github+json",
                "Authorization": f"Bearer {self._auth.token}",
                "X-GitHub-Api-Version": "2022-11-28",
            }
            payload = self.protection_to_payload(current_protection.raw_data)
            payload["required_status_checks"]["checks"] = sorted(
                payload["required_status_checks"]["checks"]
                + [{"context": check, "app_id": -1} for check in new_required_checks],
                key=lambda x: x['context'],
            )

            response = requests.put(protection_url, headers=headers, json=payload, timeout=10)
            if response.status_code != 200:
                print(
                    color_message(
                        f"Error while sending the PUT request to {protection_url}\n{response.text}", Color.RED
                    )
                )
                raise Exit(
                    color_message(f"Failed to update the required checks for the {branch_name} branch", Color.RED),
                    code=1,
                )
            print(color_message(f"The {checks} checks were successfully added!", Color.GREEN))
        else:
            print(color_message("Aborting changes to the branch required checks", Color.GREEN))

    def is_release_note_needed(self, pull_number):
        """
        Check if labels are ok for skipping QA
        """
        pr = self._repository.get_pull(int(pull_number))
        labels = [label.name for label in pr.get_labels()]
        if "changelog/no-changelog" in labels:
            return False
        return True

    def contains_release_note(self, pull_number):
        """
        Look in modified files for a release note
        """
        pr = self._repository.get_pull(int(pull_number))
        for file in pr.get_files():
            if (
                file.filename.startswith("releasenotes/notes/")
                or file.filename.startswith("releasenotes-dca/notes/")
                or file.filename.startswith("releasenotes-installscript/notes")
            ):
                return True
        return False

    def get_pulls(self, milestone=None, labels=None):
        if milestone is None:
            m = GithubObject.NotSet
        else:
            m = self.get_milestone_by_name(milestone)
            if not m:
                print(f'Unknown milestone {milestone}')
                return None
        if labels is None:
            labels = []
        issues = self._repository.get_issues(milestone=m, state='all', labels=labels)
        return [i.as_pull_request() for i in issues if i.pull_request is not None]

    def get_pr_for_branch(self, head_branch_name=None, base_branch_name=None):
        query_params = {"state": "open"}
        if head_branch_name:
            query_params["head"] = f'DataDog:{head_branch_name}'
        if base_branch_name:
            query_params["base"] = base_branch_name
        return self._repository.get_pulls(**query_params)

    def get_tags(self, pattern=""):
        """
        List all tags starting with the provided pattern.
        If the pattern is empty or None, all tags will be returned
        """
        tags = self._repository.get_tags()
        if pattern is None or len(pattern) == 0:
            return list(tags)
        return [t for t in tags if t.name.startswith(pattern)]

    def trigger_workflow(self, workflow_name, ref, inputs=None):
        """
        Create a pipeline targeting a given reference of a project.
        ref must be a branch or a tag.
        """
        workflow = self._repository.get_workflow(workflow_name)
        if workflow is None:
            return False
        if inputs is None:
            inputs = {}
        return workflow.create_dispatch(ref, inputs)

    def workflow_run(self, run_id):
        """
        Gets info on a specific workflow.
        """
        return self._repository.get_workflow_run(run_id)

    def download_artifact(self, artifact, destination_dir):
        """
        Downloads the artifact identified by artifact_id to destination_dir.
        """
        url = artifact.archive_download_url

        return self.download_from_url(url, destination_dir, destination_file=artifact.id)

    def download_from_url(self, url, destination_dir, destination_file):
        import requests

        headers = {
            "Authorization": f'{self._auth.token_type} {self._auth.token}',
            "Accept": "application/vnd.github.v3+json",
        }
        # Retrying this request if needed is handled by the caller
        with requests.get(url, headers=headers, stream=True, timeout=10) as r:
            r.raise_for_status()
            zip_target_path = os.path.join(destination_dir, f"{destination_file}.zip")
            with open(zip_target_path, "wb") as f:
                for chunk in r.iter_content(chunk_size=8192):
                    f.write(chunk)
        return zip_target_path

    def download_logs(self, run_id, destination_dir):
        run = self._repository.get_workflow_run(run_id)
        logs_url = run.logs_url
        _, headers, _ = run._requester.requestJson("GET", logs_url)

        return self.download_from_url(headers["location"], destination_dir, run.id)

    def workflow_run_for_ref_after_date(self, workflow_name, ref, oldest_date):
        """
        Gets all the workflow triggered after a given date
        """
        workflow = self._repository.get_workflow(workflow_name)
        runs = workflow.get_runs(branch=ref)
        recent_runs = [run for run in runs if run.created_at > oldest_date]

        return sorted(recent_runs, key=lambda run: run.created_at, reverse=True)

    def latest_release(self, major_version=7) -> str:
        if major_version == 6:
            return max((r for r in self.get_releases() if r.title.startswith('6.53')), key=lambda r: r.created_at).title
        release = self._repository.get_latest_release()
        return release.title

    def latest_release_tag(self) -> str:
        release = self._repository.get_latest_release()
        return release.tag_name

    def get_releases(self):
        return self._repository.get_releases()

    def latest_unreleased_release_branches(self):
        """
        Get all the release branches that are newer than the latest release.
        """
        release = self._repository.get_latest_release()
        released_version = semver.VersionInfo.parse(release.title)

        for branch in self.release_branches():
            if semver.VersionInfo.parse(branch.name.replace("x", "0")) > released_version:
                yield branch

    def release_branches(self):
        """
        Yield all the branches that match the release branch pattern (A.B.x).
        """
        for branch in self._repository.get_branches():
            if RELEASE_BRANCH_PATTERN.match(branch.name):
                yield branch

    def get_rate_limit_info(self):
        """
        Gets the current rate limit info.
        """
        return self._github.rate_limiting

    def publish_comment(self, pr, comment):
        """
        Publish a comment on a given PR.

        - pr: PR number or PR object
        """
        if not isinstance(pr, PullRequest.PullRequest):
            pr = self._repository.get_pull(int(pr))

        pr.create_issue_comment(comment)

    def find_comment(self, pr, content):
        """
        Get a comment that contains content on a given PR.

        - pr: PR number or PR object
        """

        if not isinstance(pr, PullRequest.PullRequest):
            pr = self._repository.get_pull(int(pr))

        comments = pr.get_issue_comments()
        for comment in comments:
            if content in comment.body.splitlines():
                return comment

    def get_pr(self, pr_id: int):
        return self._repository.get_pull(pr_id)

    def add_pr_label(self, pr_id: int, label: str) -> None:
        """
        Tries to add a label to the pull request
        """
        pr = self.get_pr(pr_id)
        pr.add_to_labels(label)

    def get_pr_labels(self, pr_id: int) -> list[str]:
        """
        Returns the labels of a pull request
        """
        pr = self.get_pr(pr_id)

        return [label.name for label in pr.get_labels()]

    def update_review_complexity_labels(self, pr_id: int, new_label: str) -> None:
        """
        Updates the review complexity label of a pull request
        """
        pr = self.get_pr(pr_id)
        already_there = False
        for label in pr.get_labels():
            if label.name.endswith(" review"):
                if label.name == new_label:
                    already_there = True
                else:
                    pr.remove_from_labels(label.name)

        if not already_there:
            pr.add_to_labels(new_label)

    def get_pr_files(self, pr_id: int) -> list[str]:
        """
        Returns the files involved in the PR
        """
        pr = self.get_pr(pr_id)

        return [f.filename for f in pr.get_files()]

    def get_team_members(self, team_slug: str) -> Iterable[NamedUser]:
        """
        Get the members of a team.
        """
        team = self.get_team(team_slug)
        return team.get_members()

    def get_team(self, team_slug: str):
        """
        Get the team object.
        """
        assert self._organization
        org = self._github.get_organization(self._organization)
        return org.get_team_by_slug(team_slug)

    def search_issues(self, query: str):
        """
        Search for issues with the given query.
        By default this is not scoped to the repository, it is a global Github search.
        """
        return self._github.search_issues(query)

    def commit_and_push_signed(self, branch_name: str, commit_message: str, tree: dict[str, dict[str, str]]):
        # Create a commit from the given tree, see details in https://github.com/orgs/community/discussions/50055
        base_tree = self._repository.get_git_tree(tree['base_tree'])
        git_tree = self._repository.create_git_tree(
            [InputGitTreeElement(**blob) for blob in tree['tree']], base_tree=base_tree
        )
        commit = self._repository.create_git_commit(
            commit_message,
            git_tree,
            [self._repository.get_git_commit(tree['base_tree'])],
        )
        # The update ref API endpoint is not available in PyGithub, so we need to use the raw API
        data = {"sha": commit.sha, "force": False}
        headers = {"Authorization": "Bearer " + self._auth.token, "Content-Type": "application/json"}
        res = requests.patch(
            url=f"{self._repository.url}/git/refs/heads/{branch_name}", json=data, headers=headers, timeout=10
        )
        if res.status_code == 200:
            return res.json()
        raise Exit(f"Failed to update the reference {branch_name} with commit {commit.sha}: {res.text}")

    def _chose_auth(self, public_repo):
        """
        Attempt to find a working authentication, in order:
            - Locally:
              - Short lived token generated locally
            - On CI:
              - GITHUB_TOKEN environment variable
              - A fake login user/password to reach public repositories
              - An app token through the GITHUB_APP_ID & GITHUB_KEY_B64 environment
                variables (can also use GITHUB_INSTALLATION_ID to save a request).
                This is required for Gitlab CI.
        """
        from tasks.libs.common.utils import running_in_ci

        if not running_in_ci():
            return Auth.Token(generate_local_github_token(Context()))
        if "GITHUB_TOKEN" in os.environ:
            return Auth.Token(os.environ["GITHUB_TOKEN"])
        if public_repo:
            return Auth.Login("user", "password")

        if "GITHUB_APP_ID" not in os.environ or "GITHUB_KEY_B64" not in os.environ:
            raise Exit(
                message="For private repositories on CI, you need to set the GITHUB_APP_ID and GITHUB_KEY_B64 environment variables",
                code=1,
            )

        appAuth = Auth.AppAuth(
            os.environ['GITHUB_APP_ID'], base64.b64decode(os.environ['GITHUB_KEY_B64']).decode('ascii')
        )
        installation_id = os.environ.get('GITHUB_INSTALLATION_ID', None)
        if installation_id is None:
            # Even if we don't know the installation id, there's an API endpoint to
            # retrieve it, given the other credentials (app id + key).
            integration = GithubIntegration(auth=appAuth)
            installations = integration.get_installations()
            if installations.totalCount == 0:
                raise Exit(message='No usable installation found', code=1)
            installation_id = installations[0].id
        return appAuth.get_installation_auth(int(installation_id))

    @staticmethod
    def get_token_from_app(app_id_env='GITHUB_APP_ID', pkey_env='GITHUB_KEY_B64'):
        app_id = os.environ.get(app_id_env)
        app_key_b64 = os.environ.get(pkey_env)
        if app_id is None or app_key_b64 is None:
            raise RuntimeError(f"Missing {app_id_env} or {pkey_env}")
        app_key = base64.b64decode(app_key_b64).decode("ascii")

        auth = Auth.AppAuth(app_id, app_key)
        integration = GithubIntegration(auth=auth)
        installations = integration.get_installations()
        if installations.totalCount == 0:
            raise RuntimeError("Failed to list app installations")
        install_id = installations[0].id
        auth_token = integration.get_access_token(install_id)
        print(auth_token.token)

    def create_label(self, name, color, description="", exist_ok=False):
        """
        Creates a label in the given GitHub repository.
        """

        try:
            return self._repository.create_label(name, color, description)
        except GithubException as e:
            if not (
                e.status == 422
                and len(e.data["errors"]) == 1
                and e.data["errors"][0]["code"] == "already_exists"
                and exist_ok
            ):
                raise e

    def create_milestone(self, title, exist_ok=False):
        """
        Creates a milestone in the given GitHub repository.
        """

        try:
            return self._repository.create_milestone(title)
        except GithubException as e:
            if not (
                e.status == 422
                and len(e.data["errors"]) == 1
                and e.data["errors"][0]["code"] == "already_exists"
                and exist_ok
            ):
                raise e

    def create_release(self, tag, message, draft=True):
        return self._repository.create_git_release(
            tag=tag,
            name=tag,
            message=message,
            draft=draft,
        )

    def get_codereview_complexity(self, pr_id: int) -> str:
        """
        Get the complexity of the code review for a given PR, taking into account the number of files, lines and comments.
        """
        pr = self._repository.get_pull(pr_id)
        # Criteria are defined with the average of PR attributes (files, lines, comments) so that:
        # - easy PRs are merged in less than 1 day
        # - hard PRs are merged in more than 1 week
        # More details about criteria definition: https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4271079846/Code+Review+Experience+Improvement#Complexity-label
        criteria = {
            'easy': {'files': 4, 'lines': 150, 'comments': 2},
            'hard': {'files': 12, 'lines': 650, 'comments': 9},
        }
        if (
            pr.changed_files < criteria['easy']['files']
            and pr.additions + pr.deletions < criteria['easy']['lines']
            and pr.review_comments < criteria['easy']['comments']
        ):
            return 'short review'
        elif (
            pr.changed_files > criteria['hard']['files']
            or pr.additions + pr.deletions > criteria['hard']['lines']
            or pr.review_comments > criteria['hard']['comments']
        ):
            return 'long review'
        return 'medium review'

    def find_teams(self, obj, exclude_teams=None, exclude_permissions=None, depth=None):
        """Get teams from a Github object (repository or team)"""
        teams = []
        if depth is not None:
            depth -= 1
        for team in obj.get_teams():
            if (
                exclude_teams
                and team.name in exclude_teams
                or exclude_permissions
                and team.permission in exclude_permissions
            ):
                continue
            teams.append(team)
            if depth is None or depth > 0:
                teams.extend(self.find_teams(team, depth=depth))
        return teams

    def get_active_users(self, duration_days=183):
        """Get the set of reviewers within the last <duration_days>"""
        actors = set()
        since_date = datetime.now() - timedelta(days=duration_days)
        for pr in self._repository.get_pulls(state="all"):
            actors.add(pr.user.login)
            if pr.created_at < since_date:
                break
            for review in pr.get_reviews():
                if review.user:
                    actors.add(review.user.login)
        return actors

    def get_direct_team_members(self, team):
        query = '{ organization(login: "datadog") { team(slug: "TEAM")  { members(membership: IMMEDIATE) { nodes { login } } } } }'.replace(
            "TEAM", team
        )
        data = self.graphql(query)
        return [member["login"] for member in data["data"]["organization"]["team"]["members"]["nodes"]]

    def get_fork_name(self, owner):
        forks = self._repository.get_forks()
        for fork in forks:
            if fork.owner.login == owner:
                return fork.name
        return None


def create_datadog_agent_pr(title, base_branch, target_branch, milestone_name, other_labels=None, body=""):
    print(color_message("Creating PR", "bold"))

    github = GithubAPI(repository=GITHUB_REPO_NAME)

    milestone = github.get_milestone_by_name(milestone_name)

    if not milestone or not milestone.number:
        raise Exit(
            color_message(
                f"""Could not find milestone {milestone_name} in the Github repository. Response: {milestone}
Make sure that milestone is open before trying again.""",
                "red",
            ),
            code=1,
        )

    pr = github.create_pr(
        pr_title=title,
        pr_body=body,
        base_branch=base_branch,
        target_branch=target_branch,
    )

    if not pr:
        raise Exit(
            color_message(f"Could not create PR in the Github repository. Response: {pr}", "red"),
            code=1,
        )

    print(color_message(f"Created PR #{pr.number}", "bold"))

    labels = [
        "changelog/no-changelog",
        "qa/no-code-change",
    ]

    if other_labels:
        labels += other_labels

    updated_pr = github.update_pr(
        pull_number=pr.number,
        milestone_number=milestone.number,
        labels=labels,
    )

    if not updated_pr or not updated_pr.number or not updated_pr.html_url:
        raise Exit(
            color_message(f"Could not update PR in the Github repository. Response: {updated_pr}", "red"),
            code=1,
        )

    print(color_message(f"Set labels and milestone for PR #{updated_pr.number}", "bold"))
    print(color_message(f"Done creating new PR. Link: {updated_pr.html_url}", "bold"))

    return updated_pr.html_url


def create_release_pr(title, base_branch, target_branch, version, changelog_pr=False, milestone=None):
    if milestone:
        milestone_name = milestone
    else:
        from tasks.libs.releasing.json import get_current_milestone

        milestone_name = get_current_milestone()

    labels = [
        "team/agent-delivery",
    ]
    if changelog_pr:
        labels.append(f"backport/{get_default_branch()}")

    return create_datadog_agent_pr(title, base_branch, target_branch, milestone_name, labels)


def generate_local_github_token(ctx):
    """
    Generates a github token locally.
    """

    try:
        token = ctx.run('ddtool auth github token', hide=True).stdout.strip()

        assert token.startswith('gh') and ' ' not in token, (
            "`ddtool auth github token` returned an invalid token, "
            "it might be due to ddtool outdated. "
            "Please run `brew update && brew upgrade ddtool`."
        )

        return token
    except AssertionError:
        # No retry on asserts
        raise
    except Exception:
        # Try to login and then get a token
        ctx.run('ddtool auth github login')
        token = ctx.run('ddtool auth github token', hide=True).stdout.strip()

        return token
