from __future__ import annotations

import base64
import json
import os
import platform
import re
import subprocess
from collections.abc import Iterable
from functools import lru_cache

import requests

from tasks.libs.common.color import color_message
from tasks.libs.common.constants import GITHUB_REPO_NAME

try:
    import semver
    from github import Auth, Github, GithubException, GithubIntegration, GithubObject, PullRequest
    from github.NamedUser import NamedUser
except ImportError:
    # PyGithub isn't available on some build images, ignore it for now
    # and fail hard if it gets used.
    pass
from invoke.exceptions import Exit

__all__ = ["GithubAPI"]

RELEASE_BRANCH_PATTERN = re.compile(r"\d+\.\d+\.x")


class GithubAPI:
    """
    Helper class to perform API calls against the Github API, using a Github PAT.
    """

    BASE_URL = "https://api.github.com"

    def __init__(self, repository="DataDog/datadog-agent", public_repo=False):
        self._auth = self._chose_auth(public_repo)
        self._github = Github(auth=self._auth)
        org = repository.split("/")
        self._organization = org[0] if len(org) > 1 else None
        self._repository = self._github.get_repo(repository)

    @property
    def repo(self):
        """
        Gets the repo info.
        """

        return self._repository

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

    def create_pr(self, pr_title, pr_body, base_branch, target_branch):
        """
        Creates a PR in the given Github repository.
        """
        return self._repository.create_pull(title=pr_title, body=pr_body, base=base_branch, head=target_branch)

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

    def get_pr_for_branch(self, branch_name):
        return self._repository.get_pulls(state="open", head=f'DataDog:{branch_name}')

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
        with requests.get(url, headers=headers, stream=True) as r:
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

    def latest_release(self) -> str:
        release = self._repository.get_latest_release()
        return release.title

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
        assert self._organization
        org = self._github.get_organization(self._organization)
        team = org.get_team_by_slug(team_slug)
        return team.get_members()

    def search_issues(self, query: str):
        """
        Search for issues with the given query.
        By default this is not scoped to the repository, it is a global Github search.
        """
        return self._github.search_issues(query)

    def is_organization_member(self, user):
        organization = self._repository.organization
        return (user.company and 'datadog' in user.company.casefold()) or organization.has_in_members(user)

    def _chose_auth(self, public_repo):
        """
        Attempt to find a working authentication, in order:
            - Personal access token through GITHUB_TOKEN environment variable
            - An app token through the GITHUB_APP_ID & GITHUB_KEY_B64 environment
              variables (can also use GITHUB_INSTALLATION_ID to save a request)
            - A token from macOS keychain
            - A fake login user/password to reach public repositories
        """
        if "GITHUB_TOKEN" in os.environ:
            return Auth.Token(os.environ["GITHUB_TOKEN"])
        if "GITHUB_APP_ID" in os.environ and "GITHUB_KEY_B64" in os.environ:
            appAuth = Auth.AppAuth(
                os.environ['GITHUB_APP_ID'], base64.b64decode(os.environ['GITHUB_KEY_B64']).decode('ascii')
            )
            installation_id = os.environ.get('GITHUB_INSTALLATION_ID', None)
            if installation_id is None:
                # Even if we don't know the installation id, there's an API endpoint to
                # retrieve it, given the other credentials (app id + key).
                integration = GithubIntegration(auth=appAuth)
                installations = integration.get_installations()
                if len(installations) == 0:
                    raise Exit(message='No usable installation found', code=1)
                installation_id = installations[0]
            return appAuth.get_installation_auth(int(installation_id))
        if public_repo:
            return Auth.Login("user", "password")
        if platform.system() == "Darwin":
            try:
                output = (
                    subprocess.check_output(
                        ['security', 'find-generic-password', '-a', os.environ["USER"], '-s', 'GITHUB_TOKEN', '-w']
                    )
                    .decode()
                    .strip()
                )

                if output:
                    return Auth.Token(output)
            except subprocess.CalledProcessError:
                print("GITHUB_TOKEN not found in keychain...")
                pass
        raise Exit(
            message="Please create a 'repo' access token at "
            "https://github.com/settings/tokens and "
            "add it as GITHUB_TOKEN in your keychain "
            "or export it from your .bashrc or equivalent.",
            code=1,
        )

    @staticmethod
    def get_token_from_app(app_id_env='GITHUB_APP_ID', pkey_env='GITHUB_KEY_B64'):
        app_id = os.environ.get(app_id_env)
        app_key_b64 = os.environ.get(pkey_env)
        app_key = base64.b64decode(app_key_b64).decode("ascii")

        auth = Auth.AppAuth(app_id, app_key)
        integration = GithubIntegration(auth=auth)
        installations = integration.get_installations()
        if installations.totalCount == 0:
            raise RuntimeError("Failed to list app installations")
        install_id = installations[0].id
        auth_token = integration.get_access_token(install_id)
        print(auth_token.token)

    def create_label(self, name, color, description=""):
        """
        Creates a label in the given GitHub repository.
        """
        return self._repository.create_label(name, color, description)

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
            'easy': {'files': 4, 'lines': 150, 'comments': 1},
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


def get_github_teams(users):
    for user in users:
        yield from query_teams(user.login)


@lru_cache
def query_teams(login):
    query = get_user_query(login)
    headers = {"Authorization": f"Bearer {os.environ['GITHUB_TOKEN']}", "Content-Type": "application/json"}
    response = requests.post("https://api.github.com/graphql", headers=headers, data=query)
    data = response.json()
    teams = []
    try:
        if data["data"]["user"]["organization"] and data["data"]["user"]["organization"]["teams"]:
            for team in data["data"]["user"]["organization"]["teams"]["nodes"]:
                teams.append(team["slug"])
    except KeyError:
        print(f"Error for user {login}: {data}")
        raise
    return teams


def get_user_query(login):
    variables = {"login": login, "org": "datadog"}
    query = '{"query": "query GetUserTeam($login: String!, $org: String!) { user(login: $login) {organization(login: $org) { teams(first:10, userLogins: [$login]){ nodes { slug } } } } }", '
    string_var = f'"variables": {json.dumps(variables)}'
    return query + string_var


def create_release_pr(title, base_branch, target_branch, version, changelog_pr=False):
    print(color_message("Creating PR", "bold"))

    github = GithubAPI(repository=GITHUB_REPO_NAME)

    # Find milestone based on what the next final version is. If the milestone does not exist, fail.
    milestone_name = str(version)

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
        pr_body="",
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
        "team/agent-delivery",
        "team/agent-release-management",
        "category/release_operations",
    ]

    if changelog_pr:
        labels.append("backport/main")

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
